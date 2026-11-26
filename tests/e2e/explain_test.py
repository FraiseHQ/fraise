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

"""Explained recall.

A recall written with the explain prefix, `explain recall ...` on /api/v1/q,
runs exactly as the recall it prefixes and answers in the explained envelope.
Each hit carries an explain block: whether it seeded the recall, the share of
its score its age left it, and the contributions its score was folded from.
The results carry the query's: the background rate, the admission bar per
depth, the facts each source seeded, the terms the text index read, the time
each source took and, for each time bound written as a when, that when's
answer. A plain recall carries neither block.

Through the fraise mcp bridge, an explained recall is the same JSON, and its
text, the half a model reads, is that JSON rendered with the explanation in
it.

The deprecated /api/v1/explain alias answers a recall as the prefix does, and
a when as the explained recall it runs. It is served through 0.4.x and
removed in 0.5.0, and the alias tests at the end of this file go with it.
"""

import pytest


def test_explain_breaks_down_each_hit_by_source(query, pulsar_graph, pulsar_entity):
    """Every explained hit carries its seed mark and its contribution records.

    Each hit is a text seed, and its block says so. Its text record has the
    source name, the raw BM25 mass, the rank in the text list, and a count (1
    for a seed sighting). The recall names the shared entity, so each hit also
    carries one graph record through it, marked named: the entity's degree,
    both seeds' mass and their count. No contribution carries a hop field.
    """
    status, body = query(
        f"explain recall@{pulsar_graph} pulsar entity:{pulsar_entity} depth:2"
    )
    assert status == 200, body.get("error")
    hits = body["results"]["hits"]
    assert len(hits) == 2, f"want both pulsar facts, got {hits}"

    for hit in hits:
        assert hit["explain"]["seed"] is True, f"each pulsar fact is a seed: {hit}"
        contributions = hit["explain"]["contributions"]
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


def test_explain_reports_the_query_as_the_search_read_it(
    query, pulsar_graph, pulsar_entity
):
    """The query's block holds the seeds, terms, rate and bars the search used.

    The two pulsar facts are the only text seeds and nothing else seeds, so
    the counts are exact, every source listed. The text index read the one
    term. The named entity is the only anchor the round touched, so the
    background is that anchor's observed mass per unit of its degree, the
    figures its graph record carries. The admission bar is twice that rate at
    depth 1 and the rate itself at depth 2, both reported by the one round.
    Every source has a timing in whole nanoseconds, and the two that did not
    run, the vector reading and the anchor seeding, took none.
    """
    status, body = query(
        f"explain recall@{pulsar_graph} pulsar entity:{pulsar_entity} depth:2"
    )
    assert status == 200, body.get("error")
    explained = body["results"]["explain"]
    [graph] = [
        c
        for c in body["results"]["hits"][0]["explain"]["contributions"]
        if c["source"] == "graph"
    ]

    assert explained["seeds"] == {"text": 2, "vector": 0, "anchor": 0, "graph": 0}
    assert explained["terms"] == ["pulsar"]
    background = explained["background"]
    assert background == pytest.approx(graph["score"] / graph["degree"]), (
        f"a lone anchor's mass per degree is the background: {graph}, {explained}"
    )
    assert explained["admission"] == {"1": 2 * background, "2": background}
    timings = explained["timings"]
    assert set(timings) == {"text", "vector", "anchor", "graph"}, timings
    assert all(isinstance(t, int) and t >= 0 for t in timings.values()), timings
    assert timings["vector"] == timings["anchor"] == 0, timings


