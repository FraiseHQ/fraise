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
query           = recall_query | remember_query ;


recall_query    = 'recall' graph_selector? recall_body ;

graph_selector  = '@' integer ;

(* A recall must contain at least one term OR one field clause. *)
recall_body     = term_list field_clause*
                | field_clause+ ;

term_list       = term+ ;
term            = bare_word | phrase ;
bare_word       = ?[a-z0-9_\-]+? ;
phrase          = '"' ?[^"]*? '"' ;

field_clause    = simple_field | group_field ;

simple_field    = topic_field
                | since_field
                | until_field
                | depth_field
                | top_field
                | v_field ;

(* Boolean grouping is permitted over topic_field in v0.1. *)
group_field     = '(' topic_field (bool_op topic_field)+ ')' ;
bool_op         = 'OR' | 'AND';

topic_field     = 'topic' ':' topic_value ;
topic_value     = identifier | quoted_identifier ;

since_field     = 'since' ':' time_value ;
until_field     = 'until' ':' time_value ;
time_value      = duration | iso_date ;

depth_field    = 'depth' ':' integer ;     (* graph hop depth, default 2 *)
top_field      = 'top' ':' integer ;     (* result limit, default 10 *)
vec_field        = 'vec' ':' param_ref ;   (* nearest-neighbor (semantic) *)

(* ============================================================ *)
(* REMEMBER                                                     *)
(* ============================================================ *)

remember_query  = 'remember' graph_selector? phrase topic_field+ ;


(* ============================================================ *)
(* LITERALS                                                     *)
(* ============================================================ *)

identifier        = ?[a-z][a-z0-9_\-]*? ;
quoted_identifier = '"' ?[^"]+? '"' ;
integer           = ?[0-9]+? ;
duration          = integer time_unit ;
time_unit         = 's' | 'm' | 'h' | 'd' | 'w' ;
iso_date          = ?\d{4}-\d{2}-\d{2}(T\d{2}:\d{2}:\d{2}Z?)?? ;
param_ref         = '$' identifier ;
```

## Glossary

* **fact**: a memory atomic element.
* **memory graph**: a collection of organised facts related to topics and indexed for hybrid search (full text, semantic and graph)
* **field**: additional information provided to the search in order to find the right facts.
* **field value**: value of a field used to find a fact
* **term**: a word or sequence of words used to find a fact.
* **command**: task to perform by the engine: recall, remember, forget, update
* **instruction**: a full query sent to the database engine with a command, search terms and fields (can be used interchangeably with the term query).
