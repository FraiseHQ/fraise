# MIT License

# Copyright (c) 2026 René-Jean Corneille

# Permission is hereby granted, free of charge, to any person obtaining a copy
# of this software and associated documentation files (the "Software"), to deal
# in the Software without restriction, including without limitation the rights
# to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
# copies of the Software, and to permit persons to whom the Software is
# furnished to do so, subject to the following conditions:

# The above copyright notice and this permission notice shall be included in all
# copies or substantial portions of the Software.

# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
# FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
# AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
# LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
# OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
# SOFTWARE.

"""Built-in Fraise memory tools for the Claude Agent SDK.

Where the OpenAI integration hands the framework loose ``FunctionTool``s, the Claude
Agent SDK groups tools into an *in-process MCP server*. So the entry point here
is `memory_server`, which returns a server ready to drop into
``ClaudeAgentOptions.mcp_servers``; `recall_tool`/`remember_tool` expose the
individual `SdkMcpTool`s for callers who assemble their own server.

    from claude_agent_sdk import ClaudeAgentOptions, ClaudeSDKClient
    from fraise_sdk import FraiseClient
    from fraise_sdk.integrations.claude_agents import memory_server, allowed_tools

    fraise = FraiseClient("http://localhost:9876")
    server = memory_server(fraise)
    options = ClaudeAgentOptions(
        mcp_servers={"fraise_memory": server},
        allowed_tools=allowed_tools(),
    )

As with the OpenAI integration, the tools are bound to one memory graph (default 0),
so the model decides *what* to store and recall, never *where*.
"""

from __future__ import annotations

from typing import Any

from fraise_sdk.client import FraiseClient
from fraise_sdk.errors import FraiseError
from fraise_sdk.providers import Embedder, EmbedderLike, resolve_embedder

try:
    from claude_agent_sdk import (
        McpSdkServerConfig,
        SdkMcpTool,
        create_sdk_mcp_server,
        tool,
    )
except ImportError as exc:  # pragma: no cover - exercised only without the extra
    raise ImportError(
        "The Claude Agent SDK integration requires the 'claude-agent-sdk' package. "
        "Install it with:  pip install 'fraise-sdk[anthropic]'"
    ) from exc

# The name the memory server registers under. Tool identifiers Claude sees are
# namespaced as ``mcp__<server>__<tool>``, so this drives both the mcp_servers
# key and the allowed_tools entries — keep them in sync via `allowed_tools`.
DEFAULT_SERVER_NAME = "fraise_memory"
RECALL_TOOL = "recall_memory"
REMEMBER_TOOL = "remember_fact"

# Tool-call budgets: sane ceilings so the model need not reason about scale.
_DEFAULT_TOP = 5
_DEFAULT_DEPTH = 2


def _ok(text: str) -> dict[str, Any]:
    return {"content": [{"type": "text", "text": text}]}


def _err(text: str) -> dict[str, Any]:
    return {"content": [{"type": "text", "text": text}], "is_error": True}


def recall_tool(
    client: FraiseClient,
    *,
    graph: int = 0,
    embedder: Embedder | EmbedderLike | None = None,
) -> SdkMcpTool[Any]:
    """Build the recall tool: search long-term memory and return matching facts.

    Pass an ``embedder`` and the recall implicitly vectorises: it encodes its
    keywords through the embedder and searches by that vector too. Omit it and
    the recall is keyword-only.
    """
    encode = resolve_embedder(embedder)

    @tool(
        RECALL_TOOL,
        "Search the assistant's long-term memory for facts related to some "
        "keywords and return them ranked by relevance. Call this before "
        "answering when the user refers to something they told you earlier.",
        # Full JSON Schema (not the {name: type} shorthand) so top/depth stay
        # optional — the shorthand marks every property required.
        {
            "type": "object",
            "properties": {
                "keywords": {
                    "type": "array",
                    "items": {"type": "string"},
                    "description": "Salient words to search for — names, topics, or "
                    "nouns from the question. Provide several for a broader search.",
                },
                "top": {
                    "type": "integer",
                    "description": "Maximum number of facts to return, most relevant first.",
                },
                "depth": {
                    "type": "integer",
                    "description": "How far to follow links between related facts (1 "
                    "keeps only direct keyword matches; higher pulls in connected facts).",
                },
            },
            "required": ["keywords"],
        },
    )
    async def recall_memory(args: dict[str, Any]) -> dict[str, Any]:
        keywords = args.get("keywords") or []
        top = args.get("top") or _DEFAULT_TOP
        depth = args.get("depth") or _DEFAULT_DEPTH
        vector = encode(" ".join(keywords)) if encode and keywords else None
        try:
            result = client.recall(
                *keywords, graph=graph, top=top, depth=depth, vector=vector, embed=False
            )
        except FraiseError as exc:
            return _err(f"memory lookup failed: {exc}")
        if not result.hits:
            return _ok("No stored facts matched those keywords.")
        return _ok(
            "\n".join(
                f"- {hit.value} (relevance {hit.score:.3f})" for hit in result.hits
            )
        )

    return recall_memory