@pytest.mark.parametrize("depth", [1, 2])
def test_explain_shows_transmitted_surplus(
    query,
    storm_graph,
    storm_cluster_topic,
    door_topic,
    storm_silent_member,
    storm_hub,
    depth,
):
    """Funded case: the cluster's silent member is admitted by transmission.

    The member holds no query term, so it is no seed, and its one
    contribution is a graph record naming its funding anchor: the cluster's
    topic as via, the anchor's degree, its observed mass and its funding-seed
    count. The recall enters through the door, so the cluster is discovered,
    not named. Its record carries no named mark, and its mass clears its
    degree times the admission bar of the depth written, even depth 1's
    stricter bar. This proves that an anchor passes on surplus, not
    reachability. The archive hub's memos stay out. The hub holds no more
    than its fair share, so it transmits nothing. Every other hit holds a
    query term and is one of the three text seeds, and the text index read
    both terms as their stems.
    """
    status, body = query(
        f"explain recall@{storm_graph} barometer storm topic:{door_topic} "
        f"top:20 depth:{depth}"
    )
    assert status == 200, body.get("error")
    explained = body["results"]["explain"]
    hits = {h["value"]: h["explain"] for h in body["results"]["hits"]}

    calm = hits.get(storm_silent_member)
    assert calm is not None, (
        f"want the cluster's silent member funded, got {list(hits)}"
    )
    assert calm["seed"] is False, "a fact with no query term is admitted, not seeded"
    [contribution] = calm["contributions"]
    assert contribution["source"] == "graph"
    assert contribution["via"] == storm_cluster_topic, contribution
    assert contribution["degree"] == 3, contribution
    assert contribution["count"] == 2, "two seeds fund the storm cluster"
    assert contribution["score"] > 0, "the observation carries the anchor's mass"
    assert "named" not in contribution, "the cluster was discovered, not named"
    bar = explained["admission"][str(depth)]
    assert contribution["score"] > contribution["degree"] * bar, (
        f"the cluster transmitted at depth {depth} below its bar {bar}: {contribution}"
    )

    for memo in storm_hub[1:]:
        assert memo not in hits, f"fair-share hub memo {memo!r} rode in on size alone"
    seeds = {value for value, block in hits.items() if block["seed"]}
    assert seeds == set(hits) - {storm_silent_member}, hits
    assert explained["seeds"] == {"text": 3, "vector": 0, "anchor": 0, "graph": 0}
    assert explained["terms"] == ["baromet", "storm"], "the index reads stems"


def test_explain_marks_a_named_hub(
    query, storm_graph, storm_hub_topic, storm_cluster_topic, storm_hub
):
    """Naming the archive hub lets it speak, and the payload says why.

    Discovered, the hub holds no more than its fair share and stays silent
    (above): its mass is at most its degree times depth 2's bar, the lower
    one, so only its name lets it transmit. Named, it owes no fair share: its
    memos ride in behind its matching fact, each admitted by transmission
    through one graph contribution marked named. The weather cluster,
    discovered beside it, still earns its own transmission unmarked. The
    round touched those two anchors and no other, so the background is their
    observed mass over their summed degree.
    """
    status, body = query(
        f"explain recall@{storm_graph} barometer storm topic:{storm_hub_topic} "
        "top:20 depth:2"
    )
    assert status == 200, body.get("error")
    explained = body["results"]["explain"]
    hits = {h["value"]: h["explain"] for h in body["results"]["hits"]}

    for memo in storm_hub[1:]:
        assert memo in hits, f"named hub memo {memo!r} missing from {list(hits)}"
        assert hits[memo]["seed"] is False, "a memo is admitted, not seeded"
        [contribution] = hits[memo]["contributions"]
        assert contribution["via"] == storm_hub_topic, contribution
        assert contribution["named"] is True, contribution
    [hub] = hits[storm_hub[1]]["contributions"]
    assert hub["score"] <= hub["degree"] * explained["admission"]["2"], (
        f"the hub clears its own bar, so its name proves nothing: {hub}"
    )

    cluster = [
        c
        for h in hits.values()
        for c in h["contributions"]
        if c.get("via") == storm_cluster_topic
    ]
    assert cluster, "the weather cluster still transmits beside the named hub"
    assert not any(c.get("named") for c in cluster), cluster
    weather = cluster[0]
    assert explained["background"] == pytest.approx(
        (weather["score"] + hub["score"]) / (weather["degree"] + hub["degree"])
    ), f"background {explained['background']} from {weather} and {hub}"


