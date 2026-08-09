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

"""OpenAI Agents tool tests against a mocked client — no server, no model calls.

The tools are exercised the way the framework invokes them, through
``FunctionTool.on_invoke_tool`` with a JSON argument string, so the assertions
cover the real wrapping rather than the undecorated closures.
"""

import asyncio
import json
from unittest.mock import MagicMock

import pytest
from fraise_sdk.errors import FraiseError
from fraise_sdk.models import Hit, RecallResult

# The integration imports its framework at module scope, so skip the whole file
# when the optional 'openai' dependency group is not installed.
pytest.importorskip("agents", reason="requires the 'openai' dependency group")

from agents.tool_context import ToolContext  # noqa: E402
from fraise_sdk.integrations.openai_agents import (  # noqa: E402
    memory_tools,
    recall_tool,
    remember_tool,
)


def _client(hits=(), raises=None) -> MagicMock:
    """A mock FraiseClient replaying one recall result."""
    client = MagicMock()
    client.recall.return_value = RecallResult(count=len(hits), hits=list(hits))
    if raises is not None:
        client.recall.side_effect = raises
        client.remember.side_effect = raises
    return client


def _encode(text: str) -> list[float]:
    """A bare ``callable(text) -> vector`` embedder: the text's length."""
    return [float(len(text))]


def _embedder() -> MagicMock:
    """A mock of the bare ``callable(text) -> vector`` embedder shape."""
    embedder = MagicMock(side_effect=_encode)
    # No .embed, so resolve_embedder takes the plain-callable branch.
    del embedder.embed
    return embedder


def _invoke(tool, **arguments):
    """Call a FunctionTool exactly as the agent runtime would."""
    payload = json.dumps(arguments)
    context = ToolContext(
        context=None,
        tool_name=tool.name,
        tool_call_id="test-call",
        tool_arguments=payload,
    )
    return asyncio.run(tool.on_invoke_tool(context, payload))


def test_memory_tools_returns_both_tools():
    tools = memory_tools(_client())
    assert [tool.name for tool in tools] == ["recall_memory", "remember_fact"]


def test_tool_names_are_overridable():
    client = _client()
    assert recall_tool(client, name="lookup").name == "lookup"
    assert remember_tool(client, name="store").name == "store"


def test_recall_formats_hits_by_descending_relevance():
    client = _client(
        hits=[
            Hit(value="the sky is blue", score=0.9),
            Hit(value="grass is green", score=0.5),
        ]
    )
    result = _invoke(recall_tool(client), keywords=["sky"])
    assert (
        result
        == "- the sky is blue (relevance 0.900)\n- grass is green (relevance 0.500)"
    )


def test_recall_without_hits_says_so_rather_than_returning_empty():
    # An empty string would read to the model as a broken tool; the wording is
    # what tells it to answer from its own context instead.
    result = _invoke(recall_tool(_client()), keywords=["nothing"])
    assert result == "No stored facts matched those keywords."


def test_recall_reports_server_errors_as_text():
    # Tools must not raise into the agent loop: a raised FraiseError would
    # abort the whole run rather than let the model recover.
    client = _client(raises=FraiseError("connection refused"))
    result = _invoke(recall_tool(client), keywords=["anything"])
    assert result == "memory lookup failed: connection refused"


def test_recall_passes_graph_and_budgets_through():
    client = _client()
    _invoke(recall_tool(client, graph=3), keywords=["a", "b"], top=7, depth=4)
    client.recall.assert_called_once_with(
        "a", "b", graph=3, top=7, depth=4, vector=None, embed=False
    )


def test_recall_defaults_top_and_depth_when_the_model_omits_them():
    client = _client()
    _invoke(recall_tool(client), keywords=["a"])
    call = client.recall.call_args.kwargs
    assert call["top"] == 5
    assert call["depth"] == 2


def test_recall_vectorises_through_the_embedder():
    client = _client()
    embedder = _embedder()
    _invoke(recall_tool(client, embedder=embedder), keywords=["ab", "cd"])
    # Keywords are joined before encoding, so the vector covers the whole query.
    embedder.assert_called_once_with("ab cd")
    call = client.recall.call_args.kwargs
    assert call["vector"] == [5.0]
    # embed=False: the tool has already encoded, the client must not redo it.
    assert call["embed"] is False


def test_recall_without_an_embedder_sends_no_vector():
    client = _client()
    _invoke(recall_tool(client), keywords=["a"])
    assert client.recall.call_args.kwargs["vector"] is None


def test_remember_confirms_what_it_stored():
    client = _client()
    result = _invoke(remember_tool(client), fact="the sky is blue")
    assert result == "Stored: the sky is blue"
    assert client.remember.call_args.args == ("the sky is blue",)


def test_remember_passes_topics_entities_and_graph():
    client = _client()
    _invoke(
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


def test_remember_reports_server_errors_as_text():
    client = _client(raises=FraiseError("bad value"))
    result = _invoke(remember_tool(client), fact="x")
    assert result == "could not store the fact: bad value"


def test_remember_vectorises_through_the_embedder():
    client = _client()
    embedder = _embedder()
    _invoke(remember_tool(client, embedder=embedder), fact="hello")
    embedder.assert_called_once_with("hello")
    call = client.remember.call_args.kwargs
    assert call["vector"] == [5.0]
    assert call["embed"] is False
