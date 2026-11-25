# Pydantic AI + Fraise memory

A [Pydantic AI](https://ai.pydantic.dev/) agent that stores and recalls facts through a Fraise server, using the built-in memory tools from `fraise_sdk.integrations.pydantic_ai`.

The demo runs two turns as **separate agent runs with no shared message history**, so the second turn (`What is my cat called?`) can only succeed by recalling what the first turn remembered. The same two turns then run as a **control**, on an identical agent without the memory tools, which cannot answer: the name came from Fraise, not from the context. The script exits non-zero if either run turns out otherwise.

The example is keyword-only. Pass an embedder — `memory_tools(fraise, embedder=OpenAIEmbedder())` in [`agent.py`](agent.py) — to make memory vectorise implicitly: each fact is stored with its embedding and recall searches by vector too.

## Run

Everything runs in Docker — the compose file builds and starts Fraise too:

```bash
export OPENAI_API_KEY=sk-...
docker compose run --rm agent
```

The `agent` service waits for Fraise's health check, then runs [`agent.py`](agent.py). Tear down with `docker compose down`.

## What's here

- [`agent.py`](agent.py) — the agent, its two-turn demo and the control run.
- [`Dockerfile`](Dockerfile) — installs the local `fraise-sdk[pydantic-ai]`, Pydantic AI's `openai` extra for the model, and the script.
- [`docker-compose.yaml`](docker-compose.yaml) — `fraise` + `agent` services on one network.
- [`fraise.config.toml`](fraise.config.toml) — Fraise server config mounted into the `fraise` service.

The Docker build context is the repository root so the image can install the SDK from `sdk/python` in this checkout rather than from PyPI.
