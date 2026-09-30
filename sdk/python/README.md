# fraise-sdk (Python)

A Python client for a [Fraise](../../README.md) memory server, plus ready-made memory tools for agent frameworks.

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

`recall` returns a `RecallResult` (`.count`, `.hits`, and it iterates/`len()`s over the hits). Vector search is supported by passing an embedding:

```python
fraise.remember("the kingfisher is electric blue", graph=6, vector=embedding)
hits = fraise.recall(graph=6, vector=embedding)  # seeded only by the vector
```

A recall needs a seed, not a keyword: a vector or a `topics`/`entities` filter is one on its own, so `fraise.recall(topics=["birds"])` — everything about a topic — is a query in its own right.

The typed helpers take any value and write the FQL for you: a topic, entity or keyword is quoted whenever it is not letters and digits only, such as `e-mail` or `new york`, and a keyword is also quoted when it spells a reserved word such as `since` — with a `FraiseWarning`, since `recall("since", "7d")` may have meant a `since:7d` bound rather than the word. Anything they do not cover, such as a `since:`/`until:` time bound, goes through the raw `fraise.query("recall@3 ...")` escape hatch, which sends the FQL exactly as written and leaves the checking to the server.

Some queries run but carry something that cannot help them — a keyword that is an English stop word, e.g. `fraise.recall("ferry", "the")`: stored facts never contain "the", so it cannot match. The server answers them and attaches a warning; the SDK lists it on `result.warnings` and re-emits it as a `FraiseWarning`:

```python
import warnings
from fraise_sdk import FraiseWarning

warnings.filterwarnings("ignore", category=FraiseWarning)  # silence wholesale
```

The query shapes that warn (and the neighbouring ones that stay silent) are catalogued in [Warnings](https://docs.getfraise.dev/docs/query-language/reference/warnings).

## Embeddings (optional)

Give the client an **embedder** and it encodes text to a vector automatically — `remember` embeds its value, `recall` embeds its query phrase (or its keywords):

```python
from fraise_sdk import FraiseClient
from fraise_sdk.providers import OpenAIEmbedder   # needs fraise-sdk[openai]

fraise = FraiseClient("http://localhost:9876", embedder=OpenAIEmbedder(dimensions=128))

fraise.remember("the kingfisher is electric blue", graph=6)          # stored with its vector
hits = fraise.recall(query="small bright bird", graph=6)
```

An embedder is anything implementing the `Embedder` ABC (subclass it and define `embed(text) -> Sequence[float]`) or a plain `callable(text) -> Sequence[float]`, so a lambda over your own model works too. Per call you can force it with `embed=True`, skip it with `embed=False`, or override with an explicit `vector=`. Two ship ready-made: `OpenAIEmbedder` (`fraise-sdk[openai]`) and `HuggingFaceEmbedder` (`fraise-sdk[huggingface]`); Anthropic has no embeddings API.

## Extraction (optional)

Give the client an **extractor** and `remember` files each fact under the topics and entities it is about, without you choosing them:

```python
from fraise_sdk import FraiseClient
from fraise_sdk.providers import OpenAIExtractor   # needs fraise-sdk[openai]

fraise = FraiseClient("http://localhost:9876", extractor=OpenAIExtractor())

fraise.remember("Anne's flight lands at Lisbon airport")   # stored verbatim, anchored for you
```

An extractor is anything implementing the `Extractor` ABC (subclass it and define `extract(text) -> list[Anchor]`) or a plain `callable(text) -> list[Anchor]`, where an `Anchor` has a `value` and a `type`, `"topic"` or `"entity"`. Extracted anchors are added after any `topics=`/`entities=` you pass, without repeating one you gave, and the text itself is never rewritten. If extraction fails, the fact is stored anyway under the anchors you gave, with a `FraiseWarning`. Per call, `extract=True` requires an extractor and `extract=False` skips it. `OpenAIExtractor` (`gpt-5-mini` by default) uses the same instructions the published benchmark ingests with.

## OpenAI Agents tools

`memory_tools(client)` returns two `FunctionTool`s, `recall_memory` and `remember_fact`, bound to one memory graph, so the agent decides *what* to store and retrieve:

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

Pass an embedder — `memory_tools(fraise, embedder=OpenAIEmbedder())` — to make the tools vectorise implicitly: recall and remember encode their text through it and carry the vector alongside.

See [`examples/openai-agents/`](../../examples/openai-agents) for a complete, Docker-runnable script.

## Claude Agent SDK tools

The Claude Agent SDK groups tools into an in-process MCP server, so the entry point is `memory_server(client)`; pair it with `allowed_tools()`:

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

`memory_server(fraise, embedder=OpenAIEmbedder())` makes the tools vectorise implicitly, exactly as in the OpenAI integration.

See [`examples/claude-agent-sdk/`](../../examples/claude-agent-sdk) for a complete, Docker-runnable script.

## Notes & limits

- A fact value is stored inside a single-quoted phrase where every character is literal; the SDK escapes apostrophes for you, so `remember("it's blue")` stores the text exactly as written.
- Keywords, topics, and entities may contain spaces and punctuation: a value that is not one plain word (letters and digits only) is quoted for you, and so is a keyword that spells a reserved word.
- The first vector written to a graph fixes that graph's embedding dimension; later writes to the same graph must match it.
- `FraiseClient` defaults to a 30s request timeout (`timeout=` on the constructor or on individual `query`/`remember`/`recall` calls overrides it); a request that exceeds it raises `FraiseError` naming the timeout, distinct from the error raised when the server can't be reached at all.