def test_explain_score_recomputes_from_payload(
    query, storm_graph, storm_hub_topic, storm_hub, information_scores
):
    """Every hit's score recomputes from the payload.

    With the background rate in the query's block, every hit's score equals
    the information fold applied to the payload, times the decay in the hit's
    block. The decay is on the payload because nothing else there says how
    much a fact's age cost it, and without it the score would recompute only
    to the precision of the time since the write. The fold measures each fact
    against the recall's whole pool, so the recall returns every candidate,
    and the payload is then a complete explanation, not a summary. The recall
    names the hub and discovers the cluster, so both kinds of graph term are
    recomputed: the hub's memos carry graph contributions marked named, owing
    no fair share, while the cluster funds its silent member unmarked.
    """
    status, body = query(
        f"explain recall@{storm_graph} barometer storm topic:{storm_hub_topic} top:1000"
    )
    assert status == 200, body.get("error")
    results = body["results"]
    background = results["explain"]["background"]
    assert background > 0, "two anchors are touched; the null rate must be positive"
    hits = results["hits"]
    graph = [
        c
        for hit in hits
        for c in hit["explain"]["contributions"]
        if c["source"] == "graph"
    ]
    assert any(c.get("named") for c in graph), graph
    assert not all(c.get("named") for c in graph), graph
    assert {h["value"] for h in hits} >= set(storm_hub), "the named hub lifts its memos"

    for hit, fold in zip(hits, information_scores(results), strict=True):
        decay = hit["explain"]["decay"]
        want = fold * decay
        assert abs(hit["score"] - want) < 1e-9 * max(1.0, want), (
            f"{hit['value']!r}: score {hit['score']} != recomputed {want} "
            f"from {hit['explain']['contributions']} at background {background} "
            f"and decay {decay}"
        )


def test_explain_of_anchors_alone_seeds_every_member(
    query, pulsar_graph, pulsar_entity
):
    """With no term and no vector, the named anchor's members are the seeds.

    Each pulsar fact is a seed through one anchor record naming the entity,
    and the two are counted under the anchor source. Even at depth 2 no round
    runs, since expanding from every member would return most of the graph:
    the block has no admission bar, the background is zero, and the graph
    source took no time, nor did the readings that never ran. The recall has
    no terms, and the list is empty rather than missing, so a caller reads
    one shape.
    """
    status, body = query(
        f"explain recall@{pulsar_graph} entity:{pulsar_entity} depth:2"
    )
    assert status == 200, body.get("error")
    explained = body["results"]["explain"]
    hits = body["results"]["hits"]
    assert len(hits) == 2, f"want both pulsar facts, got {hits}"

    for hit in hits:
        assert hit["explain"]["seed"] is True, f"an anchor's member seeds: {hit}"
        [contribution] = hit["explain"]["contributions"]
        assert contribution["source"] == "anchor", contribution
        assert contribution["via"] == pulsar_entity, contribution
    assert explained["seeds"] == {"text": 0, "vector": 0, "anchor": 2, "graph": 0}
    assert explained["terms"] == []
    assert "admission" not in explained, explained
    assert explained["background"] == 0
    timings = explained["timings"]
    assert timings["text"] == timings["vector"] == timings["graph"] == 0, timings


def test_explain_lists_the_terms_the_text_index_read(query, pulsar_graph):
    """The terms are the query's words as the text index read them.

    The stop word is dropped, as the recall's warning says, and "pulsars" is
    read as its stem, once, though the stem is written twice. A word no stored
    fact holds is listed too, first as it was written first: that is how a
    caller sees why a word matched nothing. The recall names no anchor, so no
    round runs: the block has no admission bar, the background is zero and
    the graph source took no time.
    """
    status, body = query(
        f"explain recall@{pulsar_graph} zzznomatchzzz the pulsars pulsar"
    )
    assert status == 200, body.get("error")
    explained = body["results"]["explain"]

    assert explained["terms"] == ["zzznomatchzzz", "pulsar"]
    assert explained["seeds"] == {"text": 2, "vector": 0, "anchor": 0, "graph": 0}
    [warning] = body["warnings"]
    assert 'term "the" is a stop word' in warning, warning
    assert "admission" not in explained, explained
    assert explained["background"] == 0
    assert explained["timings"]["graph"] == 0, explained["timings"]


def test_plain_recall_carries_no_explain_block(query, pulsar_graph, pulsar_entity):
    """A plain recall answers with no explain key at either level.

    Its results hold the count and the hits, and each hit its value,
    timestamp and score, nothing more. This guards against the explained
    envelope leaking into every recall and bloating agent token budgets, and
    keeps the key's presence the one sign of an explained recall.
    """
    status, body = query(f"recall@{pulsar_graph} pulsar entity:{pulsar_entity} depth:2")
    assert status == 200, body.get("error")
    results = body["results"]
    assert set(results) == {"count", "hits"}, (
        f"a plain recall must not carry the query's block: {results}"
    )
    assert results["hits"], "the pulsar facts must be recallable"
    for hit in results["hits"]:
        assert set(hit) == {"value", "timestamp", "score"}, (
            f"a plain hit must not carry the explain block: {hit}"
        )


