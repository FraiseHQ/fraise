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

# Two facts sharing a keyword and an entity, written idempotently to graph 2.
# Both are text seeds for "pulsar"; their shared entity is the query's only
# touched anchor, so it sits exactly at the background rate and transmits
# nothing — every hit's breakdown is a single text contribution. (Anchors are
# heard only when their members matched better than their size predicts; see
# test_explain_shows_transmitted_surplus for the funded case.)
PULSAR_FACTS = (
    "the pulsar spins thirty times a second",
    "the pulsar emits radio beams",
)
PULSAR_ENTITY = "vela"


def _seed_pulsar_facts(query):
    for phrase in PULSAR_FACTS:
        status, body = query(f"remember@2 '{phrase}' entity:{PULSAR_ENTITY}")
        assert status == 200, body.get("error")


def test_explain_breaks_down_each_hit_by_source(query, explain):
    """Every explained hit carries its contribution records: here each hit is
    a pure text seed — source name, raw BM25 mass, rank in the text list, and
    count (1 for a seed sighting). The shared entity is the only touched
    anchor, so it holds no surplus and no graph contribution appears; hop is
    gone from the wire with the walk that produced it.
    """
    _seed_pulsar_facts(query)

    status, body = explain("recall@2 pulsar depth:2")
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


# The surplus fixture: a small "weather" cluster concentrates the query's mass
# while a larger "archive" hub holds a fair share of it, so exactly one anchor
# speaks and its silent member is funded by transmission alone.
STORM_CLUSTER = (
    "the barometer falls before the storm",
    "storm clouds gather at sea",
    "the harbour is calm tonight",  # no query term: funded or invisible
)
STORM_HUB = ("a storm of paperwork",) + tuple(
    f"unrelated archive memo {i}" for i in range(7)
)

ALPHA = 0.5  # the server's per-edge attenuation; α² on the two-edge path


def _seed_storm_facts(query):
    for phrase in STORM_CLUSTER:
        status, body = query(f"remember@2 '{phrase}' topic:weather")
        assert status == 200, body.get("error")
    for phrase in STORM_HUB:
        status, body = query(f"remember@2 '{phrase}' topic:archive")
        assert status == 200, body.get("error")


def test_explain_shows_transmitted_surplus(query, explain):
    """The funded case: the cluster's silent member surfaces carrying a graph
    contribution that names its funding anchor — via "weather", the anchor's
    degree, its observed mass and funding-seed count — proving surplus, not
    reachability, is what an anchor passes on. The archive hub's memos stay
    out: at fair share it transmits nothing.
    """
    _seed_storm_facts(query)

    status, body = explain("recall@2 barometer storm top:20")
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

    for memo in STORM_HUB[1:]:
        assert memo not in hits, f"fair-share hub memo {memo!r} rode in on size alone"


def test_explain_score_recomputes_from_payload(query, explain):
    """The recompute pin: with the query-level background rate, every hit's
    score equals the formula applied to its own payload — S = m + α²·Σ max(0,
    M_A − m − d_A·ρ₀) — within float tolerance (the hair of recency decay
    between write and read). The payload is therefore a complete explanation,
    not a summary.
    """
    _seed_storm_facts(query)

    status, body = explain("recall@2 barometer storm top:20")
    assert status == 200, body.get("error")
    background = body["results"].get("background", 0)
    assert background > 0, "two anchors are touched; the null rate must be positive"

    for hit in body["results"]["hits"]:
        contributions = hit["contributions"]
        m = sum(c["score"] for c in contributions if c["source"] in ("text", "vector"))
        surplus = sum(
            max(0.0, c["score"] - m - c["degree"] * background) / c["degree"]
            for c in contributions
            if c["source"] == "graph"
        )
        want = m + ALPHA * ALPHA * surplus
        assert abs(hit["score"] - want) < 1e-3, (
            f"{hit['value']!r}: score {hit['score']} != recomputed {want} "
            f"from {contributions} at background {background}"
        )


def test_plain_query_carries_no_contributions(query):
    """The /q response is unchanged by explain existing: no hit exposes a
    contributions key. Guards against the breakdown leaking into every recall
    and bloating agent token budgets.
    """
    _seed_pulsar_facts(query)

    status, body = query("recall@2 pulsar depth:2")
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
        "recall",  # no term
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
