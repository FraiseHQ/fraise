# Query Language Specification

## Introduction

Fraise is designed to be queried by agents over and over again. The main decision was to create a simple query language allowing to retrieve information. The FQL is
designed for token economy while being LLM-friendly.

### Principles

Human DSL design optimizes for expressiveness and aestetic flexibility, this query language optimizes for simplicity and chooses to remove ambiguity whenever possibe: for an AI-agent, degrees of freedom are a potential source of failure. Here are a few design principles:

* the query language is exclusively written in lower case (except for string literals). Upper case detected are flagged as a warning and the expression converted to lowercase before execution.
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

Keywords are specific query instructions. They are reserved keywords.

|        Keyword        |           Usage           | Type      |
|-----------------------|---------------------------|-----------|
| REMEMBER              | remember_query            | Command   |
| RECALL                | recall_query              | Command   |
| OR                    | bool_op                   | Op        |
| AND                   | bool_op                   | Op        |
| NOT                   | bool_op                   | Op        |
| TOPIC                 | topic_field               | Field     |
| ENTITY                | entity_field              | Field     |
| SINCE                 | since_field               | Field     |
| UNTIL                 | until_field               | Field     |
| TOP                   | top_field                 | Field     |
| VEC                   | vec_field                 | Field     |
| DEPTH                 | depth_field               | Field     |

### Punctuation

### Identifiers

### Literals

### Field reference

## Grammar

```ebnf
(* ============================================================ *)
(* Fraise Query Language (FQL) — v0.1.0                         *)
(* ============================================================ *)

query           = recall_query | remember_query ;

(* ------------------------------------------------------------ *)
(* RECALL                                                       *)
(* ------------------------------------------------------------ *)

recall_query    = 'recall' graph_selector? recall_body ;

(* Canonical order: terms, then anchors, then modifiers.         *)
(* SEMANTIC RULE: at least one POSITIVE seed is required — a      *)
(* term, a 'vec', or a non-'-' anchor. A query whose only        *)
(* anchors are '-' (MUST_NOT) matches nothing and is rejected.   *)
recall_body     = term* anchor* modifiers ;

term            = bare_word | phrase ;        (* soft ranking seed (SHOULD) *)

anchor          = occur? anchor_field ;       (* at most one prefix; not composable *)
occur           = '+' | '-' | '~' ;           (* MUST | MUST_NOT | LOOSEN *)
anchor_field    = topic_field | entity_field ;
topic_field     = 'topic'  ':' anchor_value ;
entity_field    = 'entity' ':' anchor_value ;
anchor_value    = identifier | quoted_identifier ;

modifiers       = since_field? until_field? depth_field? top_field? vec_field? ;
since_field     = 'since' ':' time_value ;     (* lower time bound; duration read as "ago" *)
until_field     = 'until' ':' time_value ;     (* upper time bound; duration read as "ago" *)
depth_field     = 'depth' ':' integer ;        (* graph fact-hops, default 2 *)
top_field       = 'top'   ':' integer ;        (* result limit, default 10 *)
vec_field       = 'vec'   ':' param_ref ;      (* semantic seed (optional) *)

(* ------------------------------------------------------------ *)
(* REMEMBER                                                     *)
(* ------------------------------------------------------------ *)

(* Asserts a fact (phrase) and the topics it is about.           *)
(* Entities are extracted at ingestion, not asserted here.       *)
(* Occur prefixes (+ - ~) are NOT valid on remember.             *)
remember_query  = 'remember' graph_selector? phrase topic_assign+ ;
topic_assign    = 'topic' ':' anchor_value ;

(* ------------------------------------------------------------ *)
(* SHARED                                                       *)
(* ------------------------------------------------------------ *)

graph_selector  = '@' integer ;                (* memory graph index, default 0 *)

(* ------------------------------------------------------------ *)
(* LITERALS                                                     *)
(* ------------------------------------------------------------ *)

term              = ?[a-z0-9_][a-z0-9_\-]*? ;  (* must NOT start with '-' (see notes) *)
phrase            = '"' ?[^"]*? '"' ;
identifier        = ?[a-z][a-z0-9_\-]*? ;
quoted_identifier = '"' ?[^"]+? '"' ;
integer           = ?[0-9]+? ;
time_value        = duration | iso_date ;
duration          = integer time_unit ;
time_unit         = 's' | 'm' | 'h' | 'd' | 'w' ;
iso_date          = ?\d{4}-\d{2}-\d{2}(T\d{2}:\d{2}:\d{2}Z?)?? ;
param_ref         = '$' identifier ;
```

## Examples

```
recall billing +entity:acme since:7d top:5
recall ~topic:billing -entity:acme
recall "annual contract" topic:billing entity:acme depth:3
recall @3 +topic:auth +entity:okta
remember "acme moved to annual billing" topic:billing topic:contracts
```

## Glossary

* **fact**: a memory atomic element.
* **memory graph**: a collection of organised facts related to topics and / or entities and indexed for hybrid search (full text, semantic and graph)
* **field**: additional information provided to the search in order to find the right facts.
* **field value**: value of a field used to find a fact
* **term**: a word or sequence of words used to find a fact.
* **command**: task to perform by the engine: recall, remember.
* **instruction**: a full query sent to the database engine with a command, search terms and fields (can be used interchangeably with the term query).
