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


def test_explain_breaks_down_each_hit_by_source(explain, pulsar_graph, pulsar_entity):
    """Every explained hit carries its contribution records.

    Each hit is a text seed. Its text record has the source name, the raw BM25
    mass, the rank in the text list, and a count (1 for a seed sighting). The
    recall names the shared entity, so each hit also carries one graph record
    through it, marked named: the entity's degree, both seeds' mass and their
    count. No contribution carries a hop field.
    """
    status, body = explain(
        f"recall@{pulsar_graph} pulsar entity:{pulsar_entity} depth:2"
    )
    assert status == 200, body.get("error")
    hits = body["results"]["hits"]
    assert len(hits) == 2, f"want both pulsar facts, got {hits}"

    for hit in hits:
        contributions = hit.get("contributions")
        assert contributions, f"explained hit carries no contributions: {hit}"

        by_source = {c["source"]: c for c in contributions}
        assert set(by_source) == {
            "text",
            "graph",
        }, f"want a text seed lifted through the named entity, got {contributions}"
        graph = by_source["graph"]
        assert graph["via"] == pulsar_entity, graph
        assert graph["named"] is True, "the recall named the entity"
        assert graph["degree"] == 2, graph
        assert graph["count"] == 2, "both pulsar seeds fund the entity"
        assert by_source["text"]["rank"] in (0, 1), "two text seeds rank 0 and 1"
        assert by_source["text"]["count"] == 1, "a seed sighting counts once"
        assert "hop" not in by_source["text"], "hop left the wire with the walk"
        for c in contributions:
            assert isinstance(c["score"], (int, float)), c


def test_explain_shows_transmitted_surplus(
    explain,
    storm_graph,
    storm_cluster_topic,
    door_topic,
    storm_silent_member,
    storm_hub,
):
    """Funded case: the cluster's silent member carries a graph contribution.

    The contribution names its funding anchor: the cluster's topic as via, the
    anchor's degree, its observed mass and its funding-seed count. The recall
    enters through the door, so the cluster is discovered, not named, and the
    contribution carries no named mark. This proves that an anchor passes on
    surplus, not reachability. The archive hub's memos stay out. The hub holds
    no more than its fair share, so it transmits nothing.
    """
    status, body = explain(
        f"recall@{storm_graph} barometer storm topic:{door_topic} top:20"
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
    assert "named" not in contribution, "the cluster was discovered, not named"

    for memo in storm_hub[1:]:
        assert memo not in hits, f"fair-share hub memo {memo!r} rode in on size alone"


def test_explain_marks_a_named_hub(
    explain, storm_graph, storm_hub_topic, storm_cluster_topic, storm_hub
):
    """Naming the archive hub lets it speak, and the payload says why.

    Discovered, the hub holds no more than its fair share and stays silent
    (above). Named, it owes no fair share: its memos ride in behind its
    matching fact, each through one graph contribution marked named. The
    weather cluster, discovered beside it, still earns its own transmission
    unmarked.
    """
    status, body = explain(
        f"recall@{storm_graph} barometer storm topic:{storm_hub_topic} top:20"
    )
    assert status == 200, body.get("error")
    hits = {h["value"]: h for h in body["results"]["hits"]}

    for memo in storm_hub[1:]:
        assert memo in hits, f"named hub memo {memo!r} missing from {list(hits)}"
        [contribution] = hits[memo]["contributions"]
        assert contribution["via"] == storm_hub_topic, contribution
        assert contribution["named"] is True, contribution

    cluster = [
        c
        for h in hits.values()
        for c in h["contributions"]
        if c.get("via") == storm_cluster_topic
    ]
    assert cluster, "the weather cluster still transmits beside the named hub"
    assert not any(c.get("named") for c in cluster), cluster


def test_explain_score_recomputes_from_payload(
    explain, storm_graph, storm_hub_topic, alpha
):
    """Every hit's score recomputes from its own payload.

    With the query-level background rate, each score equals the formula applied
    to the hit's contributions:

        S = m + α²·Σ max(0, M_A − m − d_A·ρ₀)/d_A

    A contribution marked named drops the d_A·ρ₀ term, since a named anchor
    owes no fair share. The recall names the hub and discovers the cluster, so
    both kinds of term are recomputed. Each anchor's surplus arrives as its
    per-edge share. The tolerance absorbs the recency decay between write and
    read. So the payload is a complete explanation, not a summary.
    """
    status, body = explain(
        f"recall@{storm_graph} barometer storm topic:{storm_hub_topic} top:20"
    )
    assert status == 200, body.get("error")
    background = body["results"].get("background", 0)
    assert background > 0, "two anchors are touched; the null rate must be positive"

    for hit in body["results"]["hits"]:
        contributions = hit["contributions"]
        m = sum(c["score"] for c in contributions if c["source"] in ("text", "vector"))
        surplus = sum(
            max(
                0.0,
                c["score"] - m - (0 if c.get("named") else c["degree"] * background),
            )
            / max(c["degree"], 0.01)
            for c in contributions
            if c["source"] == "graph"
        )
        want = m + alpha * alpha * surplus
        assert abs(hit["score"] - want) < 1e-3, (
            f"{hit['value']!r}: score {hit['score']} != recomputed {want} "
            f"from {contributions} at background {background}"
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
