# Query Language Specification

## Introduction

Fraise is designed to be queried by agents over and over again. The main decision was to create a simple query language allowing to retrieve information. The FQL is
designed for token economy while being LLM-friendly.

### Principles

Human DSL design optimizes for expressiveness and aestetic flexibility, this query language optimizes for simplicity and chooses to remove ambiguity whenever possibe: for an AI-agent, degrees of freedom are a potential source of failure. Here are a few design principles:

* commands and field names are written in lower case. They are syntax, matched exactly, so `RECALL x` is a parse error rather than a query — nothing is silently rewritten on the way in.
* everything else is data: terms, anchor values and the text inside a phrase are **stored** exactly as written, and **matched** without regard to case. `remember 'MiXeD Case'` comes back spelled that way, and `recall mixed` finds it. An agent should not have to remember how it capitalised something a hundred turns ago.
* the query language only allows to run one command at a time. Multiple commands in a single instructions leads to a runtime error.
* the query language only allows for single line instructions. New line characters are processed as whitespace and the query is executed if valid.

The principle of removing ambiguity is one that remains central to new versions of the DSL. As much as possible we'll avoid introducing features that allow to express the same concept in multiple ways.

## Representation

Queries are not designed to be read from file, but directly from a UTF-8 encoded string.

## Tokens

Tokens form the vocabulary of the fraise query language. The classes of token are the following:

* keywords (command, operators and fields)
* literals
* punctuations
* fields

### Keywords

Keywords are specific query instructions. They are reserved: the lexer gives
each one its own token, so none of them can be used as a bare term. `recall
topic` is a parse error, not a search for the word "topic" — quote it (`recall
'topic'`) to search for the word itself.

| Keyword  | Usage          | Type    |
|----------|----------------|---------|
| remember | remember_query | Command |
| recall   | recall_query   | Command |
| topic    | topic_field    | Field   |
| entity   | entity_field   | Field   |
| since    | since_field    | Field   |
| until    | until_field    | Field   |
| top      | top_field      | Field   |
| depth    | depth_field    | Field   |
| vec      | vec_field      | Field   |

`forget` and `update` are reserved by the lexer as well, but no command
implements them: a query starting with either is rejected. They are listed here
because reserving them is already observable — neither can be used as a term.

There are no boolean operators. Several terms in one recall are a union (any
match seeds the search), and there is no way to write a conjunction or a
negation.

### Punctuation

Four characters carry meaning. Each one is a single token, and each one also
ends the word before it — which is why they never need whitespace around them,
and why a term cannot contain one.

| Character | Meaning                                                     |
|-----------|-------------------------------------------------------------|
| `:`       | separates a field from its value (`topic:billing`)          |
| `'`       | delimits a phrase; `''` inside one is an escaped apostrophe |
| `$`       | introduces a parameter reference (`vec:$v`)                 |
| `@`       | selects the graph, glued to the command (`recall@3`)        |

`(`, `)`, `+`, `-` and `~` are also lexed as tokens, but no production accepts
them: a query containing one is rejected wherever it appears. `-` is the
exception worth knowing, since it is ordinary inside a word — `foo-bar` is a
single term, while a leading `-` starts a token nothing can consume.

### Identifiers

An identifier is a bare word: a run of characters ending at whitespace or at one
of the punctuation characters above. Terms, anchor values, and field values are
all identifiers unless they are quoted.

* Letters, digits, `_` and `-` are ordinary. `foo-bar`, `foo_bar` and `123` are
  each a single identifier.
* Case is stored but not matched on: a fact keeps the case it was written with,
  and `topic:Billing`, `topic:billing` and `topic:BILLING` all select it. An
  agent never has to remember how it capitalised something.
* A reserved keyword is not an identifier (see above). Quote it to use one as a
  value: `topic:'entity'`.
* An identifier cannot be empty — `topic:` with nothing after it is an error,
  not an empty anchor.

Anything a bare word cannot express, a quoted phrase can: quoting is the single
escape hatch, and inside quotes nothing is reserved.

### Literals

| Literal   | Form              | Notes                                                       |
|-----------|-------------------|-------------------------------------------------------------|
| phrase    | `'...'`           | opaque: every character is literal, `''` is one apostrophe  |
| integer   | `0`, `10`, `1000` | non-negative only; a leading `-` is a separate token        |
| duration  | `7d`, `30m`, `1w` | `<integer><unit>`, units `s` `m` `h` `d` `w`, read as "ago" |
| date      | `2026-01-15`      | `YYYY-MM-DD`                                                |
| param ref | `$v`              | see below                                                   |

A date carries no time of day. `since:2026-01-15T10:00:00Z` does not parse — the
first `:` ends the value, so the rest of the timestamp arrives as stray tokens.
Whole days are the finest an absolute bound can be written to; use a duration
when finer is needed.

### Field reference

A field reference binds a value the query does not carry inline. It is written
`$name` and resolved against the `parameters` object of the request:

```json
{"query": "recall bird vec:$v top:5", "parameters": {"v": [0.1, 0.2, 0.3]}}
```

