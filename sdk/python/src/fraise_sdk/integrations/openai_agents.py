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

"""Built-in Fraise memory tools for the OpenAI Agents SDK.

`recall_tool` and `remember_tool` wrap a :class:`~fraise_sdk.client.FraiseClient`
as `FunctionTool`s an ``agents.Agent`` can call directly, and `memory_tools`
returns both at once::

    from agents import Agent
    from fraise_sdk import FraiseClient
    from fraise_sdk.integrations.openai_agents import memory_tools

    fraise = FraiseClient("http://localhost:9876")
    agent = Agent(
        name="Assistant",
        instructions="Remember useful facts and recall them when relevant.",
        tools=memory_tools(fraise),
    )

The tools are bound to one graph (default 0), so the agent decides *what* to
store and retrieve, never *where* — the memory partition is an application
concern, not something the model should pick.
"""

from __future__ import annotations

from fraise_sdk.client import FraiseClient
from fraise_sdk.errors import FraiseError
from fraise_sdk.providers import Embedder, EmbedderLike, resolve_embedder

try:
    from agents import FunctionTool, function_tool
except ImportError as exc:  # pragma: no cover - exercised only without the extra
    raise ImportError(
        "The OpenAI Agents integration requires the 'openai-agents' package. "
        "Install it with:  pip install 'fraise-sdk[openai]'"
    ) from exc

# Tool-call budgets: sane ceilings so the model does not have to reason about
# scale. Overridable per call by the model within the tool's own arguments.
_DEFAULT_TOP = 5
_DEFAULT_DEPTH = 2


def recall_tool(
    client: FraiseClient,
    *,
    graph: int = 0,
    embedder: Embedder | EmbedderLike | None = None,
    name: str = "recall_memory",
) -> FunctionTool:
    """Build a tool that searches long-term memory and returns matching facts.

    Pass an ``embedder`` and the recall implicitly vectorises: it encodes its
    keywords through the embedder and searches by that vector too. Omit it and
    the recall is keyword-only.
    """
    encode = resolve_embedder(embedder)

    def recall_memory(
        keywords: list[str], top: int = _DEFAULT_TOP, depth: int = _DEFAULT_DEPTH
    ) -> str:
        """Search long-term memory for facts related to the given keywords.

        Call this before answering when the user refers to something they may
        have told you earlier, or whenever prior context would help.

        Args:
            keywords: Salient words to search for — names, topics, or nouns from
                the question. Provide several for a broader search.
            top: Maximum number of facts to return, most relevant first.
            depth: How far to follow links between related facts (1 keeps only
                direct keyword matches; higher pulls in connected facts).

        """
        vector = encode(" ".join(keywords)) if encode and keywords else None
        try:
            result = client.recall(
                *keywords, graph=graph, top=top, depth=depth, vector=vector, embed=False
            )
        except FraiseError as exc:
            return f"memory lookup failed: {exc}"
        if not result.hits:
            return "No stored facts matched those keywords."
        return "\n".join(
            f"- {hit.value} (relevance {hit.score:.3f})" for hit in result.hits
        )

    return function_tool(
        recall_memory,
        name_override=name,
        description_override="Search the assistant's long-term memory for facts "
        "related to some keywords and return them ranked by relevance.",
    )


def remember_tool(
    client: FraiseClient,
    *,
    graph: int = 0,
    embedder: Embedder | EmbedderLike | None = None,
    name: str = "remember_fact",
) -> FunctionTool:
    """Build a tool that stores a single fact in long-term memory.

    Pass an ``embedder`` and the fact is implicitly vectorised: it is encoded
    through the embedder and stored with its vector. Omit it to store text only.
    """
    encode = resolve_embedder(embedder)

    def remember_fact(
        fact: str,
        topics: list[str] | None = None,
        entities: list[str] | None = None,
    ) -> str:
        """Store a fact in long-term memory so it can be recalled in later turns.

        Use this when the user shares something durable worth remembering — a
        preference, a name, a decision. Store one self-contained fact per call.

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

    return function_tool(
        remember_fact,
        name_override=name,
        description_override="Store a single self-contained fact in the "
        "assistant's long-term memory for later recall.",
    )


def memory_tools(
    client: FraiseClient,
    *,
    graph: int = 0,
    embedder: Embedder | EmbedderLike | None = None,
) -> list[FunctionTool]:
    """Return both memory tools (recall + remember) bound to one graph.

    Convenience for the common case: ``tools=memory_tools(fraise)``. Pass an
    ``embedder`` to make the tools vectorise implicitly — recall and remember
    then encode their text through it and carry the vector alongside.
    """
    return [
        recall_tool(client, graph=graph, embedder=embedder),
        remember_tool(client, graph=graph, embedder=embedder),
    ]
