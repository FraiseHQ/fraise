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

"""OpenAI Agents tool tests against a fake client — no server, no model calls.

The tools are exercised the way the framework invokes them, through
``FunctionTool.on_invoke_tool`` with a JSON argument string, so the assertions
cover the real wrapping rather than the undecorated closures.
"""

import asyncio
import json

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


class _FakeClient:
    """Records recall/remember calls and replays a canned result."""

    def __init__(self, hits=None, raises=None):
        self._hits = hits or []
        self._raises = raises
        self.recall_calls = []
        self.remember_calls = []

    def recall(self, *keywords, **kwargs):
        self.recall_calls.append({"keywords": keywords, **kwargs})
        if self._raises:
            raise self._raises
        return RecallResult(count=len(self._hits), hits=self._hits)

    def remember(self, value, **kwargs):
        self.remember_calls.append({"value": value, **kwargs})
        if self._raises:
            raise self._raises


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
    tools = memory_tools(_FakeClient())
    assert [tool.name for tool in tools] == ["recall_memory", "remember_fact"]


def test_tool_names_are_overridable():
    client = _FakeClient()
    assert recall_tool(client, name="lookup").name == "lookup"
    assert remember_tool(client, name="store").name == "store"


def test_recall_formats_hits_by_descending_relevance():
    client = _FakeClient(
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
    result = _invoke(recall_tool(_FakeClient(hits=[])), keywords=["nothing"])
    assert result == "No stored facts matched those keywords."


def test_recall_reports_server_errors_as_text():
    # Tools must not raise into the agent loop: a raised FraiseError would
    # abort the whole run rather than let the model recover.
    client = _FakeClient(raises=FraiseError("connection refused"))
    result = _invoke(recall_tool(client), keywords=["anything"])
    assert result == "memory lookup failed: connection refused"


def test_recall_passes_graph_and_budgets_through():
    client = _FakeClient(hits=[])
    _invoke(recall_tool(client, graph=3), keywords=["a", "b"], top=7, depth=4)
    call = client.recall_calls[0]
    assert call["keywords"] == ("a", "b")
    assert call["graph"] == 3
    assert call["top"] == 7
    assert call["depth"] == 4


def test_recall_defaults_top_and_depth_when_the_model_omits_them():
    client = _FakeClient(hits=[])
    _invoke(recall_tool(client), keywords=["a"])
    call = client.recall_calls[0]
    assert call["top"] == 5
    assert call["depth"] == 2


def test_recall_vectorises_through_the_embedder():
    client = _FakeClient(hits=[])
    _invoke(
        recall_tool(client, embedder=lambda text: [len(text), 1.0]),
        keywords=["ab", "cd"],
    )
    call = client.recall_calls[0]
    # Keywords are joined before encoding, so the vector covers the whole query.
    assert call["vector"] == [5, 1.0]
    # embed=False: the tool has already encoded, the client must not redo it.
    assert call["embed"] is False


def test_recall_without_an_embedder_sends_no_vector():
    client = _FakeClient(hits=[])
    _invoke(recall_tool(client), keywords=["a"])
    assert client.recall_calls[0]["vector"] is None


def test_remember_confirms_what_it_stored():
    client = _FakeClient()
    result = _invoke(remember_tool(client), fact="the sky is blue")
    assert result == "Stored: the sky is blue"
    assert client.remember_calls[0]["value"] == "the sky is blue"


def test_remember_passes_topics_entities_and_graph():
    client = _FakeClient()
    _invoke(
        remember_tool(client, graph=2),
        fact="anne likes orange",
        topics=["colour"],
        entities=["anne"],
    )
    call = client.remember_calls[0]
    assert call["graph"] == 2
    assert call["topics"] == ["colour"]
    assert call["entities"] == ["anne"]


def test_remember_reports_server_errors_as_text():
    client = _FakeClient(raises=FraiseError("bad value"))
    result = _invoke(remember_tool(client), fact="x")
    assert result == "could not store the fact: bad value"


def test_remember_vectorises_through_the_embedder():
    client = _FakeClient()
    _invoke(remember_tool(client, embedder=lambda text: [0.5]), fact="hello")
    call = client.remember_calls[0]
    assert call["vector"] == [0.5]
    assert call["embed"] is False
