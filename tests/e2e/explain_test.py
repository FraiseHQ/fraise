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
hit's per-source contribution breakdown and its decay. Where a time bound read
a when, it attaches that when's answer too. /q responses stay free of all
three.
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
    explain, storm_graph, storm_hub_topic, storm_hub, information_scores
):
    """Every hit's score recomputes from the payload.

    With the query-level background rate, every hit's score equals the
    information fold applied to the payload, times the hit's decay. The decay
    is on the payload because nothing else there says how much a fact's age
    cost it, and without it the score would recompute only to the precision of
    the time since the write. The fold measures each fact against the recall's
    whole pool, so the recall returns every candidate, and the payload is then
    a complete explanation, not a summary. The recall names the hub and
    discovers the cluster, so both kinds of graph term are recomputed: the
    hub's memos carry graph contributions marked named, owing no fair share,
    while the cluster funds its silent member unmarked.
    """
    status, body = explain(
        f"recall@{storm_graph} barometer storm topic:{storm_hub_topic} top:1000"
    )
    assert status == 200, body.get("error")
    background = body["results"].get("background", 0)
    assert background > 0, "two anchors are touched; the null rate must be positive"
    hits = body["results"]["hits"]
    graph = [c for hit in hits for c in hit["contributions"] if c["source"] == "graph"]
    assert any(c.get("named") for c in graph), graph
    assert not all(c.get("named") for c in graph), graph
    assert {h["value"] for h in hits} >= set(storm_hub), "the named hub lifts its memos"

    for hit, fold in zip(hits, information_scores(hits, background), strict=True):
        want = fold * hit["decay"]
        assert abs(hit["score"] - want) < 1e-9 * max(1.0, want), (
            f"{hit['value']!r}: score {hit['score']} != recomputed {want} "
            f"from {hit['contributions']} at background {background} "
            f"and decay {hit['decay']}"
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


def test_explain_carries_each_hits_decay_from_when_it_was_written(
    explain, timeline_graph, timeline_keyword, timeline_facts
):
    """Every explained hit carries its decay, and a fact ages from its write.

    The decay is the share of its score a fact keeps at its age. The timeline's
    facts were all written moments ago, so each keeps nearly all of it,
    whatever date the fact was said or happened on. Aged from May 2023 instead,
    at the suite's 2160h half-life, they would have halved some fourteen times
    and kept less than a thousandth.
    """
    status, body = explain(f"recall@{timeline_graph} {timeline_keyword}")
    assert status == 200, body.get("error")
    hits = body["results"]["hits"]
    assert len(hits) == len(timeline_facts), hits

    for hit in hits:
        assert 0.99 < hit["decay"] <= 1, f"{hit['value']!r}: decay {hit['decay']}"


def test_explain_carries_the_answer_of_each_when_a_bound_read(
    explain, timeline_graph, timeline_keyword, timeline_facts
):
    """An explained recall shows the when behind each of its bounds.

    A bound written as a when is a time the caller never saw: a search of its
    own found a fact, and the fact's time became the bound. So the payload
    holds that search under results.when, by the name of the bound that read
    it. Each entry is the when's own results: the fact it kept, explained like
    any other hit, and the start and end it answered with. since: opened the
    window at the end of the regatta and until: closed it at the start of the
    cruise, which leaves the protest.
    """
    status, body = explain(
        f"recall@{timeline_graph} {timeline_keyword} "
        "since:(when regatta) until:(when cruise)"
    )
    assert status == 200, body.get("error")
    results = body["results"]
    assert [hit["value"] for hit in results["hits"]] == [timeline_facts["protest"]]
    assert set(results["when"]) == {"since", "until"}

    since = results["when"]["since"]
    assert [hit["value"] for hit in since["hits"]] == [timeline_facts["regatta"]]
    assert (since["start"], since["end"]) == (
        "2023-05-07T00:00:00Z",
        "2023-05-08T00:00:00Z",
    )

    until = results["when"]["until"]
    assert [hit["value"] for hit in until["hits"]] == [timeline_facts["cruise"]]
    assert (until["start"], until["end"]) == (
        "2023-05-10T00:00:00Z",
        "2023-05-17T00:00:00Z",
    )

    for hit in since["hits"] + until["hits"]:
        assert hit["contributions"], f"the when's own hit is unexplained: {hit}"
        assert "decay" in hit, hit


@pytest.mark.parametrize(
    ("clause", "bounds"),
    [
        ("until:(when regatta)", ["until"]),
        ("since:(when regatta)", ["since"]),
        ("since:(when regatta) until:'2023-05-11'", ["since"]),
        ("since:'2023-05-08' until:'2023-05-11'", []),
        ("", []),
    ],
)
def test_explain_names_only_the_bounds_that_read_a_when(
    clause, bounds, explain, timeline_graph, timeline_keyword
):
    """results.when has an entry for a bound that held a when, and no other.

    A bound written as a date explains itself, so it has no entry. A recall
    with no when in either bound has no when key at all.
    """
    status, body = explain(f"recall@{timeline_graph} {timeline_keyword} {clause}")
    assert status == 200, body.get("error")
    results = body["results"]

    assert sorted(results.get("when", {})) == bounds, results.get("when")
    assert ("when" in results) == bool(bounds), results


def test_explain_shows_the_when_that_matched_nothing(
    explain, timeline_graph, timeline_keyword
):
    """A bound left unread is explained by the empty answer of its when.

    The recall returns no hits, and the warning says why. The payload shows
    the same thing: the when under the bound's name, with no hits and no time.
    """
    status, body = explain(
        f"recall@{timeline_graph} {timeline_keyword} since:(when zzznomatchzzz)"
    )
    assert status == 200, body.get("error")
    assert body == {
        "results": {
            "count": 0,
            "hits": [],
            "when": {"since": {"count": 0, "hits": []}},
        },
        "warnings": [
            "since: the when matched no fact, so it has no time to answer with; "
            "the recall has no bound to read and returns no hits"
        ],
    }


def test_explain_accepts_a_when(explain, timeline_graph, timeline_facts):
    """A when is a recall that answers with a time, so it explains like one.

    The hit it read its time from carries its contributions and its decay, and
    the results keep the start and end.
    """
    status, body = explain("when regatta")
    assert status == 200, body.get("error")
    results = body["results"]
    [hit] = results["hits"]

    assert hit["value"] == timeline_facts["regatta"]
    assert [c["source"] for c in hit["contributions"]] == ["text"]
    assert "decay" in hit, hit
    assert (results["start"], results["end"]) == (
        "2023-05-07T00:00:00Z",
        "2023-05-08T00:00:00Z",
    )


def test_plain_query_carries_no_decay_and_no_when(
    query, timeline_graph, timeline_keyword
):
    """A /q response holds neither a hit's decay nor the when a bound read.

    Both explain a ranking, and an agent reading memories pays for every key.
    The recall runs the same bound an explain would show under results.when.
    """
    status, body = query(
        f"recall@{timeline_graph} {timeline_keyword} until:(when regatta)"
    )
    assert status == 200, body.get("error")
    results = body["results"]
    assert results["hits"], "the bound keeps the drills"

    assert "when" not in results, results
    for hit in results["hits"]:
        assert "decay" not in hit, hit


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