def remember_tool(
    client: FraiseClient,
    *,
    graph: int = 0,
    embedder: Embedder | EmbedderLike | None = None,
) -> SdkMcpTool[Any]:
    """Build the remember tool: store a single fact in long-term memory.

    Pass an ``embedder`` and the fact is implicitly vectorised: it is encoded
    through the embedder and stored with its vector. Omit it to store text only.
    """
    encode = resolve_embedder(embedder)

    @tool(
        REMEMBER_TOOL,
        "Store a single self-contained fact in the assistant's long-term memory "
        "for later recall. Use it when the user shares something durable worth "
        "remembering — a preference, a name, a decision.",
        # Full JSON Schema so only `fact` is required; topics/entities are optional.
        {
            "type": "object",
            "properties": {
                "fact": {
                    "type": "string",
                    "description": "A single, self-contained statement to remember. "
                    "Must not contain an apostrophe (').",
                },
                "topics": {
                    "type": "array",
                    "items": {"type": "string"},
                    "description": "Optional subject tags grouping related facts (e.g. "
                    "'color', 'travel'); facts sharing a topic become reachable together.",
                },
                "entities": {
                    "type": "array",
                    "items": {"type": "string"},
                    "description": "Optional named things the fact is about — a person, "
                    "place, or object (e.g. 'anne').",
                },
            },
            "required": ["fact"],
        },
    )
    async def remember_fact(args: dict[str, Any]) -> dict[str, Any]:
        fact = args.get("fact", "")
        vector = encode(fact) if encode and fact else None
        try:
            client.remember(
                fact,
                graph=graph,
                topics=args.get("topics"),
                entities=args.get("entities"),
                vector=vector,
                embed=False,
            )
        except FraiseError as exc:
            return _err(f"could not store the fact: {exc}")
        return _ok(f"Stored: {fact}")

    return remember_fact


def memory_tools(
    client: FraiseClient,
    *,
    graph: int = 0,
    embedder: Embedder | EmbedderLike | None = None,
) -> list[SdkMcpTool[Any]]:
    """Return both memory tools (recall + remember) bound to one graph.

    Pass an ``embedder`` to make the tools vectorise implicitly — recall and
    remember then encode their text through it and carry the vector alongside.
    """
    return [
        recall_tool(client, graph=graph, embedder=embedder),
        remember_tool(client, graph=graph, embedder=embedder),
    ]


def memory_server(
    client: FraiseClient,
    *,
    graph: int = 0,
    embedder: Embedder | EmbedderLike | None = None,
    name: str = DEFAULT_SERVER_NAME,
    version: str = "0.0.1",
) -> McpSdkServerConfig:
    """Bundle the memory tools into an in-process MCP server.

    Drop the result into ``ClaudeAgentOptions.mcp_servers`` under ``name`` and
    pair it with ``allowed_tools=allowed_tools(name)``. Pass an ``embedder`` to
    make the tools vectorise implicitly.
    """
    return create_sdk_mcp_server(
        name=name,
        version=version,
        tools=memory_tools(client, graph=graph, embedder=embedder),
    )


def allowed_tools(server_name: str = DEFAULT_SERVER_NAME) -> list[str]:
    """The fully-qualified tool names to pass to ``ClaudeAgentOptions.allowed_tools``.

    MCP tools are namespaced ``mcp__<server>__<tool>``; this returns both memory
    tools under ``server_name`` so the two never drift apart.
    """
    return [
        f"mcp__{server_name}__{RECALL_TOOL}",
        f"mcp__{server_name}__{REMEMBER_TOOL}",
    ]
