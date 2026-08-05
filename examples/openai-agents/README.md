# OpenAI Agents + Fraise memory

An [OpenAI Agents SDK](https://github.com/openai/openai-agents-python) agent that
stores and recalls facts through a Fraise server, using the built-in memory tools
from `fraise_sdk.integrations.openai_agents`.

The demo runs two turns as **separate agent runs with no shared history**, so the
second turn (`What is my favourite colour?`) can only succeed by recalling what
the first turn remembered.

The tools are wired with an `OpenAIEmbedder`, so memory **vectorises implicitly**:
each fact is stored with its embedding and recall searches by vector too. Remove
the `embedder=...` argument in [`agent.py`](agent.py) for plain keyword memory.

## Run

Everything runs in Docker — the compose file builds and starts Fraise too:

```bash
export OPENAI_API_KEY=sk-...
docker compose run --rm agent
```

The `agent` service waits for Fraise's health check, then runs [`agent.py`](agent.py).
Tear down with `docker compose down`.

## What's here

- [`agent.py`](agent.py) — the agent and its two-turn demo.
- [`Dockerfile`](Dockerfile) — installs the local `fraise-sdk[openai]` and the script.
- [`docker-compose.yaml`](docker-compose.yaml) — `fraise` + `agent` services on one network.
- [`fraise.config.toml`](fraise.config.toml) — Fraise server config mounted into the `fraise` service.

The Docker build context is the repository root so the image can install the
local, unpublished SDK from `sdk/python`.
