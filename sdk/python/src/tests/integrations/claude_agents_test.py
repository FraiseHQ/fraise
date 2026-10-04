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

"""Claude Agent SDK tool tests against a mocked client — no server, no model calls.

Unlike the OpenAI tools, these return MCP content payloads rather than plain
strings and flag a failure with ``is_error``, so the assertions check that
envelope as well as the text.
"""

import pytest
from fraise_sdk.constants import DEFAULT_SERVER_NAME
from fraise_sdk.errors import FraiseError
from fraise_sdk.models import Hit

# The integration imports its framework at module scope, so skip the whole file
# when the optional 'anthropic' extra is not installed.
pytest.importorskip(
    "claude_agent_sdk", reason="requires the 'anthropic' dependency group"
)

from fraise_sdk.integrations.claude_agents import (  # noqa: E402
    allowed_tools,
    memory_server,
    memory_tools,
    recall_tool,
    remember_tool,
)


def test_memory_tools_returns_both_tools(mock_client):
    """memory_tools returns both tools, recall first."""
    tools = memory_tools(mock_client())
    assert [tool.name for tool in tools] == ["recall_memory", "remember_fact"]


def test_allowed_tools_matches_the_registered_tool_names():
    """allowed_tools names the tools as Claude checks them, ``mcp__<server>__<tool>``.

    A name that drifts from the tool's own leaves the tool silently uncallable.
    """
    assert allowed_tools() == [
        f"mcp__{DEFAULT_SERVER_NAME}__recall_memory",
        f"mcp__{DEFAULT_SERVER_NAME}__remember_fact",
    ]


def test_allowed_tools_follows_a_custom_server_name():
    """A custom server name moves both tool names under its namespace."""
    assert allowed_tools("other") == [
        "mcp__other__recall_memory",
        "mcp__other__remember_fact",
    ]


def test_memory_server_builds_with_both_tools():
    """The server is built under the default name with both memory tools.

    The name is what ``allowed_tools`` namespaces the tools under, so a server
    registered under any other name leaves both tools uncallable; and a server
    missing either tool loses that half of the memory.
    """
    with patch("fraise_sdk.integrations.claude_agents.create_sdk_mcp_server") as create:
        server = memory_server(_client())
    create.assert_called_once()
    assert server is create.return_value
    kwargs = create.call_args.kwargs
    assert kwargs["name"] == DEFAULT_SERVER_NAME
    assert [tool.name for tool in kwargs["tools"]] == [
        "recall_memory",
        "remember_fact",
    ]


def test_recall_schema_requires_only_keywords(mock_client):
    """Only keywords are required, so the model never has to invent a budget.

    The shorthand schema form would mark top and depth required as well.
    """
    schema = recall_tool(mock_client()).input_schema
    assert schema["required"] == ["keywords"]
    assert set(schema["properties"]) == {"keywords", "top", "depth"}


def test_remember_schema_requires_only_fact(mock_client):
    """Only the fact is required; topics and entities are optional."""
    schema = remember_tool(mock_client()).input_schema
    assert schema["required"] == ["fact"]


def test_recall_formats_hits(mock_client, invoke_mcp_tool, mcp_text):
    """Hits come back one per line with their relevance, in a payload not flagged as an error."""
    client = mock_client(
        hits=[
            Hit(value="the sky is blue", score=0.9),
            Hit(value="grass is green", score=0.5),
        ]
    )
    payload = invoke_mcp_tool(recall_tool(client), keywords=["sky"])
    assert mcp_text(payload) == (
        "- the sky is blue (relevance 0.900)\n- grass is green (relevance 0.500)"
    )
    assert "is_error" not in payload


def test_recall_without_hits_says_so(mock_client, invoke_mcp_tool, mcp_text):
    """A recall that matched nothing says so in words, and is not an error."""
    payload = invoke_mcp_tool(recall_tool(mock_client()), keywords=["nothing"])
    assert mcp_text(payload) == "No stored facts matched those keywords."
    assert "is_error" not in payload


def test_recall_flags_server_errors_with_is_error(
    mock_client, invoke_mcp_tool, mcp_text
):
    """A FraiseError comes back as text with ``is_error`` set instead of being raised."""
    client = mock_client(raises=FraiseError("connection refused"))
    payload = invoke_mcp_tool(recall_tool(client), keywords=["anything"])
    assert mcp_text(payload) == "memory lookup failed: connection refused"
    assert payload["is_error"] is True


