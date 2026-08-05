# fraise-sdk (TypeScript)

A TypeScript client for a [Fraise](../../README.md) memory server, plus ready-made
memory tools for the OpenAI Agents SDK. The core client is dependency-free (it
uses the built-in `fetch`); the OpenAI pieces are optional peer dependencies.

## Install

```bash
npm install fraise-sdk                        # core client only
npm install fraise-sdk @openai/agents zod     # + OpenAI Agents tools
npm install fraise-sdk openai                 # + OpenAI embedder
```

## Client

Two operations, both over the server's single query endpoint. Everything is
async:

```ts
import { FraiseClient } from "fraise-sdk";

const fraise = new FraiseClient({ baseUrl: "http://localhost:9876" });

await fraise.remember("anne loves the color orange", { topics: ["color"], entities: ["anne"] });

const result = await fraise.recall(["anne", "color"], { top: 5 });
for (const hit of result.hits) console.log(hit.value, hit.score);
```

`recall(keywords, options)` returns a `RecallResult` (`{ count, hits }`).
Anything the typed helpers don't cover is reachable through the raw
`fraise.query("recall@3 ...")` escape hatch.

## Embeddings (optional)

Give the client an **embedder** and it encodes text to a vector automatically —
`remember` embeds its value, `recall` embeds its `query` phrase (or its keywords):

```ts
import { FraiseClient } from "fraise-sdk";
import { OpenAIEmbedder } from "fraise-sdk/providers";   // needs `openai`

const fraise = new FraiseClient({
  baseUrl: "http://localhost:9876",
  embedder: new OpenAIEmbedder({ dimensions: 128 }),
});

await fraise.remember("the kingfisher is electric blue", { graph: 6 });
const hits = await fraise.recall([], { graph: 6, query: "small bright bird" });
```

An embedder is anything extending the `Embedder` abstract class (implement
`embed(text): number[] | Promise<number[]>`) or a plain
`(text: string) => number[] | Promise<number[]>` function. Per call you can
force it with `embed: true`, skip it with `embed: false`, or override with an
explicit `vector`.

## OpenAI Agents tools

`memoryTools(client)` returns `recall` and `remember` tools for an
`@openai/agents` `Agent`:

```ts
import { Agent, run } from "@openai/agents";
import { FraiseClient } from "fraise-sdk";
import { memoryTools } from "fraise-sdk/integrations/openai-agents";
import { OpenAIEmbedder } from "fraise-sdk/providers";

const fraise = new FraiseClient({ baseUrl: "http://localhost:9876" });
const agent = new Agent({
  name: "Assistant",
  instructions: "Remember durable facts the user shares, and recall them when relevant.",
  tools: memoryTools(fraise, { embedder: new OpenAIEmbedder() }),
});

const result = await run(agent, "My favourite colour is orange. Remember that.");
console.log(result.finalOutput);
```

Pass an `embedder` to `memoryTools` to make the tools vectorise implicitly; omit
it for keyword-only memory. The tools bind to one graph (default 0).

## Notes & limits

- A fact value is stored inside a single-quoted phrase and the grammar has no
  escape for an apostrophe, so `remember("it's blue")` throws `FraiseQueryError`.
- Keywords, topics, and entities are single whitespace-free tokens.
- The first vector written to a graph fixes that graph's embedding dimension;
  later writes to the same graph must match it.
- Only the OpenAI integration/embedder ship today.