`vec` is the only field that takes one, and it accepts nothing else — the `$` is
required, so `vec:v` is an error. It is valid on both commands: on `recall` the
vector is a semantic seed, on `remember` it is the embedding stored with the
fact.

Keeping vectors out of the query string is the point. A query is a cache key and
a log line; an inline embedding would make both unusable, and would put a few
thousand floats through the lexer on every call.

A query naming a parameter the request does not carry is rejected as a client
error, rather than running as though the field had been omitted.

## Grammar

```ebnf
(* ============================================================ *)
(* Fraise Query Language (FQL) — v0.1.0                         *)
(* ============================================================ *)

query           = recall_query | remember_query ;

(* ------------------------------------------------------------ *)
(* RECALL                                                       *)
(* ------------------------------------------------------------ *)

recall_query    = recall_cmd recall_body ;
recall_cmd      = 'recall' graph_selector? ;   (* graph glued to verb: recall@3 — no whitespace before '@' *)

(* SEMANTIC RULE: a recall needs at least one term, and the terms  *)
(* come first. Anchors and modifiers narrow a search, they cannot  *)
(* start one, so neither 'recall topic:billing' nor 'recall        *)
(* vec:$v' is a query — and after the first field, a bare term is  *)
(* an error. The fields themselves are free to interleave in any   *)
(* order: repeating an anchor adds a filter, while repeating a     *)
(* modifier keeps the last value given.                            *)
recall_body     = term+ field* ;
field           = anchor | modifier ;

term            = bare_word | phrase ;        (* soft ranking seed (SHOULD) *)

anchor          = anchor_field ;              (* every anchor filters; there is no way to say MUST_NOT *)
anchor_field    = topic_field | entity_field ;
topic_field     = 'topic'  ':' anchor_value ;
entity_field    = 'entity' ':' anchor_value ;
anchor_value    = identifier | quoted_identifier ;

modifier        = since_field | until_field | depth_field | top_field | vec_field ;
since_field     = 'since' ':' time_value ;     (* lower time bound; duration read as "ago" *)
until_field     = 'until' ':' time_value ;     (* upper time bound; duration read as "ago" *)
depth_field     = 'depth' ':' integer ;        (* accepted, currently inert: reserved for iterated transmission *)
top_field       = 'top'   ':' integer ;        (* result limit, default 10 *)
vec_field       = 'vec'   ':' param_ref ;      (* semantic seed (optional) *)

(* ------------------------------------------------------------ *)
(* REMEMBER                                                     *)
(* ------------------------------------------------------------ *)

(* Asserts a fact (phrase), the anchors it is filed under, and    *)
(* optionally its embedding. Anchors are not required: a fact with *)
(* none is stored and reachable by text search alone.              *)
remember_query  = remember_cmd phrase (anchor_field | vec_field)* ;
remember_cmd    = 'remember' graph_selector? ;   (* graph glued to verb: remember@8 — no whitespace before '@' *)

(* ------------------------------------------------------------ *)
(* SHARED                                                       *)
(* ------------------------------------------------------------ *)

graph_selector  = '@' integer ;                (* default 0; lexed WITH the command as one token: (recall|remember)(@[0-9]+)? *)

(* ------------------------------------------------------------ *)
(* LITERALS                                                     *)
(* ------------------------------------------------------------ *)

bare_word         = ?[^\s:'$()@+~-][^\s:'$()@]*? ; (* '-' may appear inside a word, never at its start *)
phrase            = "'" { ?[^']? | "''" } "'" ; (* opaque: any char is literal; '' is an escaped quote *)
identifier        = bare_word ;                (* case is preserved, not folded *)
quoted_identifier = "'" { ?[^']? | "''" } "'" ; (* same opaque rule as phrase *)
integer           = ?[0-9]+? ;                 (* non-negative; '-' is its own token *)
time_value        = duration | iso_date ;
duration          = integer time_unit ;
time_unit         = 's' | 'm' | 'h' | 'd' | 'w' ;
iso_date          = ?\d{4}-\d{2}-\d{2}? ;         (* date only: ':' would end the token *)
param_ref         = '$' identifier ;
```

## Examples

```text
recall billing entity:acme since:7d top:5
recall billing topic:billing
recall 'annual contract' topic:billing entity:acme depth:3
recall@3 auth topic:auth entity:okta
remember 'acme moved to annual billing' topic:billing topic:contracts
remember 'acme signed with okta' topic:auth entity:okta
remember 'meeting at 3:30pm about the topic' topic:meetings   (* colons and reserved words are literal inside a phrase *)
remember 'alice''s laptop' topic:devices                     (* '' is an escaped apostrophe -> alice's laptop *)
```

## Glossary

* **fact**: a memory atomic element.
* **memory graph**: a collection of organised facts related to topics and / or entities and indexed for hybrid search (full text, semantic and graph)
* **field**: additional information provided to the search in order to find the right facts.
* **field value**: value of a field used to find a fact
* **term**: a word or sequence of words used to find a fact.
* **command**: task to perform by the engine: recall, remember.
* **instruction**: a full query sent to the database engine with a command, search terms and fields (can be used interchangeably with the term query).
