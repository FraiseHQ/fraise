# OpenAI Agents (TypeScript) + Fraise memory

An [OpenAI Agents SDK](https://github.com/openai/openai-agents-js) agent (in
TypeScript) that stores and recalls facts through a Fraise server, using the
built-in memory tools from `fraise-sdk/integrations/openai-agents`.

The demo runs two turns as **separate agent runs with no shared history**, so the
second turn (`What is my favourite colour?`) can only succeed by recalling what
the first turn remembered. The tools are wired with an `OpenAIEmbedder`, so
memory **vectorises implicitly** — each fact is stored with its embedding and
recall searches by vector too.

## Run

Everything runs in Docker — the compose file builds and starts Fraise too, and
builds the local TypeScript SDK into the agent image:

```bash
export OPENAI_API_KEY=sk-...
docker compose run --rm agent
```

The `agent` service waits for Fraise's health check, then runs
[`agent.ts`](agent.ts) directly via Node's built-in TypeScript type-stripping
(`node --experimental-strip-types`). Tear down with `docker compose down`.

## What's here

- [`agent.ts`](agent.ts) — the agent and its two-turn demo.
- [`Dockerfile`](Dockerfile) — builds the local `fraise-sdk`, installs it plus `@openai/agents`, runs the script with Node's type-stripping.
- [`docker-compose.yaml`](docker-compose.yaml) — `fraise` + `agent` services on one network.
- [`fraise.config.toml`](fraise.config.toml) — Fraise server config mounted into the `fraise` service.

The Docker build context is the repository root so the image can build and
install the unpublished SDK from `sdk/typescript`.

## Running locally (without Docker)

Build the SDK, link it in, then start the agent against a running Fraise server:

```bash
(cd ../../sdk/typescript && pnpm install && pnpm run build)
pnpm install
pnpm add ../../sdk/typescript      # link the freshly built SDK
FRAISE_URL=http://localhost:9876 OPENAI_API_KEY=sk-... pnpm start
```