def test_explain_carries_each_hits_decay_from_when_it_was_written(
    query, timeline_graph, timeline_keyword, timeline_facts
):
    """Every explained hit's block carries its decay, and a fact ages from its write.

    The decay is the share of its score a fact keeps at its age. The timeline's
    facts were all written moments ago, so each keeps nearly all of it,
    whatever date the fact was said or happened on. Aged from May 2023 instead,
    at the suite's 2160h half-life, they would have halved some fourteen times
    and kept less than a thousandth. The decay explains a ranking, so it rides
    in the hit's block and nowhere beside it.
    """
    status, body = query(f"explain recall@{timeline_graph} {timeline_keyword}")
    assert status == 200, body.get("error")
    hits = body["results"]["hits"]
    assert len(hits) == len(timeline_facts), hits

    for hit in hits:
        decay = hit["explain"]["decay"]
        assert 0.99 < decay <= 1, f"{hit['value']!r}: decay {decay}"
        assert "decay" not in hit, f"the decay left the hit's block: {hit}"


def test_explain_carries_the_answer_of_each_when_a_bound_read(
    query, timeline_graph, timeline_keyword, timeline_facts
):
    """An explained recall shows the when behind each of its bounds.

    A bound written as a when is a time the caller never saw: a search of its
    own found a fact, and the fact's time became the bound. So the query's
    block holds that search under when, by the name of the bound that read
    it. Each entry is the when's own results, explained as any recall is: the
    fact it kept with that hit's block, the start and end it answered with,
    and the block of its own search, which read the when's term, not the
    recall's. since: opened the window at the end of the regatta and until:
    closed it at the start of the cruise, which leaves the protest.
    """
    status, body = query(
        f"explain recall@{timeline_graph} {timeline_keyword} "
        "since:(when regatta) until:(when cruise)"
    )
    assert status == 200, body.get("error")
    results = body["results"]
    assert [hit["value"] for hit in results["hits"]] == [timeline_facts["protest"]]
    assert "when" not in results, "a when's answer rides in the query's block"
    when = results["explain"]["when"]
    assert set(when) == {"since", "until"}

    since = when["since"]
    assert [hit["value"] for hit in since["hits"]] == [timeline_facts["regatta"]]
    assert (since["start"], since["end"]) == (
        "2023-05-07T00:00:00Z",
        "2023-05-08T00:00:00Z",
    )
    assert since["explain"]["terms"] == ["regatta"], since["explain"]

    until = when["until"]
    assert [hit["value"] for hit in until["hits"]] == [timeline_facts["cruise"]]
    assert (until["start"], until["end"]) == (
        "2023-05-10T00:00:00Z",
        "2023-05-17T00:00:00Z",
    )
    assert until["explain"]["terms"] == ["cruis"], "the index reads stems"

    for hit in since["hits"] + until["hits"]:
        assert hit["explain"]["contributions"], (
            f"the when's own hit is unexplained: {hit}"
        )
        assert "decay" in hit["explain"], hit


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
    clause, bounds, query, timeline_graph, timeline_keyword
):
    """The query's block has a when entry for a bound that held a when, and no other.

    A bound written as a date explains itself, so it has no entry. A recall
    with no when in either bound has no when key at all.
    """
    status, body = query(
        f"explain recall@{timeline_graph} {timeline_keyword} {clause}".rstrip()
    )
    assert status == 200, body.get("error")
    explained = body["results"]["explain"]

    assert sorted(explained.get("when", {})) == bounds, explained.get("when")
    assert ("when" in explained) == bool(bounds), explained


