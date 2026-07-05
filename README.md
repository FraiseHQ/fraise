<p align="center">
  <img height="300px" style="height:300px;" src="assets/logo.png">
</p>

# Fraise

[![Discord](https://img.shields.io/discord/1523303330326253759?logo=discord&logoColor=white&label=discord&color=5865F2)](https://discord.gg/eHDFwnwHq)

Fraise is an in-memory memory store for AI agents. A real database your agent queries directly, with a language built for tokens, not humans.

## Why Fraise

- **Agent-native query language (FQL).** Two verbs: `remember` and `recall` designed for token economy and zero ambiguity. Less surface area for an LLM to get wrong.
- **Hybrid retrieval.** Every fact is indexed for full-text, semantic, and graph search. A hybrid engine ranks across all three.
- **Temporal by default.** Recent memories outrank older ones, so recall is recency-aware out of the box.
- **In-memory.** Real-time reads and writes, mid-step, while the user waits.
- **Open source, MIT.**

## How it works

Fraise stores knowledge as a **temporal memory graph** of facts, entities, and relationships. Updates arrive as **streams** (one update per stream). Each fact is indexed for full-text and vector search; a hybrid query engine handles retrieval and ranking. A database holds multiple memory graphs (default: 8).

## Get Started

This section is a short tutorial on how to run fraise locally or with a docker container and integrate with popular
agentic AI libraries.

### Install from Source

### Install with Docker

### Integrate with Claude Agents

### Integrate with OpenAI Agents

## References

To learn more about fraise, you can consult the following resources:

* [Roadmap](https://github.com/users/RonsenbergVI/projects/2)
* [Database design](./docs/design.md)
* [Query language specs](./docs/query-spec.md)
* [Issues](https://github.com/RonsenbergVI/fraise/issues)

## Contributing

## Code of Conduct

## Community

Questions, ideas, or building something with Fraise? Join the
[Discord](https://discord.gg/eHDFwnwHq). Bugs and feature requests belong in
[issues](https://github.com/RonsenbergVI/fraise/issues).

## License
