# Fraise design

This document discusses the components and overall architecture design of Fraise.

## Introduction

Fraise is designed to be a high-performance, in-memory database for AI agents contextual memory. Agents are given acess to a simple dsl that allows them to
store and retrieve contextual memory in real time.

## Context engineering

## Design

Fraise is an in-memory database with a complex query system flexible enough to store and retrieve contextual information. As mentioned earlier, memory is
only one of many parts of context, however, any long term memory storage can find its way in the model short term memory. A system that allows to keep track
and update existing memory.

This is done via sending `streams` to the database. A stream contains a single update to the `temporal memory graph`. A memory graph is made of knowledge:

* `facts`
* `entities`
* `relationships`

and a memory retrieval system. Facts, entites and relationships are also indexed for fast retrieval via graph, semantic and text representation. Those representations are gathered in indices for fast query. A federated query engine manages retrieval and ranking. The memory graph is refered to as temporal because short term memories are by default
deemed more relevant than older ones.

## Architecture

## Indices

## Query Engine

Fraise query language is an extension of Lucene

## Temporality

## References
