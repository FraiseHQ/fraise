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

"""Built-in Fraise memory tools for Pydantic AI.

`recall_tool` and `remember_tool` wrap a :class:`~fraise_sdk.client.FraiseClient`
as Pydantic AI `Tool`s, which a ``pydantic_ai.Agent`` takes in ``tools=``.
`memory_tools` returns both at once::

    from pydantic_ai import Agent
    from fraise_sdk import FraiseClient
    from fraise_sdk.integrations.pydantic_ai import memory_tools

    fraise = FraiseClient("http://localhost:9876")
    agent = Agent(
        "openai:gpt-5-nano",
        instructions="Remember useful facts and recall them when relevant.",
        tools=memory_tools(fraise),
    )

Pydantic AI builds each tool's schema from its function's signature and
docstring, and validates the model's arguments against it. So the depth bound
below reaches the model, and a depth past it comes back to the model as a retry
without the server being called.

The tools are bound to one graph (default 0). So the agent decides *what* to
store and retrieve, never *where*. The memory partition is an application
concern, not something the model should pick.
"""

from __future__ import annotations

from typing import Annotated

from fraise_sdk.client import FraiseClient
from fraise_sdk.constants import DEFAULT_TOP, MAX_DEPTH, RECALL_TOOL, REMEMBER_TOOL
from fraise_sdk.errors import FraiseError
from fraise_sdk.integrations.render import render_hit
from fraise_sdk.providers import Embedder, EmbedderLike, resolve_embedder

try:
    from pydantic import Field
    from pydantic_ai import Tool
except ImportError as exc:  # pragma: no cover - exercised only without the extra
    raise ImportError(
        "The Pydantic AI integration requires the 'pydantic-ai-slim' package. "
        "Install it with:  pip install 'fraise-sdk[pydantic-ai]'"
    ) from exc


def recall_tool(
    client: FraiseClient,
    *,
    graph: int = 0,
    embedder: Embedder | EmbedderLike | None = None,
    name: str = RECALL_TOOL,
) -> Tool[None]:
    """Build a tool that searches long-term memory and returns matching facts.

    Pass an ``embedder`` and the recall vectorises implicitly. It encodes its
    keywords through the embedder and searches by that vector too. Omit it and
    the recall is keyword-only.
    """
    encode = resolve_embedder(embedder)

    def recall_memory(
        keywords: list[str],
        top: int = DEFAULT_TOP,
        depth: Annotated[int | None, Field(ge=0, le=MAX_DEPTH)] = None,
    ) -> str:
        """Search long-term memory for facts related to the given keywords.

        Args:
            keywords: Salient words to search for — names, topics, or nouns from
                the question. Provide several for a broader search.
            top: Maximum number of facts to return, most relevant first.
            depth: Retrieval lane, 0 to 2. 0 searches the text and vector
                indices only; 1 also lets topics and entities that clearly
                concentrate the matches pull in the facts filed under them; 2
                admits them at their fair share for maximum recall. The graph
                lanes need a topic or entity on the recall, which this tool
                does not name, so omit it and the server's default applies.

        """
        vector = encode(" ".join(keywords)) if encode and keywords else None
        try:
            result = client.recall(
                *keywords, graph=graph, top=top, depth=depth, vector=vector, embed=False
            )
        except FraiseError as exc:
            return f"memory lookup failed: {exc}"
        if result.empty:
            return "Nothing has been stored in memory yet."
        if not result.hits:
            return "No stored facts matched those keywords."
        return "\n".join(render_hit(hit) for hit in result.hits)

    return Tool(
        recall_memory,
        takes_ctx=False,
        name=name,
        description="Search the assistant's long-term memory for facts related "
        "to some keywords and return them ranked by relevance. Each fact comes "
        "back with its dates in front: when it was said and when what it states "
        "happened, as one date where the two are the same. Call this before "
        "answering when the user refers to something they told you earlier.",
    )


def remember_tool(
    client: FraiseClient,
    *,
    graph: int = 0,
    embedder: Embedder | EmbedderLike | None = None,
    name: str = REMEMBER_TOOL,
) -> Tool[None]:
    """Build a tool that stores a single fact in long-term memory.

    Pass an ``embedder`` and the fact is vectorised implicitly. The embedder
    encodes it, and the fact is stored with its vector. Omit it to store text
    only.
    """
    encode = resolve_embedder(embedder)

    def remember_fact(
        fact: str,
        topics: list[str] | None = None,
        entities: list[str] | None = None,
    ) -> str:
        """Store a fact in long-term memory so it can be recalled in later turns.

        Args:
            fact: A single, self-contained statement to remember.
            topics: Optional subject tags grouping related facts (e.g. "color",
                "travel"); facts sharing a topic become reachable together.
            entities: Optional named things the fact is about — a person, place,
                or object (e.g. "anne").

        """
        vector = encode(fact) if encode else None
        try:
            client.remember(
                fact,
                graph=graph,
                topics=topics,
                entities=entities,
                vector=vector,
                embed=False,
            )
        except FraiseError as exc:
            return f"could not store the fact: {exc}"
        return f"Stored: {fact}"

    return Tool(
        remember_fact,
        takes_ctx=False,
        name=name,
        description="Store a single self-contained fact in the assistant's "
        "long-term memory for later recall. Use it when the user shares "
        "something durable worth remembering — a preference, a name, a decision.",
    )


def memory_tools(
    client: FraiseClient,
    *,
    graph: int = 0,
    embedder: Embedder | EmbedderLike | None = None,
) -> list[Tool[None]]:
    """Return both memory tools (recall + remember) bound to one graph.

    It is a convenience for the common case: ``tools=memory_tools(fraise)``.
    Pass an ``embedder`` to make the tools vectorise implicitly. Recall and
    remember then encode their text through it and send the vector along.
    """
    return [
        recall_tool(client, graph=graph, embedder=embedder),
        remember_tool(client, graph=graph, embedder=embedder),
    ]
