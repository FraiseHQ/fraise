# Query Language Specification

## Introduction

Fraise is designed to be queried by agents over and over again. The main decision was to create a simple query language allowing to retrieve information. The FQL is
designed for token economy while being LLM-friendly.

### Principles

Human DSL design optimizes for expressiveness and aestetic flexibility, this query language optimizes for simplicity and chooses to remove ambiguity whenever possibe: for an AI-agent, degrees of freedom are a potential source of failure. Here are a few design principles:

* commands and field names are written in lower case. They are syntax, matched exactly, so `RECALL x` is a parse error rather than a query — nothing is silently rewritten on the way in.
* everything else is data. A remembered fact — the quoted phrase — is **stored** exactly as written and **matched** without regard to case: `remember 'MiXeD Case'` comes back spelled that way, and `recall mixed` finds it. Terms and anchor values are identity rather than prose, so the parser folds them to lower case on the way in — `topic:Billing` and `topic:billing` are the same anchor. An agent should not have to remember how it capitalised something a hundred turns ago.
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

Keywords are specific query instructions. They are reserved by position, not
by spelling alone: where a clause can start, a keyword reads as syntax, and
`recall food topic` is a parse error rather than a search including the word
"topic". Where only a value can appear — the right-hand side of a field's `:`,
or the leading term a recall must start with — a keyword is ordinary data:
`entity:top` files under the word "top", and `recall top` searches for it.
One tie-breaker resolves the boundary: a keyword immediately followed by `:`
is always a field, so `top:3` never means the word "top", and `entity:top:3`
is an error rather than a guess. Quoting (`recall food 'topic'`) remains the
escape hatch wherever a bare keyword still reads as syntax.

Keywords are written in lower case only, and casing does not un-reserve one:
where a clause could start, a mis-cased keyword is an error naming the casing
— `recall x Since 7d` is rejected, never read as a three-term search. Folding
`Since` into a term there would let a mis-typed clause silently become data,
scoping the query by nothing with no error to correct from.

