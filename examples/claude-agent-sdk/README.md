# Claude Agent SDK + Fraise memory

A [Claude Agent SDK](https://github.com/anthropics/claude-agent-sdk-python) agent
that stores and recalls facts through a Fraise server. Fraise's memory tools are
exposed as an **in-process MCP server** via
`fraise_sdk.integrations.claude_agents`.

The demo runs two turns as **separate client sessions with no shared history**,
so the second turn (`What is my favourite colour?`) can only succeed by recalling
what the first turn remembered.

## Run

Everything runs in Docker — the compose file builds and starts Fraise too:

```bash
export ANTHROPIC_API_KEY=sk-ant-...
docker compose run --rm agent
```

The `agent` service waits for Fraise's health check, then runs [`agent.py`](agent.py).
Tear down with `docker compose down`.

## What's here

- [`agent.py`](agent.py) — the agent and its two-turn demo.
- [`Dockerfile`](Dockerfile) — installs Node + the `claude` CLI, the local `fraise-sdk[anthropic]`, and the script.
- [`docker-compose.yaml`](docker-compose.yaml) — `fraise` + `agent` services on one network.
- [`fraise.config.toml`](fraise.config.toml) — Fraise server config mounted into the `fraise` service.

## Note on the Claude Code CLI

The Claude Agent SDK drives Claude by spawning the Claude Code CLI, so the image
installs Node.js and `@anthropic-ai/claude-code` (providing the `claude` binary)
alongside Python. This is the main difference from the OpenAI Agents example,
whose SDK calls the API directly.
