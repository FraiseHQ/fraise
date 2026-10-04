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

"""The explain endpoint: /api/v1/explain runs a recall through the ordinary
pipeline and attaches each hit's per-source contribution breakdown, while /q
responses stay free of it.
"""

import pytest


def test_explain_breaks_down_each_hit_by_source(explain, pulsar_graph):
    """Every explained hit carries its contribution records: here each hit is
    a pure text seed — source name, raw BM25 mass, rank in the text list, and
    count (1 for a seed sighting). The shared entity is the only touched
    anchor, so it holds no surplus and no graph contribution appears; no
    contribution carries a hop field.
    """
    status, body = explain(f"recall@{pulsar_graph} pulsar entity:vela depth:2")
    assert status == 200, body.get("error")
    hits = body["results"]["hits"]
    assert len(hits) == 2, f"want both pulsar facts, got {hits}"

    for hit in hits:
        contributions = hit.get("contributions")
        assert contributions, f"explained hit carries no contributions: {hit}"

        by_source = {c["source"]: c for c in contributions}
        assert set(by_source) == {"text"}, (
            f"a lone fair-share anchor must transmit nothing, got {contributions}"
        )
        assert by_source["text"]["rank"] in (0, 1), "two text seeds rank 0 and 1"
        assert by_source["text"]["count"] == 1, "a seed sighting counts once"
        assert "hop" not in by_source["text"], "hop left the wire with the walk"
        for c in contributions:
            assert isinstance(c["score"], (int, float)), c


def test_explain_shows_transmitted_surplus(explain, storm_graph, storm_hub):
    """The funded case: the cluster's silent member surfaces carrying a graph
    contribution that names its funding anchor — via "weather", the anchor's
    degree, its observed mass and funding-seed count — proving surplus, not
    reachability, is what an anchor passes on. The archive hub's memos stay
    out: holding no more than its fair share, it transmits nothing.
    """
    status, body = explain(
        f"recall@{storm_graph} barometer storm topic:weather topic:archive top:20"
    )
    assert status == 200, body.get("error")
    hits = {h["value"]: h for h in body["results"]["hits"]}

    calm = hits.get("the harbour is calm tonight")
    assert calm is not None, (
        f"want the cluster's silent member funded, got {list(hits)}"
    )
    [contribution] = calm["contributions"]
    assert contribution["source"] == "graph"
    assert contribution["via"] == "weather", contribution
    assert contribution["degree"] == 3, contribution
    assert contribution["count"] == 2, "two seeds fund the weather cluster"
    assert contribution["score"] > 0, "the observation carries the anchor's mass"

    for memo in storm_hub[1:]:
        assert memo not in hits, f"fair-share hub memo {memo!r} rode in on size alone"


def test_explain_score_recomputes_from_payload(explain, storm_graph, alpha):
    """The recompute pin: with the query-level background rate, every hit's
    score equals the formula applied to its own payload — S = m + α²·Σ max(0,
    M_A − m − d_A·ρ₀)/d_A, each anchor's surplus arriving as its per-edge
    share — within a tolerance that absorbs the recency decay between write
    and read. The payload is therefore a complete explanation, not a summary.
    """
    status, body = explain(
        f"recall@{storm_graph} barometer storm topic:weather topic:archive top:20"
    )
    assert status == 200, body.get("error")
    background = body["results"].get("background", 0)
    assert background > 0, "two anchors are touched; the null rate must be positive"

    for hit in body["results"]["hits"]:
        contributions = hit["contributions"]
        m = sum(c["score"] for c in contributions if c["source"] in ("text", "vector"))
        surplus = sum(
            max(0.0, c["score"] - m - c["degree"] * background) / max(c["degree"], 0.01)
            for c in contributions
            if c["source"] == "graph"
        )
        want = m + alpha * alpha * surplus
        assert abs(hit["score"] - want) < 1e-3, (
            f"{hit['value']!r}: score {hit['score']} != recomputed {want} "
            f"from {contributions} at background {background}"
        )


def test_plain_query_carries_no_contributions(query, pulsar_graph):
    """The /q response is unchanged by explain existing: no hit exposes a
    contributions key. Guards against the breakdown leaking into every recall
    and bloating agent token budgets.
    """
    status, body = query(f"recall@{pulsar_graph} pulsar entity:vela depth:2")
    assert status == 200, body.get("error")
    hits = body["results"]["hits"]
    assert hits, "the pulsar facts must be recallable"
    for hit in hits:
        assert "contributions" not in hit, (
            f"/q must not carry the explain breakdown: {hit}"
        )


def test_explain_rejects_remember(explain):
    """A write has no ranking to explain: /api/v1/explain refuses it with 400
    so probing a ranking can never mutate a graph.
    """
    status, body = explain("remember@2 'should never land' topic:explainprobe")
    assert status == 400
    assert "recall" in body["error"], body


@pytest.mark.parametrize(
    "bad_query",
    [
        "recall",  # no seed
        "explain me",  # not a command
    ],
)
def test_explain_rejects_unparsable_queries(explain, bad_query):
    """Parse errors surface as 400 on /explain exactly as they do on /q — the
    two routes share one pipeline.
    """
    status, body = explain(bad_query)
    assert status == 400
    assert "error" in body