One ambiguity survives, at the leading term, and it is surfaced rather than
guessed at: `recall since 7d` is a valid search for the words "since" and
"7d", and also one `:` away from `recall since:7d`. The query runs as the
term search, and the response carries a warning naming both readings. The
shapes that warn — and the neighbouring ones that stay silent — are
catalogued in [Warnings](#warnings).

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
implements them: a query starting with either is rejected. In value position
they behave like every other keyword — `entity:update` is the word "update".

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
* Case is folded: an identifier is an identity, not prose, so `topic:Billing`,
  `topic:billing` and `topic:BILLING` are one anchor, stored as `billing`.
  Only the quoted fact of a remember keeps the case it was written with. An
  agent never has to remember how it capitalised something.
* A reserved keyword is a valid identifier in value position, unless a `:`
  follows it (see Keywords), and in any case there: `entity:Top` and
  `entity:top` are the same anchor. Where a keyword still reads as syntax — a
  non-leading recall term — no casing makes it an identifier: `Topic` is a
  mis-cased keyword and an error, and quoting (`recall food 'topic'`) is the
  escape.
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
(* The leading term is the one place a bare reserved word reads as *)
(* a term — no clause can start there. After it, a keyword starts  *)
(* a clause; quote it ('top') to search for the word itself.       *)
recall_body     = term+ field* ;
field           = anchor | modifier ;

term            = bare_word | phrase ;        (* soft ranking seed (SHOULD) *)

anchor          = anchor_field ;              (* every anchor filters; there is no way to say MUST_NOT *)
anchor_field    = topic_field | entity_field ;
topic_field     = 'topic'  ':' anchor_value ;
entity_field    = 'entity' ':' anchor_value ;
anchor_value    = identifier | quoted_identifier ; (* a reserved word qualifies, unless ':' follows it *)

modifier        = since_field | until_field | depth_field | top_field | vec_field ;
since_field     = 'since' ':' time_value ;     (* lower time bound; duration read as "ago" *)
until_field     = 'until' ':' time_value ;     (* upper time bound; duration read as "ago" *)
depth_field     = 'depth' ':' integer ;        (* retrieval lane: <2 BM25 floor, >=2 excess; ceiling 2 *)
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
identifier        = bare_word ;                (* folded to lower case on the way in *)
quoted_identifier = "'" { ?[^']? | "''" } "'" ; (* same opaque rule as phrase *)
integer           = ?[0-9]+? ;                 (* non-negative; '-' is its own token *)
time_value        = duration | iso_date ;
duration          = integer time_unit ;
time_unit         = 's' | 'm' | 'h' | 'd' | 'w' ;
iso_date          = ?\d{4}-\d{2}-\d{2}? ;         (* date only: ':' would end the token *)
param_ref         = '$' identifier ;
```

## Warnings

An error rejects a query; a warning accompanies one that ran. The bar for a
warning is deliberately high: the query must be valid with exactly one
reading, *and* sit one typo away from a different valid query — close enough
that a slip of a colon would change the results without changing the status
code. The hits are real and complete either way; the warning only says what
else the query could have meant.

Warnings travel beside the results, never instead of them, and the key is
absent when there is nothing to say — the response shape of a clean query is
unchanged:

```json
{
  "results": {"count": 1, "hits": ["..."]},
  "warnings": ["parse warning at column 12: term \"since\" is also a keyword: write since:<value> if a clause was meant, or quote it ('since') to search for the word"]
}
```

Each entry is positioned like a parse error — `parse warning at column N:`,
the column naming the last character of the token it is about — and names
both readings with the syntax that selects each one, so it can be resolved
from the message alone.

### Queries that warn

One shape warns: **a recall whose leading term spells a reserved keyword**
(any of `recall`, `remember`, `forget`, `update`, `topic`, `entity`,
`since`, `until`, `top`, `depth`, `vec`), in any casing. The leading term is
the one position where a bare reserved word legally reads as data — a recall
must start with a term, so no clause can begin there — which also makes it
the one position where a mistyped clause slips through as a search instead
of an error.

| query              | reading that runs              | near-miss it warns about        |
|--------------------|--------------------------------|---------------------------------|
| `recall since 7d`  | a search for "since" and "7d"  | `recall since:7d`, a time bound |
| `recall top`       | a search for "top"             | an unfinished `top:<n>` clause  |
| `recall Top shelf` | a search for "top" and "shelf" | the same, mis-cased             |

### Queries that stay silent

Every neighbouring shape resolves without ambiguity, so it carries no
warning — the grammar either runs it silently or rejects it outright:

* `recall 'since' 7d` — quoting the term states the intent, and is the way
  to silence the warning above.
* `recall x since:7d` — an actual clause is what it says it is.
* `entity:top`, `entity:Top` — value position: after a field's `:` only a
  value can appear, so there is nothing to mistake it for.
* `recall x since 7d` — clause position: a missing `:` is an error, not a
  warning; results scoped by nothing would be worse than either.
* `recall x Since 7d` — clause position, mis-cased: an error naming the
  casing.

The bar is meant to keep warnings rare and the list short: a shape joins it
only when it is a valid query one typo from a different valid query *and*
the grammar has no way to resolve which was meant. Anything the grammar can
settle is settled — as a parse, or as an error.

## Examples

```text
recall billing entity:acme since:7d top:5
recall billing topic:billing
recall 'annual contract' topic:billing entity:acme depth:1
recall@3 auth topic:auth entity:okta
remember 'acme moved to annual billing' topic:billing topic:contracts
remember 'acme signed with okta' topic:auth entity:okta
remember 'meeting at 3:30pm about the topic' topic:meetings   (* colons and reserved words are literal inside a phrase *)
remember 'she reached the top' topic:hiking entity:top       (* a keyword after a field's ':' is the word itself *)
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