def test_explain_shows_the_when_that_matched_nothing(
    query, timeline_graph, timeline_keyword
):
    """A bound left unread is explained by the empty answer of its when.

    The recall returns no hits, and the warning says why. The payload shows
    the same thing: the when under the bound's name, with no hits and no
    time, its own search having read its one term and seeded nothing. The
    recall around it had no bound to read, so it never searched, and its
    block says so: no seed, no term read, no time taken. Only the time the
    when's search took moves between runs, so it alone is set aside.
    """
    status, body = query(
        f"explain recall@{timeline_graph} {timeline_keyword} since:(when zzznomatchzzz)"
    )
    assert status == 200, body.get("error")
    timings = body["results"]["explain"]["when"]["since"]["explain"].pop("timings")
    assert set(timings) == {"text", "vector", "anchor", "graph"}, timings
    zeros = {"text": 0, "vector": 0, "anchor": 0, "graph": 0}
    assert body == {
        "results": {
            "count": 0,
            "hits": [],
            "explain": {
                "background": 0,
                "seeds": zeros,
                "terms": [],
                "timings": zeros,
                "when": {
                    "since": {
                        "count": 0,
                        "hits": [],
                        "explain": {
                            "background": 0,
                            "seeds": zeros,
                            "terms": ["zzznomatchzzz"],
                        },
                    },
                },
            },
        },
        "warnings": [
            "since: the when matched no fact, so it has no time to answer with; "
            "the recall has no bound to read and returns no hits"
        ],
    }


def test_plain_recall_carries_no_decay_and_no_when(
    query, timeline_graph, timeline_keyword
):
    """A plain recall holds neither a hit's decay nor the when a bound read.

    Both explain a ranking, so both ride only in the explain blocks, and an
    agent reading memories pays for every key. The recall runs the same bound
    an explained recall shows in its query's block.
    """
    status, body = query(
        f"recall@{timeline_graph} {timeline_keyword} until:(when regatta)"
    )
    assert status == 200, body.get("error")
    results = body["results"]
    assert results["hits"], "the bound keeps the drills"

    assert "explain" not in results, results
    assert "when" not in results, results
    for hit in results["hits"]:
        assert "explain" not in hit, hit
        assert "decay" not in hit, hit


def test_explain_remember_is_rejected_at_the_verb_and_lands_nothing(query):
    """`explain remember` is a 400 at the verb, and nothing is stored.

    A write has no ranking to explain, and probing a ranking must never
    mutate a graph. The parser blames the verb the prefix stands before, so
    the remember never runs: a later recall of its one distinctive word finds
    nothing.
    """
    status, body = query(
        "explain remember@2 'explainprobe should never land' topic:explainprobe"
    )
    assert status == 400, body
    assert body["error"] == (
        'parse error at column 16: "remember" has no ranking to explain: '
        "the explain prefix stands only before recall — drop the prefix"
    )

    status, body = query("recall@2 explainprobe")
    assert status in (200, 204), body.get("error")
    assert body.get("results", {}).get("count", 0) == 0, (
        "a rejected explain remember landed on graph 2"
    )


@pytest.mark.parametrize(
    "template",
    [
        "explain recall@{pulsar_graph} pulsar entity:{pulsar_entity} depth:2",
        "explain recall@{pulsar_graph} entity:{pulsar_entity} depth:2",
        "explain recall@{storm_graph} barometer storm topic:{door_topic} top:20 depth:2",
        "explain recall@{timeline_graph} {timeline_keyword} top:20",
        "explain recall@{timeline_graph} {timeline_keyword} "
        "since:(when regatta) until:(when cruise)",
        "explain recall@{timeline_graph} {timeline_keyword} since:(when zzznomatchzzz)",
        "recall@{timeline_graph} {timeline_keyword} top:20",
    ],
)
def test_the_bridge_renders_the_json_the_server_answers(
    template,
    query,
    bridge_recall,
    rendered,
    without_the_clock,
    pulsar_graph,
    pulsar_entity,
    storm_graph,
    door_topic,
    timeline_graph,
    timeline_keyword,
):
    """Through fraise mcp, a recall is the server's JSON, then that JSON rendered.

    The bridge's structured content is the response the server gives the same
    recall over HTTP, all but the score, decay and timings, which move with
    the clock between the two runs. Its text is that JSON rendered: the
    query's explanation above the facts, each fact's dates, then its own
    explanation under it, and the warnings last. The text is what a model
    reads, and most clients hand it nothing else, so an explanation the JSON
    carries and the text drops never reaches the reader it was asked for.
    That is how explain shipped once: right in the JSON, absent from the text,
    and no test read the text. The cases cover text and named graph
    sightings, anchors seeding alone, transmission through an unnamed anchor,
    every date form the timeline holds, the when each bound read, a when that
    matched nothing, and a plain recall, which carries no explanation at all.
    """
    text = template.format(
        pulsar_graph=pulsar_graph,
        pulsar_entity=pulsar_entity,
        storm_graph=storm_graph,
        door_topic=door_topic,
        timeline_graph=timeline_graph,
        timeline_keyword=timeline_keyword,
    )
    status, body = query(text)
    assert status == 200, body.get("error")

    found = bridge_recall(text)
    relayed = found["structuredContent"]
    assert without_the_clock(relayed) == without_the_clock(body)

    explained = text.startswith("explain ")
    assert ("explain" in relayed["results"]) == explained, relayed
    assert all(("explain" in hit) == explained for hit in relayed["results"]["hits"])
    assert found["content"][0]["text"] == rendered(relayed)


