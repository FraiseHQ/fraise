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
# Each is a text seed for "pulsar" AND is reached by the other seed's walk
# through the shared entity at hop 2, so every hit's breakdown carries exactly
# one text contribution and one graph contribution — both retrieval sources in
# one deterministic query, with no embedding model needed.
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
    """Every explained hit carries its contribution records: the seed sighting
    from the text index (hop 0) and the walk sighting through the shared
    entity (hop 2), each with a source name, raw score, and rank.
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
        assert set(by_source) == {"text", "graph"}, (
            f"want one text and one graph sighting, got {contributions}"
        )
        assert by_source["text"]["hop"] == 0, "a seed sighting is hop 0"
        assert by_source["text"]["rank"] in (0, 1), "two tied text seeds rank 0 and 1"
        assert by_source["graph"]["hop"] == 2, (
            "the sibling fact is two hops away through the shared entity"
        )
        for c in contributions:
            assert isinstance(c["score"], (int, float)), c


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
