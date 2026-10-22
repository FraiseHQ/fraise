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

"""The explain endpoint.

/api/v1/explain runs a recall through the ordinary pipeline and attaches each
hit's per-source contribution breakdown. /q responses stay free of it.
"""

import pytest


def test_explain_breaks_down_each_hit_by_source(explain, pulsar_graph, unfiled_topic):
    """Every explained hit carries its contribution records: here each hit is
    a pure text seed — source name, raw BM25 mass, rank in the text list, and
    count (1 for a seed sighting). The recall names a topic nothing is filed
    under, which opens the round and leaves the facts' shared entity to it:
    the only touched anchor, it holds no surplus, so no graph contribution
    appears. No contribution carries a hop field.
    """
    status, body = explain(
        f"recall@{pulsar_graph} pulsar topic:{unfiled_topic} depth:2"
    )
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


def test_explain_shows_transmitted_surplus(
    explain,
    storm_graph,
    storm_cluster_topic,
    unfiled_topic,
    storm_silent_member,
    storm_hub,
):
    """The funded case: the cluster's silent member surfaces carrying a graph
    contribution that names its funding anchor — via the cluster's topic, the
    anchor's degree, its observed mass and funding-seed count — proving
    surplus, not reachability, is what an anchor passes on. The archive hub's
    memos stay out: holding no more than its fair share, it transmits nothing.
    The recall names a topic nothing is filed under, so both anchors are left
    to the round.
    """
    status, body = explain(
        f"recall@{storm_graph} barometer storm topic:{unfiled_topic} top:20"
    )
    assert status == 200, body.get("error")
    hits = {h["value"]: h for h in body["results"]["hits"]}

    calm = hits.get(storm_silent_member)
    assert calm is not None, (
        f"want the cluster's silent member funded, got {list(hits)}"
    )
    [contribution] = calm["contributions"]
    assert contribution["source"] == "graph"
    assert contribution["via"] == storm_cluster_topic, contribution
    assert contribution["degree"] == 3, contribution
    assert contribution["count"] == 2, "two seeds fund the storm cluster"
    assert contribution["score"] > 0, "the observation carries the anchor's mass"

    for memo in storm_hub[1:]:
        assert memo not in hits, f"fair-share hub memo {memo!r} rode in on size alone"


def test_explain_score_recomputes_from_payload(
    explain, storm_graph, storm_hub_topic, storm_hub, information_scores
):
    """The recompute pin: with the query-level background rate, every hit's
    score equals the information fold applied to the payload, within a
    tolerance that absorbs the recency decay between write and read. The fold
    measures each fact against the recall's whole pool, so the recall returns
    every candidate, and the payload is then a complete explanation, not a
    summary. Naming the hub puts both kinds of graph evidence in it: the
    hub's memos carry named observations, owing no fair share, while the
    cluster, left to the round, funds its silent member with a graph one.
    """
    status, body = explain(
        f"recall@{storm_graph} barometer storm topic:{storm_hub_topic} top:1000"
    )
    assert status == 200, body.get("error")
    background = body["results"].get("background", 0)
    assert background > 0, "two anchors are touched; the null rate must be positive"
    hits = body["results"]["hits"]
    sources = {c["source"] for hit in hits for c in hit["contributions"]}
    assert {"text", "graph", "named"} <= sources, sources
    assert {h["value"] for h in hits} >= set(storm_hub), (
        "the named hub raises its memos"
    )

    for hit, want in zip(hits, information_scores(hits, background), strict=True):
        assert abs(hit["score"] - want) < 1e-6 * max(1.0, want), (
            f"{hit['value']!r}: score {hit['score']} != recomputed {want} "
            f"from {hit['contributions']} at background {background}"
        )


def test_plain_query_carries_no_contributions(query, pulsar_graph, pulsar_entity):
    """No /q hit carries a contributions key.

    This guards against the breakdown leaking into every recall and bloating
    agent token budgets.
    """
    status, body = query(f"recall@{pulsar_graph} pulsar entity:{pulsar_entity} depth:2")
    assert status == 200, body.get("error")
    hits = body["results"]["hits"]
    assert hits, "the pulsar facts must be recallable"
    for hit in hits:
        assert "contributions" not in hit, (
            f"/q must not carry the explain breakdown: {hit}"
        )


def test_explain_rejects_remember(explain):
    """/api/v1/explain refuses a remember with a 400.

    A write has no ranking to explain, and probing a ranking must never mutate
    a graph.
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
    """Parse errors are a 400 on /explain, exactly as on /q.

    The two routes share one pipeline.
    """
    status, body = explain(bad_query)
    assert status == 400
    assert "error" in body