@pytest.mark.parametrize("prefix", ["", "explain "])
def test_alias_answers_a_recall_as_the_explain_prefix_does(
    query, explain_alias, storm_graph, storm_hub_topic, prefix
):
    """The deprecated alias answers a recall exactly as `explain recall` on /q.

    Written with the prefix or without it, the recall comes back from the
    alias with the same hits in the same order, the same explain blocks and
    no warning of its own, so a caller moving off the alias reads the
    envelope it already reads. Only what moves between two requests is set
    aside: the time each source took, and each fact's recency decay, in its
    block and in its score. Goes with the alias in 0.5.0.
    """
    statement = (
        f"recall@{storm_graph} barometer storm topic:{storm_hub_topic} top:20 depth:2"
    )
    status, want = query(f"explain {statement}")
    assert status == 200, want.get("error")
    status, got = explain_alias(f"{prefix}{statement}")
    assert status == 200, got.get("error")

    scores, decays = [], []
    for body in (want, got):
        timings = body["results"]["explain"].pop("timings")
        assert set(timings) == {"text", "vector", "anchor", "graph"}, timings
        hits = body["results"]["hits"]
        scores.append([hit.pop("score") for hit in hits])
        decays.append([hit["explain"].pop("decay") for hit in hits])
    assert got == want
    assert scores[1] == pytest.approx(scores[0], rel=1e-6)
    assert decays[1] == pytest.approx(decays[0], rel=1e-6)


def test_alias_explains_a_when_as_the_recall_it_runs(
    explain_alias, timeline_graph, timeline_facts
):
    """The deprecated alias explains a when as the recall it runs.

    A when is a recall that answers with a time. The hit it read its time
    from carries its block, contributions and decay, the results keep the
    start and end, and the query's block holds what the when's search read.
    On /q the prefix stands only before recall, so this answer is the
    alias's alone. Goes with the alias in 0.5.0.
    """
    status, body = explain_alias("when regatta")
    assert status == 200, body.get("error")
    results = body["results"]
    [hit] = results["hits"]

    assert hit["value"] == timeline_facts["regatta"]
    assert [c["source"] for c in hit["explain"]["contributions"]] == ["text"]
    assert "decay" in hit["explain"], hit
    assert (results["start"], results["end"]) == (
        "2023-05-07T00:00:00Z",
        "2023-05-08T00:00:00Z",
    )
    assert results["explain"]["terms"] == ["regatta"], results["explain"]


def test_alias_rejects_a_write_and_lands_nothing(query, explain_alias):
    """The deprecated alias refuses a remember with a 400, and nothing is stored.

    A write has no ranking to explain, and probing a ranking must never
    mutate a graph, so the alias answers a remember it parsed with its own
    error rather than running it. Goes with the alias in 0.5.0.
    """
    status, body = explain_alias(
        "remember@2 'explainprobe should never land' topic:explainprobe"
    )
    assert status == 400, body
    assert body == {"error": "explain applies to recall queries only"}

    status, body = query("recall@2 explainprobe")
    assert status in (200, 204), body.get("error")
    assert body.get("results", {}).get("count", 0) == 0, (
        "a remember sent to the alias landed on graph 2"
    )


@pytest.mark.parametrize(
    "bad_query",
    [
        "recall",  # no seed
        "explain me",  # the prefix before no command
        "explain explain recall x",  # two prefixes
        "explain remember@2 'explainprobe should never land'",  # a prefixed write
        "explain when regatta",  # the prefix before a when
    ],
)
def test_alias_rejects_an_unparsable_query_as_q_does(query, explain_alias, bad_query):
    """A statement the parser rejects is the same 400 on the alias as on /q.

    The two routes share one parser, so the alias adds no error of its own,
    even to a statement that already carries the prefix. Goes with the alias
    in 0.5.0.
    """
    status, body = explain_alias(bad_query)
    assert status == 400, body
    assert (status, body) == query(bad_query)