def test_recall_defaults_top_and_leaves_depth_to_the_server(
    mock_client, invoke_mcp_tool
):
    """An omitted top takes the tool's default; an omitted depth is passed as
    None so no clause is emitted and the server's configured lane applies. A
    tool-side depth would be a lane the tool cannot use, since it names no
    topic or entity, and any value above the floor draws a warning per call.
    """
    client = mock_client()
    invoke_mcp_tool(recall_tool(client), keywords=["a"])
    call = client.recall.call_args.kwargs
    assert call["top"] == 5
    assert call["depth"] is None


def test_recall_schema_bounds_depth_to_the_lanes(mock_client):
    """The schema states the lanes' range so the model never has to guess it.

    A depth past 2 is not a deeper search but a request the server rejects,
    so the bound belongs in the contract the model reads, not only in the
    check behind it.
    """
    depth = recall_tool(mock_client()).input_schema["properties"]["depth"]
    assert (depth["minimum"], depth["maximum"]) == (0, 2)


@pytest.mark.parametrize("depth", [-1, 3, 99])
def test_recall_refuses_a_depth_past_the_lanes_before_calling_the_server(
    depth, mock_client, invoke_mcp_tool, mcp_text
):
    """An out-of-range depth is answered with a correction and never sent.

    Sent on, it would fail anyway, in the query builder or at the server; the
    tool error names the range up front so the retry can be right.
    """
    client = mock_client()
    payload = invoke_mcp_tool(recall_tool(client), keywords=["a"], depth=depth)
    assert payload["is_error"] is True
    assert mcp_text(payload) == f"depth must be between 0 and 2, got {depth}"
    client.recall.assert_not_called()


def test_recall_passes_graph_and_budgets_through(mock_client, invoke_mcp_tool):
    """The tool's graph and the model's keywords, top and depth reach recall as given."""
    client = mock_client()
    invoke_mcp_tool(recall_tool(client, graph=3), keywords=["a", "b"], top=7, depth=2)
    client.recall.assert_called_once_with(
        "a", "b", graph=3, top=7, depth=2, vector=None, embed=False
    )


def test_recall_vectorises_through_the_embedder(
    mock_client, invoke_mcp_tool, callable_embedder
):
    """The keywords, joined into one text, are encoded once and sent with ``embed=False``."""
    client = mock_client()
    embedder = callable_embedder()
    invoke_mcp_tool(recall_tool(client, embedder=embedder), keywords=["ab", "cd"])
    embedder.assert_called_once_with("ab cd")
    call = client.recall.call_args.kwargs
    assert call["vector"] == [5.0] * 4
    assert call["embed"] is False


def test_recall_without_keywords_sends_no_vector_even_with_an_embedder(
    mock_client, invoke_mcp_tool, callable_embedder
):
    """With no keywords the embedder is never called.

    Encoding an empty string would seed the search with a meaningless vector.
    """
    client = mock_client()
    embedder = callable_embedder()
    invoke_mcp_tool(recall_tool(client, embedder=embedder), keywords=[])
    embedder.assert_not_called()
    assert client.recall.call_args.kwargs["vector"] is None


def test_remember_confirms_what_it_stored(mock_client, invoke_mcp_tool, mcp_text):
    """The answer repeats the fact stored, so the model sees what was kept."""
    client = mock_client()
    payload = invoke_mcp_tool(remember_tool(client), fact="the sky is blue")
    assert mcp_text(payload) == "Stored: the sky is blue"
    assert client.remember.call_args.args == ("the sky is blue",)


def test_remember_passes_topics_entities_and_graph(mock_client, invoke_mcp_tool):
    """The fact, its topics and entities, and the tool's graph reach remember as given."""
    client = mock_client()
    invoke_mcp_tool(
        remember_tool(client, graph=2),
        fact="anne likes orange",
        topics=["colour"],
        entities=["anne"],
    )
    client.remember.assert_called_once_with(
        "anne likes orange",
        graph=2,
        topics=["colour"],
        entities=["anne"],
        vector=None,
        embed=False,
    )


def test_remember_flags_server_errors_with_is_error(
    mock_client, invoke_mcp_tool, mcp_text
):
    """A FraiseError on remember comes back as text with ``is_error`` set."""
    client = mock_client(raises=FraiseError("bad value"))
    payload = invoke_mcp_tool(remember_tool(client), fact="x")
    assert mcp_text(payload) == "could not store the fact: bad value"
    assert payload["is_error"] is True


def test_remember_vectorises_through_the_embedder(
    mock_client, invoke_mcp_tool, callable_embedder
):
    """The fact is encoded once and sent as the vector, with ``embed=False``."""
    client = mock_client()
    embedder = callable_embedder()
    invoke_mcp_tool(remember_tool(client, embedder=embedder), fact="hello")
    embedder.assert_called_once_with("hello")
    call = client.remember.call_args.kwargs
    assert call["vector"] == [5.0] * 4
    assert call["embed"] is False
