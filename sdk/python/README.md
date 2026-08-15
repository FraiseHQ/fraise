# fraise-sdk (Python)

A Python client for a [Fraise](../../README.md) memory server, plus ready-made
memory tools for agent frameworks.

## Install

```bash
pip install fraise-sdk                 # core client only
pip install 'fraise-sdk[openai]'       # + OpenAI Agents SDK tools
pip install 'fraise-sdk[anthropic]'    # + Claude Agent SDK tools
```

## Client

Two operations, both over the server's single query endpoint:

```python
from fraise_sdk import FraiseClient

with FraiseClient("http://localhost:9876") as fraise:
    fraise.remember("anne loves the color orange", topics=["color"], entities=["anne"])

    result = fraise.recall("anne", "color", top=5)
    for hit in result:
        print(hit.value, hit.score)
```

`recall` returns a `RecallResult` (`.count`, `.hits`, and it iterates/`len()`s
over the hits). Vector search is supported by passing an embedding:

```python
fraise.remember("the kingfisher is electric blue", graph=6, vector=embedding)
hits = fraise.recall("zzznomatch", graph=6, vector=embedding)  # seeded only by the vector
```

Anything the typed helpers do not cover is reachable through the raw
`fraise.query("recall@3 ...")` escape hatch.

## Embeddings (optional)

Give the client an **embedder** and it encodes text to a vector automatically —
`remember` embeds its value, `recall` embeds its query phrase (or its keywords):

```python
from fraise_sdk import FraiseClient
from fraise_sdk.providers import OpenAIEmbedder   # needs fraise-sdk[openai]

fraise = FraiseClient("http://localhost:9876", embedder=OpenAIEmbedder(dimensions=128))

fraise.remember("the kingfisher is electric blue", graph=6)          # stored with its vector
hits = fraise.recall("small bright bird", graph=6, query="small bright bird")
```

An embedder is anything implementing the `Embedder` ABC (subclass it and define
`embed(text) -> Sequence[float]`) or a plain `callable(text) -> Sequence[float]`,
so a lambda over your own model works too. Per call you can force it with
`embed=True`, skip it with `embed=False`, or override with an explicit `vector=`.
Only OpenAI is provided today — Anthropic has no embeddings API.

## OpenAI Agents tools

`memory_tools(client)` returns a `recall` and a `remember` `FunctionTool` bound
to one memory graph, so the agent decides *what* to store and retrieve:

```python
from agents import Agent, Runner
from fraise_sdk import FraiseClient
from fraise_sdk.integrations.openai_agents import memory_tools

fraise = FraiseClient("http://localhost:9876")
agent = Agent(
    name="Assistant",
    instructions="Remember durable facts the user shares, and recall them when relevant.",
    tools=memory_tools(fraise),
)

result = Runner.run_sync(agent, "My favourite colour is orange. Remember that.")
print(result.final_output)
```

Pass an embedder — `memory_tools(fraise, embedder=OpenAIEmbedder())` — to make
the tools vectorise implicitly: recall and remember encode their text through it
and carry the vector alongside.

See [`examples/openai-agents/`](../../examples/openai-agents) for a complete,
Docker-runnable script.

## Claude Agent SDK tools

The Claude Agent SDK groups tools into an in-process MCP server, so the entry
point is `memory_server(client)`; pair it with `allowed_tools()`:

```python
from claude_agent_sdk import ClaudeAgentOptions, ClaudeSDKClient
from fraise_sdk import FraiseClient
from fraise_sdk.integrations.claude_agents import memory_server, allowed_tools

fraise = FraiseClient("http://localhost:9876")
options = ClaudeAgentOptions(
    system_prompt="Remember durable facts the user shares, and recall them when relevant.",
    mcp_servers={"fraise_memory": memory_server(fraise)},
    allowed_tools=allowed_tools(),
)
```

`memory_server(fraise, embedder=OpenAIEmbedder())` makes the tools vectorise
implicitly, exactly as in the OpenAI integration.

See [`examples/claude-agent-sdk/`](../../examples/claude-agent-sdk) for a
complete, Docker-runnable script.

## Notes & limits

- A fact value is stored inside a single-quoted phrase where every character
  is literal; the SDK escapes apostrophes for you, so `remember("it's blue")`
  stores the text exactly as written.
- Keywords, topics, and entities are single whitespace-free tokens.
- The first vector written to a graph fixes that graph's embedding dimension;
  later writes to the same graph must match it.
- `FraiseClient` defaults to a 30s request timeout (`timeout=` on the
  constructor or on individual `query`/`remember`/`recall` calls overrides
  it); a request that exceeds it raises `FraiseError` naming the timeout,
  distinct from the error raised when the server can't be reached at all.
