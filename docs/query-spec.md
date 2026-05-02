# Query Language Specification

## Introduction

Fraise is designed to be queried by agents over and over again. The main decision was to create a simple query language allowing to retrieve information. The FQL is
designed for token economy while being LLM-friendly.

This query language is designed to be minimalist, with a simple syntax. Here are a few constraints:

* the query language is exclusively written in lower case (except for string literals). Upper case detected are flagged as a warning and the expression converted to lowercase before execution.
* the query language only allows to run one command at a time. Multiple commands in a single instructions leads to a runtime error.
* the query language only allows for single line instructions. Multi-line instructions
are converted to a single line and executed if valid.

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
| REMEMBER              |                           | Command   |
| RECALL                |                           | Command   |
| UPDATE                |                           | Command   |
| FORGET                |                           | Command   |
| OR                    |                           | Op        |
| AND                   |                           | Op        |
| NOT                   |                           | Op        |
| TOPIC                 |                           | Field     |
| SINCE                 |                           | Field     |
| UNTIL                 |                           | Field     |
| TOP                   |                           | Field     |
| VEC                   |                           | Field     |
| DEPTH                 |                           | Field     |

### Punctuation

### Identifiers

### Field reference

## Grammar

```ebnf
query           = recall_query | remember_query ;


recall_query    = 'recall' recall_body ;

(* A recall must contain at least one term OR one field clause. *)
recall_body     = term_list field_clause*
                | field_clause+ ;
```

## Design decisions
