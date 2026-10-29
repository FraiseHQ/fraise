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
"""The MCP server: the recall and remember tools over a Fraise client.

Each tool takes one FQL statement and sends it to the daemon through
:meth:`fraise_sdk.client.FraiseClient.query`. The daemon parses, plans and
runs it. When the statement takes a vector (see
:func:`fraise_mcp.statement.embedding_text`), the server encodes its text,
appends ``vec:$v`` and binds the vector out of band. So the tools carry
hybrid retrieval without the model ever writing a vector. The daemon's error
messages come back to the model as tool errors, so it can read them and
correct its statement.
"""

from __future__ import annotations

from typing import Annotated

from fraise_sdk.client import FraiseClient
from fraise_sdk.constants import VECTOR_PARAM
from fraise_sdk.errors import FraiseAPIError, FraiseError
from fraise_sdk.models import RecallResult
from fraise_sdk.providers.base import EmbedderLike
from mcp.server.mcpserver import MCPServer
from mcp.server.mcpserver.exceptions import ToolError
from pydantic import Field

from fraise_mcp.statement import embedding_text, with_vector

RECALL_DESCRIPTION = (
    "Search the agent's long-term memory: run an FQL recall against the fraise "
    "daemon and return the matching facts ranked by relevance and recency. "
    "Anchors alone — topic:/entity: with no search terms — seed the search with "
    "every fact filed under them, scored one unit per named anchor and decayed by "
    "age, newest first under a single anchor: the way to see what is stored under "
    "a topic before you know what to search for."
)

RECALL_QUERY = (
    "A full FQL recall: search terms, then optional topic:/entity: anchors (beside "
    "terms they lift the facts filed under them through the graph and filter "
    "nothing, so the recall runs at depth:1 at least), top:N result cap, depth:0-2 "
    "retrieval lane (the graph is searched only beside a topic:/entity:), "
    "since:/until: time bounds (a date is quoted: since:'2026-01-15'). A bare term "
    "or value is letters and digits only; quote anything else: 'e-mail', "
    "topic:'machine-learning'. A search term that spells a keyword (top, since, "
    "topic...) is quoted too: 'since'. Example: recall@2 barometer storm "
    "topic:weather top:5 depth:1 since:7d. Anchors alone, with no terms, seed the "
    "search with every fact filed under those topics/entities, scored one unit per "
    "named anchor and decayed by age, so newest first under one anchor (depth: has "
    "no effect). Example: recall@2 topic:weather top:20"
)

REMEMBER_DESCRIPTION = (
    "Store a self-contained fact in the agent's long-term memory: run an FQL "
    "remember against the fraise daemon."
)

REMEMBER_QUERY = (
    "A full FQL remember: the fact in single quotes, anchors as repeatable "
    "topic:/entity: pairs, @N selecting the graph. An anchor value that is not "
    "letters and digits only is quoted too: topic:'machine-learning'. Example: "
    "remember@2 'the barometer falls before the storm' topic:weather entity:harbour"
)


def render(body: dict) -> str:
    """Render a query response for the model.

    A write is acknowledged with a status token, so its confirmation is fixed
    text. A recall is one line per hit, best first, relevance to three
    decimals, in the voice of the SDK's framework tools. A recall can come back
    with nothing in two ways. Either the graph holds nothing at all, which the
    daemon answers 204 with no body, or a populated graph has no matching
    fact. They read differently on purpose: the first calls for remembering
    something, the second for asking differently. Warnings follow, one per
    line. The statement ran, and the model needs them to fix its next one.

    Args:
        body: the decoded response body, ``{}`` for a 204.

    Returns:
        The text the model reads.
    """
    if "status" in body:
        lines = ["Remembered."]
    else:
        result = RecallResult.from_json(
            body.get("results") or {}, empty="results" not in body
        )
        lines = [f"- {hit.value} (relevance {hit.score:.3f})" for hit in result]
        if not result and result.empty:
            lines = [
                "No stored facts matched: this graph is empty, nothing has been "
                "remembered in it yet."
            ]
        elif not result:
            lines = ["No stored facts matched."]
    lines += [f"warning: {warning}" for warning in body.get("warnings") or []]
    return "\n".join(lines)


def build_server(
    client: FraiseClient, embedder: EmbedderLike | None, *, version: str = ""
) -> MCPServer:
    """Build the MCP server and register its recall and remember tools.

    Args:
        client: the client the statements are sent through.
        embedder: what encodes a statement's text into its vector, or ``None``
            to send every statement on text alone.
        version: the version the server reports to its MCP client.

    Returns:
        The server, ready to run over stdio.
    """
    server = MCPServer(name="fraise", version=version)

    def run(statement: str) -> str:
        text = embedding_text(statement) if embedder is not None else None
        parameters = None
        if embedder is not None and text is not None:
            try:
                vector = [float(x) for x in embedder(text)]
            except Exception as exc:
                # The model loads, and downloads the first time, on this call.
                # Whatever stops it is the operator's to fix, so it is said in
                # words rather than as an anonymous crash.
                raise ToolError(f"could not embed the statement: {exc}") from exc
            parameters = {VECTOR_PARAM: vector}
            statement = with_vector(statement)
        try:
            body = client.query(statement, parameters=parameters)
        except FraiseAPIError as exc:
            raise ToolError(exc.message) from exc
        except FraiseError as exc:
            raise ToolError(str(exc)) from exc
        return render(body)

    def recall(query: Annotated[str, Field(description=RECALL_QUERY)]) -> str:
        return run(query)

    def remember(query: Annotated[str, Field(description=REMEMBER_QUERY)]) -> str:
        return run(query)

    server.add_tool(recall, description=RECALL_DESCRIPTION, structured_output=False)
    server.add_tool(remember, description=REMEMBER_DESCRIPTION, structured_output=False)
    return server
