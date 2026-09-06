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

"""The two tools (pkg/mcp/tools.go) end to end: real daemon, real bridge,
real stdio. What passes here is what an agent's tool call actually does.
"""

from datetime import datetime


def test_remember_then_recall_round_trips_a_fact(mcp):
    """A fact stored through the bridge is found through the bridge.

    Both faces of the result are pinned: the rendered text an agent reads,
    and the structured content a programmatic client parses — count, value,
    score, and a timestamp that is a real RFC 3339 instant.
    """
    stored = mcp.call(
        "remember",
        {"query": "remember@1 'the kittiwake nests on sheer cliffs' topic:birds"},
    )
    assert not stored.get("isError", False), stored
    assert stored["content"][0]["text"] == "Remembered."

    found = mcp.call("recall", {"query": "recall@1 kittiwake"})
    assert not found.get("isError", False), found

    results = found["structuredContent"]["results"]
    assert results["count"] == 1
    hit = results["hits"][0]
    assert hit["value"] == "the kittiwake nests on sheer cliffs"
    assert hit["score"] > 0
    datetime.fromisoformat(hit["timestamp"])  # raises if not a real instant

    assert (
        "the kittiwake nests on sheer cliffs (relevance" in found["content"][0]["text"]
    )


def test_vector_parameters_bind_through_the_bridge(mcp):
    """Out-of-band vector bindings survive the stdio hop: a fact stored with
    vec:$v is recallable by vector seed alone, which can only work if the
    parameters travelled beside both queries intact.
    """
    vector = [0.5, 0.25, 0.125, 0.0625]
    stored = mcp.call(
        "remember",
        {
            "query": "remember@2 'the foghorn sounds at one-minute intervals' vec:$v",
            "parameters": {"v": vector},
        },
    )
    assert not stored.get("isError", False), stored

    found = mcp.call(
        "recall", {"query": "recall@2 vec:$v", "parameters": {"v": vector}}
    )
    assert not found.get("isError", False), found
    values = [hit["value"] for hit in found["structuredContent"]["results"]["hits"]]
    assert "the foghorn sounds at one-minute intervals" in values


def test_daemon_rejections_surface_in_band(mcp):
    """A query the daemon rejects comes back as a tool error carrying the
    daemon's own message — the text a model needs to correct itself — never
    as a protocol failure that would end the conversation.
    """
    result = mcp.call("recall", {"query": "recall@1 anything top:0"})
    assert result.get("isError") is True
    assert "top:0 out of range" in result["content"][0]["text"]


def test_parse_warnings_ride_beside_the_results(mcp):
    """A query that runs with a parse warning delivers the warning on both
    faces: in the structured payload and rendered for the model.
    """
    result = mcp.call("recall", {"query": "recall@1 since 7d"})
    assert not result.get("isError", False), result
    assert result["structuredContent"].get("warnings"), (
        "the keyword-shaped term must warn"
    )
    assert "warning:" in result["content"][0]["text"]


def test_an_unreachable_daemon_is_an_in_band_error(mcp_without_daemon):
    """With nothing listening, the bridge still converses: the handshake ran
    (the fixture proves it) and a tool call fails in band, naming the address
    it tried and the service commands that start the daemon.
    """
    result = mcp_without_daemon.call("recall", {"query": "recall@1 anything"})
    assert result.get("isError") is True
    text = result["content"][0]["text"]
    assert "is the daemon running" in text


def test_anchor_only_recall_returns_what_is_filed_under_the_anchor(mcp):
    """Anchors alone seed the search with what is filed under them, so an
    agent can see what it knows about a topic before it knows what to search
    for.

    Two facts filed under one topic, and nothing else on graph 3, both come
    back from `recall@3 topic:cormorants`, each scored like any recall hit:
    the unit of mass its anchor lends, decayed by age.
    """
    facts = (
        "the cormorant dries its wings on the post",
        "the cormorant dives from the surface",
    )
    for phrase in facts:
        stored = mcp.call(
            "remember", {"query": f"remember@3 '{phrase}' topic:cormorants"}
        )
        assert not stored.get("isError", False), stored

    found = mcp.call("recall", {"query": "recall@3 topic:cormorants"})
    assert not found.get("isError", False), found
    hits = found["structuredContent"]["results"]["hits"]
    assert {hit["value"] for hit in hits} == set(facts)
    assert all(hit["score"] > 0 for hit in hits), hits
