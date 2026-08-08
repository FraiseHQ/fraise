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

"""Claude Agent SDK tool tests against a fake client — no server, no model calls.

Unlike the OpenAI tools these return MCP content payloads rather than plain
strings, and failures are flagged with ``is_error`` rather than raised, so the
assertions check that envelope as well as the text.
"""

import asyncio

import pytest
from fraise_sdk.errors import FraiseError
from fraise_sdk.models import Hit, RecallResult

# The integration imports its framework at module scope, so skip the whole file
# when the optional 'anthropic' dependency group is not installed.
pytest.importorskip(
    "claude_agent_sdk", reason="requires the 'anthropic' dependency group"
)

from fraise_sdk.integrations.claude_agents import (  # noqa: E402
    DEFAULT_SERVER_NAME,
    allowed_tools,
    memory_server,
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
    """Call an SdkMcpTool's handler the way the in-process MCP server would."""
    return asyncio.run(tool.handler(arguments))


def _text(payload):
    return payload["content"][0]["text"]


def test_memory_tools_returns_both_tools():
    tools = memory_tools(_FakeClient())
    assert [tool.name for tool in tools] == ["recall_memory", "remember_fact"]


def test_allowed_tools_matches_the_registered_tool_names():
    # These strings are what Claude checks against; if they drift from the tool
    # names the tools are silently never callable.
    assert allowed_tools() == [
        f"mcp__{DEFAULT_SERVER_NAME}__recall_memory",
        f"mcp__{DEFAULT_SERVER_NAME}__remember_fact",
    ]


def test_allowed_tools_follows_a_custom_server_name():
    assert allowed_tools("other") == [
        "mcp__other__recall_memory",
        "mcp__other__remember_fact",
    ]


def test_memory_server_builds_with_both_tools():
    server = memory_server(_FakeClient())
    assert server is not None


def test_recall_schema_requires_only_keywords():
    # top/depth must stay optional: the shorthand schema form would mark every
    # property required and force the model to invent budgets.
    schema = recall_tool(_FakeClient()).input_schema
    assert schema["required"] == ["keywords"]
    assert set(schema["properties"]) == {"keywords", "top", "depth"}


def test_remember_schema_requires_only_fact():
    schema = remember_tool(_FakeClient()).input_schema
    assert schema["required"] == ["fact"]


def test_recall_formats_hits():
    client = _FakeClient(
        hits=[
            Hit(value="the sky is blue", score=0.9),
            Hit(value="grass is green", score=0.5),
        ]
    )
    payload = _invoke(recall_tool(client), keywords=["sky"])
    assert _text(payload) == (
        "- the sky is blue (relevance 0.900)\n- grass is green (relevance 0.500)"
    )
    assert "is_error" not in payload


def test_recall_without_hits_says_so():
    payload = _invoke(recall_tool(_FakeClient(hits=[])), keywords=["nothing"])
    assert _text(payload) == "No stored facts matched those keywords."
    assert "is_error" not in payload


def test_recall_flags_server_errors_with_is_error():
    client = _FakeClient(raises=FraiseError("connection refused"))
    payload = _invoke(recall_tool(client), keywords=["anything"])
    assert _text(payload) == "memory lookup failed: connection refused"
    assert payload["is_error"] is True


def test_recall_defaults_budgets_when_the_model_omits_them():
    client = _FakeClient(hits=[])
    _invoke(recall_tool(client), keywords=["a"])
    call = client.recall_calls[0]
    assert call["top"] == 5
    assert call["depth"] == 2


def test_recall_passes_graph_and_budgets_through():
    client = _FakeClient(hits=[])
    _invoke(recall_tool(client, graph=3), keywords=["a", "b"], top=7, depth=4)
    call = client.recall_calls[0]
    assert call["keywords"] == ("a", "b")
    assert call["graph"] == 3
    assert call["top"] == 7
    assert call["depth"] == 4


def test_recall_vectorises_through_the_embedder():
    client = _FakeClient(hits=[])
    _invoke(
        recall_tool(client, embedder=lambda text: [len(text)]), keywords=["ab", "cd"]
    )
    call = client.recall_calls[0]
    assert call["vector"] == [5]
    assert call["embed"] is False


def test_recall_without_keywords_sends_no_vector_even_with_an_embedder():
    # Encoding an empty string would seed the search with a meaningless vector.
    client = _FakeClient(hits=[])
    _invoke(recall_tool(client, embedder=lambda text: [1.0]), keywords=[])
    assert client.recall_calls[0]["vector"] is None


def test_remember_confirms_what_it_stored():
    client = _FakeClient()
    payload = _invoke(remember_tool(client), fact="the sky is blue")
    assert _text(payload) == "Stored: the sky is blue"
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


def test_remember_flags_server_errors_with_is_error():
    client = _FakeClient(raises=FraiseError("bad value"))
    payload = _invoke(remember_tool(client), fact="x")
    assert _text(payload) == "could not store the fact: bad value"
    assert payload["is_error"] is True


def test_remember_vectorises_through_the_embedder():
    client = _FakeClient()
    _invoke(remember_tool(client, embedder=lambda text: [0.5]), fact="hello")
    call = client.remember_calls[0]
    assert call["vector"] == [0.5]
    assert call["embed"] is False
