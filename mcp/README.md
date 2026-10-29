# fraise-mcp

An MCP server for a running [Fraise](https://github.com/FraiseHQ/fraise) daemon. It exposes two tools, `recall` and `remember`, each taking one FQL statement, and embeds every statement's text locally so retrieval is hybrid (text and vector) out of the box. The model writes plain FQL and never a vector.

<!-- mcp-name: io.github.FraiseHQ/fraise -->

## Run

```bash
uvx fraise-mcp
```

Register it with your agent host:

```bash
claude mcp add fraise -- uvx fraise-mcp
codex mcp add fraise -- uvx fraise-mcp
```

The server forwards to a daemon that runs separately (`brew services start fraise`, or `systemctl --user start fraise`), like a database behind a connection URL.

## Configure

Every setting is a flag, and each flag falls back to an environment variable:

| Flag | Environment | Default |
| --- | --- | --- |
| `--addr` | `FRAISE_ADDR` | `http://localhost:9876` |
| `--model` | `FRAISE_EMBEDDING_MODEL` | `BAAI/bge-small-en-v1.5` |
| `--no-vectors` | `FRAISE_NO_VECTORS=1` | vectors on |

The model is any text model [fastembed](https://qdrant.github.io/fastembed/examples/Supported_Models/) supports. It runs on ONNX Runtime, with no torch, and loads on the first statement that needs a vector, so the server starts without it. The first load downloads the model files; `FASTEMBED_CACHE_PATH` sets where they are kept. A graph's first vector fixes its dimension, so a graph keeps the model it was first written with.

## What gets embedded

A `remember` embeds its fact. A `recall` embeds its search terms, the words and phrases before its first clause. The server appends `vec:$v` and binds the vector beside the statement. A recall with no terms, where anchors alone seed the search, is sent as written, and so is any statement the server cannot read that far: the daemon checks every statement, and its error messages come back to the model as tool errors.
