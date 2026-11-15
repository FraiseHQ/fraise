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

"""Remember/recall semantics, end to end.

Covers the store-then-find round trip, how anchors transmit or stay silent, how
top shapes a recall's results, and text-index matching across facts. It also
covers recall through topic:/entity: anchors, including anchors whose value is
a grammar keyword, and the case folding of terms and anchor values. The last
section covers the three times a fact holds: which of them a time bound reads,
and the when command, which answers with one.
"""

import pytest


def test_recall_on_empty_graph(query):
    """A recall that matches nothing answers 200 with an empty hit list.

    Graph 0 is primed, so this is a miss on a populated graph. It is not the
    204 of a graph holding nothing, which api_test.py pins.
    """
    status, body = query("recall nothingindexedyet")
    assert status == 200
    results = body["results"]
    assert results is not None
    assert results["count"] == 0
    assert results["hits"] == []


def test_recall_with_clauses(query):
    """A recall with anchor, top and depth clauses together answers 200."""
    status, body = query("recall@2 anna bob entity:alice topic:job top:10 depth:2")
    assert status == 200
    assert body["results"] is not None


def test_remember_is_accepted(query):
    """A remember with a topic and an entity is accepted with a 200."""
    status, _ = query(
        "remember@1 'anne loves the color orange' topic:color entity:anne"
    )
    assert status == 200


def test_remember_then_recall(query):
    """The real end-to-end round trip: store a fact, then find it."""
    status, body = query("remember@3 'the parrot is turquoise' topic:color")
    assert status == 200, body.get("error")

    status, body = query("recall@3 parrot")
    assert status == 200, body.get("error")
    assert body["results"]["count"] > 0, (
        "recall found nothing, want the remembered fact"
    )


@pytest.mark.parametrize(
    ("marker", "value"),
    [
        ("freeflowplain", "a plain sentence with spaces"),
        ("freeflowpunct", "punctuation, semicolons; and dashes - like this"),
        ("freeflowsymbols", "digits 1234 and symbols @ # % $ ( ) : mixed in"),
        ("freeflowquotes", "it's got an apostrophe, and rock 'n' roll has two"),
        ("freeflowaccents", "le barometre chute avant la tempete"),
        ("freeflowlines", "line one\nline two\r\n\tindented"),
        ("freeflowunicode", "déjà vu 😀 東京"),
        ("freeflowbackslash", 'C:\\temp\\new says "hi"'),
        ("freeflownul", "a nul\x00survives json transport"),
        ("freeflowjson", '{"looks": ["like", "json"]} - [markdown](too)'),
        ("freeflowreserved", "recall the top topic since until depth vec entity"),
        ("freeflowlong", "a" * 300),
    ],
)
def test_free_flowing_text_round_trips(query, marker, value):
    """Ingested prose is stored and returned byte-for-byte over raw HTTP.

    Inside a quoted phrase every character is literal. Reserved words and
    symbols carry no meaning, and a doubled quote escapes an apostrophe.
    Newlines, control characters, emoji and backslashes are all data. That
    includes a NUL, which JSON delivers as \\u0000. The SDK's integration tests
    parse the same battery (query_test.py). This test asserts more: each value
    is recalled by the marker word stored beside it and must come back exactly
    as written.

    Facts are keyed by their value, so the writes stay idempotent across reruns
    against a long-lived server.
    """
    fact = f"{value} {marker}"
    status, body = query(
        "remember@1 '{}' topic:freeflow".format(fact.replace("'", "''"))
    )
    assert status == 200, body.get("error")

    status, body = query(f"recall@1 {marker}")
    assert status == 200, body.get("error")
    values = [h["value"] for h in body["results"]["hits"]]
    assert fact in values, f"want the text back verbatim, got {values}"


def test_recall_question_travels_as_a_quoted_phrase(query):
    """A natural-language question travels as one quoted phrase term.

    Clauses follow the phrase. Reserved words like "topic" stay literal inside
    the quotes. Bare question words would collide with the grammar's keywords
    and are rejected. The quoted form is the supported shape.
    """
    status, body = query(
        "recall@1 'what topic has jules been blogging about recently' top:10"
    )
    assert status == 200, body.get("error")
    assert body["results"] is not None


def test_recall_depth_selects_a_lane(planets_graph, door_topic, recall_count):
    """Every depth gives this query the same answer, for different reasons.

    depth picks the retrieval lane. The recall names the door, an anchor beside
    a term, so it runs at depth:1 at least: a written depth:0 runs as depth:1.
    depth:1 and depth:2 run the anchor round. They return the seed alone. The
    star's topic hub is discovered, not named, and a lone seed's hub sits
    exactly at the background rate, so it transmits nothing at either admission
    bar. Omitting the clause uses the configured default. This test cannot tell
    a silent hub from a graph channel that never ran. The lantern tests below
    separate the lanes by their results.
    """
    g = planets_graph
    for clause in ("depth:0", "depth:1", "depth:2", ""):
        text = f"recall@{g} mercury topic:{door_topic} {clause}".strip()
        assert recall_count(text) == 1, f"{text}: a fair-share hub must not transmit"


def test_floor_lane_beside_an_anchor_runs_as_depth_one(
    lantern_graph, lantern_silent, query
):
    """depth:0 beside an anchor runs as depth:1, and the response says so.

    An anchor beside a term takes part only through what it transmits, and the
    floor lane transmits nothing. So the recall runs the anchor round instead:
    the cluster's silent member, which carries no query term, is funded. The
    warning sits beside the hits, at the depth clause.
    """
    status, body = query(
        f"recall@{lantern_graph} lantern topic:lanterns depth:0 top:20"
    )
    assert status == 200, body.get("error")

    values = [hit["value"] for hit in body["results"]["hits"]]
    assert lantern_silent in values, (
        f"depth:0 did not run as depth:1: {lantern_silent!r} unfunded; got {values}"
    )
    warnings = body.get("warnings") or []
    assert any("depth:0 ran as depth:1" in w for w in warnings), warnings


@pytest.mark.parametrize("clause", ["depth:1", "depth:2"])
def test_graph_lanes_transmit_to_a_fact_the_floor_cannot_reach(
    lantern_graph, lantern_silent, query, clause
):
    """depth:1 and depth:2 run the round, and their hits show it.

    The silent member arrives funded by its cluster's transmission alone. The
    recall names only the cluster. The almanac hub is discovered, and holds no
    more than its fair share of the query's mass across eight members. It
    transmits nothing, so its entries stay out.

    The cluster is named, so it clears both admission bars whatever its share.
    depth:1 (precision) and depth:2 (max recall) differ on a discovered anchor
    between one and two times its fair share. That needs mass arithmetic too
    delicate to pin over HTTP. TestSearchDepthOnePrecisionBar covers it.
    """
    status, body = query(
        f"recall@{lantern_graph} lantern topic:lanterns {clause} top:20"
    )
    assert status == 200, body.get("error")

    values = [hit["value"] for hit in body["results"]["hits"]]
    assert lantern_silent in values, (
        f"{clause} did not fund {lantern_silent!r}; got {values}"
    )
    assert not any(v.startswith("unrelated almanac entry") for v in values), (
        f"a fair-share hub transmitted to its members: {values}"
    )


def test_recall_top_truncates_results(planets_graph, planet_facts, recall_count):
    """top caps the number of ranked results and never pads.

    All four facts contain "planet", so the text index matches every one
    directly.
    """
    g = planets_graph
    n = len(planet_facts)

    assert recall_count(f"recall@{g} planet top:1") == 1
    assert recall_count(f"recall@{g} planet top:2") == 2
    assert recall_count(f"recall@{g} planet top:3") == 3
    assert recall_count(f"recall@{g} planet top:10") == n


def test_recall_unique_keyword_returns_only_its_fact(
    planets_graph, planet_facts, query
):
    """A recall for one fact's unique keyword returns exactly that fact.

    It names no anchor, so only the text index answers and the fact's siblings
    stay out.
    """
    g = planets_graph
    status, body = query(f"recall@{g} mercury")
    assert status == 200, body.get("error")
    values = [hit["value"] for hit in body["results"]["hits"]]
    assert values == [planet_facts["mercury"]]


def test_recall_returns_every_document_sharing_a_keyword(query, comet_facts):
    """A recall by a shared keyword returns every fact containing it, not just one."""
    graph = 0
    for phrase, topic in comet_facts.items():
        status, body = query(f"remember@{graph} '{phrase}' topic:{topic}")
        assert status == 200, body.get("error")

    status, body = query(f"recall@{graph} comet top:10")
    assert status == 200, body.get("error")

    values = {hit["value"] for hit in body["results"]["hits"]}
    assert set(comet_facts) <= values, (
        "recall by a shared keyword must return every matching fact, not just "
        f"one; want all of {set(comet_facts)}, got {values}"
    )


def test_recall_unions_matches_across_keywords(query):
    """A recall naming several keywords returns the union of their matches.

    The two facts share no word and carry different topics, so nothing links
    them in the graph. "saturn" matches only the first, and "neptune" only the
    second. Recalling both keywords must return both facts. Graph 2's other
    facts (the primer and the explain probes) contain neither word.
    """
    graph = 2
    facts = {
        "saturn has bright rings": "rings",
        "neptune is a deep blue giant": "orbit",
    }
    for phrase, topic in facts.items():
        status, body = query(f"remember@{graph} '{phrase}' topic:{topic}")
        assert status == 200, body.get("error")

    status, body = query(f"recall@{graph} saturn neptune top:10")
    assert status == 200, body.get("error")

    values = {hit["value"] for hit in body["results"]["hits"]}
    assert set(facts) <= values, (
        f"recall across two keywords should union both matches; want {set(facts)}, got {values}"
    )


def test_recall_by_emoji_finds_the_fact(query):
    """A fact whose salient content is an emoji is recallable by that emoji.

    The text index keeps symbols (Unicode category So: emoji, check marks and
    the like) as terms of their own. So ``recall '🍊'`` reaches the fruit fact
    through the index, as any word would. An emoji is not a letter, so the
    grammar takes it quoted. A tokenizer that kept only letters and digits
    would store the fact verbatim but index it under no term, so it could never
    be found. Graph 6 otherwise holds prose only, so the emoji term is unique
    to this fact and the hit list is exact.
    """
    fact = "🍊 🍋 🍌"
    status, body = query(f"remember@6 '{fact}' topic:fruit")
    assert status == 200, body.get("error")

    status, body = query("recall@6 '🍊'")
    assert status == 200, body.get("error")
    values = [hit["value"] for hit in body["results"]["hits"]]
    assert values == [fact], f"recall by emoji should find the fruit fact; got {values}"


def test_recall_with_anchor_filters_returns_tagged_fact(query):
    """A fact written with topic:/entity: anchors is recallable through them.

    The write stores the Topic and NamedEntity nodes as well as the edges to
    them. Anchored recalls resolve a fact's anchors through its stored
    neighbours. An edge to an unstored anchor would make every anchored recall
    filter everything out.
    """
    graph = 5
    status, body = query(
        f"remember@{graph} 'ulysse moved to quimper' topic:relocation entity:ulysse"
    )
    assert status == 200, body.get("error")

    for q in (
        f"recall@{graph} quimper",
        f"recall@{graph} quimper topic:relocation",
        f"recall@{graph} quimper entity:ulysse",
    ):
        status, body = query(q)
        assert status == 200, body.get("error")
        hits = body["results"]["hits"]
        assert len(hits) == 1, f"{q!r} -> {body['results']}"
        assert hits[0]["value"] == "ulysse moved to quimper"


def test_keyword_anchor_values_round_trip(query, cairn_fact):
    """A fact filed under entity:top and topic:top is reachable through both.

    The remember alone proves the parse. The anchored recalls prove the anchors
    landed as the word "top". They were not dropped or misread as a
    result-limit clause. Each filter must narrow the marker recall to exactly
    this fact.
    """
    status, body = query(f"remember@5 '{cairn_fact}' topic:top entity:top")
    assert status == 200, body.get("error")

    for q in (
        "recall@5 cairnprobe entity:top",
        "recall@5 cairnprobe topic:top",
    ):
        status, body = query(q)
        assert status == 200, body.get("error")
        hits = body["results"]["hits"]
        assert len(hits) == 1, f"{q!r} -> {body['results']}"
        assert hits[0]["value"] == cairn_fact


def test_keyword_recalls_as_a_leading_term(query, cairn_fact):
    """In `recall 'top' top:10`, the first "top" is a search term and the
    second is the result-limit clause.

    A bare keyword among a recall's terms is rejected, so a term that spells
    one is quoted. A keyword immediately followed by ':' is always a clause.
    The term must then reach the cairnprobe fact through the text index, like
    any other word, since its text contains "top".
    """
    status, body = query(f"remember@5 '{cairn_fact}' topic:top entity:top")
    assert status == 200, body.get("error")

    status, body = query("recall@5 'top' top:10")
    assert status == 200, body.get("error")
    values = [hit["value"] for hit in body["results"]["hits"]]
    assert cairn_fact in values, (
        f"the leading term 'top' should match the cairnprobe fact; got {values}"
    )


def test_anchor_case_folds_while_the_fact_keeps_its_spelling(query, caseprobe_fact):
    """topic:Mountaineering, topic:MOUNTAINEERING and topic:mountaineering are
    one anchor, and the stored fact keeps its capitalisation.

    The write uses mixed-case anchors. Each recall uses a different casing of
    the term, the topic and the entity, and every one must land on the same
    single fact. If the fold regressed, one anchor would exist under as many
    nodes as clients have capitalisations. An anchored recall would then
    silently miss facts filed under another spelling. There would be no error,
    just absent memories.
    """
    status, body = query(
        f"remember@5 '{caseprobe_fact}' topic:Mountaineering entity:Karakoram"
    )
    assert status == 200, body.get("error")

    for q in (
        "recall@5 caseprobe topic:mountaineering",
        "recall@5 Caseprobe topic:MOUNTAINEERING",
        "recall@5 CASEPROBE entity:karakoram",
        "recall@5 caseprobe entity:Karakoram",
    ):
        status, body = query(q)
        assert status == 200, body.get("error")
        hits = body["results"]["hits"]
        assert len(hits) == 1, f"{q!r} -> {body['results']}"
        assert hits[0]["value"] == caseprobe_fact, (
            f"{q!r}: the fact must come back spelled exactly as written; "
            f"got {hits[0]['value']!r}"
        )


def test_identical_recalls_return_identically_ranked_hits(
    query, quasar_facts, recall_ranking
):
    """A repeated recall must rank the same facts the same way every time.

    Candidates are pooled out of maps, whose iteration order changes per call.
    So the ranking must be a total order: score descending, then fact key on a
    tie. The test does not write the expected order down. It follows from the
    scores and the configured hash function. Callers are promised only that it
    does not move.
    """
    graph = 0
    for phrase in quasar_facts:
        status, body = query(f"remember@{graph} '{phrase}'")
        assert status == 200, body.get("error")

    # The facts carry no anchor and the query names none, so the result is
    # exactly the five facts the text index matches.
    text = f"recall@{graph} quasar"
    ranking = recall_ranking(text)
    assert set(ranking) == set(quasar_facts), (
        f"{text!r} must match every quasar fact and nothing else; got {ranking}"
    )

    for call in range(2, 11):
        assert recall_ranking(text) == ranking, (
            f"call {call} of {text!r} ranked the same facts differently; "
            "recall is not reproducible"
        )


@pytest.mark.parametrize("top", [1, 2, 3])
def test_recall_top_keeps_the_head_of_the_ranking(
    top, query, quasar_facts, recall_ranking
):
    """`top` must keep the head of the ranking, not an arbitrary slice.

    Truncating before the order is total would drop whichever tied facts the
    candidate map yielded last. A top:1 answer could then name a fact other
    than the one the untruncated recall ranks first. The truncated recall must
    be a prefix of the untruncated one.
    """
    graph = 0
    for phrase in quasar_facts:
        status, body = query(f"remember@{graph} '{phrase}'")
        assert status == 200, body.get("error")

    full = recall_ranking(f"recall@{graph} quasar")
    truncated = recall_ranking(f"recall@{graph} quasar top:{top}")
    assert truncated == full[:top], (
        f"top:{top} must be the first {top} of the full ranking {full}; got {truncated}"
    )


def test_recall_depth_one_is_not_polluted_by_a_fact_named_like_a_topic(
    query, ledger_topic, ledger_facts, door_topic, recall_ranking
):
    """A fact whose text is its topic's name must not stand in for the topic.

    Recall returns facts, never the hubs it walks through. Through the door,
    depth:1 runs the anchor round from the seed. The ledger topic is the only
    anchor the seed touches, discovered rather than named, so it stays silent
    and the seed comes back alone. A "ledgerprobe" fact standing in for the
    topic would show up here as a second hit. Graph 5's other facts carry
    different topics, so nothing else is one hop from the seed.
    """
    graph = 5
    for phrase in ledger_facts:
        status, body = query(f"remember@{graph} '{phrase}' topic:{ledger_topic}")
        assert status == 200, body.get("error")

    hits = recall_ranking(f"recall@{graph} quarterly topic:{door_topic} depth:1")
    assert hits == ["acme settles invoices quarterly"], (
        f"depth:1 must return the seed alone; {ledger_facts[0]!r} reached it as a "
        f"neighbour, so it is serving as the {ledger_topic!r} topic node: {hits}"
    )


# Anchor-seeded recall. A recall naming anchors and no term or vector is seeded
# by the anchors themselves. Every fact filed under them enters the candidates,
# instead of being filtered by them. The ordinary ranking puts the newest
# first: each fact gets a unit of mass per named anchor it is filed under,
# decayed by age. The planet star is the single-anchor case. The tidepool probe
# (conftest) is the union.


def test_anchor_only_recall_returns_every_member_newest_first(
    planets_graph, planet_facts, recall_ranking
):
    """`recall topic:planets` returns the whole star, newest first.

    The topic seeds the recall. The fixture writes the facts in dict order, so
    the ranking is that order reversed. Without anchor seeding, the query would
    have no seed and return nothing. A caller could not tell that from an empty
    topic.
    """
    ranking = recall_ranking(f"recall@{planets_graph} topic:planets top:10")
    assert ranking == list(reversed(planet_facts.values())), (
        f"want the star newest first, got {ranking}"
    )


def test_anchor_seeded_hits_are_scored_by_the_ordinary_ranking(planets_graph, query):
    """An anchor-seeded recall is ranked like any other.

    Each hit carries the unit of mass its anchor lends, decayed by age. So
    scores are positive and strictly descend with the ranking. They are not
    absent or flat. The test asks for strictly descending scores, because a
    flat set would also drain from the ranker in non-increasing order. The
    star's writes are sequential round trips, so their decay factors differ by
    about a part in ten billion. The daemon resolves that, because
    tests/fraise.config.toml runs it at float64 precision.
    """
    status, body = query(f"recall@{planets_graph} topic:planets top:10")
    assert status == 200, body.get("error")
    scores = [hit["score"] for hit in body["results"]["hits"]]
    assert scores, "the star must be returned"
    assert all(s > 0 for s in scores), scores
    assert all(a > b for a, b in zip(scores, scores[1:], strict=False)), (
        f"scores must strictly descend with recency, got {scores}"
    )


def test_anchor_seeds_union_ranking_the_fact_under_both_first(
    tidepool_graph, tidepool_anchors, tidepool_facts, recall_ranking
):
    """`recall topic:tidepool entity:limpet` returns everything under either
    anchor, each fact once.

    Alone, the anchors seed rather than filter, so the result is the union.
    Beside a term, the same two clauses would narrow to facts filed under both.
    The fact filed under both anchors carries two units of seed mass. The facts
    were written moments apart, so their decay barely differs, and that fact
    ranks first. The other two follow newest first. The entity's fact was
    written after the topic's.
    """
    topic, entity = tidepool_anchors
    ranking = recall_ranking(
        f"recall@{tidepool_graph} topic:{topic} entity:{entity} top:10"
    )
    assert ranking == [
        tidepool_facts["both"],
        tidepool_facts["entity"],
        tidepool_facts["topic"],
    ], f"want the union with the doubly-filed fact first, got {ranking}"


@pytest.mark.parametrize("top", [1, 2, 3])
def test_anchor_seeded_top_keeps_the_head_of_the_ranking(
    top, planets_graph, recall_ranking
):
    """`top` truncates an anchor-seeded recall like any other: it keeps the
    head, never an arbitrary slice.
    """
    full = recall_ranking(f"recall@{planets_graph} topic:planets top:10")
    truncated = recall_ranking(f"recall@{planets_graph} topic:planets top:{top}")
    assert truncated == full[:top], (
        f"top:{top} must be the first {top} of {full}; got {truncated}"
    )


def test_anchor_seeded_recall_without_top_takes_the_configured_default(
    saltmarsh_graph, saltmarsh_facts, default_top, recall_ranking
):
    """An anchor-seeded recall with no top: clause is capped at default-top.

    The saltmarsh anchor holds more facts than the configured default. So the
    recall with no top: must come back exactly default-top long, the head of
    the ranking. A top: past the count returns every member.
    """
    assert len(saltmarsh_facts) > default_top
    full = recall_ranking(
        f"recall@{saltmarsh_graph} topic:saltmarsh top:{len(saltmarsh_facts)}"
    )
    assert sorted(full) == sorted(saltmarsh_facts)
    capped = recall_ranking(f"recall@{saltmarsh_graph} topic:saltmarsh")
    assert capped == full[:default_top], (
        f"want the first {default_top} of {full}; got {capped}"
    )


@pytest.mark.parametrize(
    ("clause", "count"),
    [
        ("since:1d", 4),
        ("since:'2999-01-01'", 0),
        ("until:1d", 0),
        ("since:106751d", 4),
        ("since:15250w", 4),
        ("until:106751d", 0),
    ],
)
def test_anchor_seeded_recall_honours_time_bounds(
    clause, count, planets_graph, recall_count
):
    """since:/until: bound an anchor-seeded recall as they bound any other.

    The star was written moments ago. A window opening a day ago holds all of
    it. One opening in 2999 holds nothing, and so does one closing a day ago.
    The longest duration each unit can hold still opens in the past, about 292
    years ago, so it holds the whole star. One day more is rejected as out of
    range (see parser_test.py).
    """
    assert recall_count(f"recall@{planets_graph} topic:planets {clause}") == count


@pytest.mark.parametrize("clause", ["depth:0", "depth:1", "depth:2"])
def test_anchor_seeded_recall_ignores_depth(clause, planets_graph, recall_ranking):
    """depth: has no effect when the anchors seed: every lane returns the star.

    The members are already in hand, so nothing is expanded from them. No lane
    can pull in the lantern and almanac facts that share graph 7.
    """
    full = recall_ranking(f"recall@{planets_graph} topic:planets top:10")
    assert (
        recall_ranking(f"recall@{planets_graph} topic:planets {clause} top:10") == full
    )


def test_recall_of_an_unknown_anchor_is_empty(query):
    """An anchor nothing is filed under seeds nothing.

    The answer is a 200 with no hits, not an error. The question was
    well-formed, and the anchor is empty.
    """
    status, body = query("recall@7 topic:nosuchanchorprobe")
    assert status == 200, body.get("error")
    assert body["results"] == {"count": 0, "hits": []}


def test_a_recall_with_a_term_lifts_the_anchors_facts(
    planets_graph, planet_facts, recall_ranking
):
    """Beside a term, the anchor filters nothing and lifts the facts under it.

    `recall mercury topic:planets` returns the mercury fact first, on its own
    match, then the rest of the star: the named topic passes the seed's mass to
    every fact filed under it. Those facts hold that one observation and
    nothing else, so they share a score and recency alone orders them, newest
    first. The fixture writes the star in dict order, so they come in that
    order reversed.
    """
    others = [fact for key, fact in planet_facts.items() if key != "mercury"]
    want = [planet_facts["mercury"], *reversed(others)]
    hits = recall_ranking(f"recall@{planets_graph} mercury topic:planets")
    assert hits == want, f"want the match, then the star newest first; got {hits}"


@pytest.mark.parametrize("top", [1, 2, 3])
def test_a_named_anchors_lifted_facts_are_cut_by_top(
    top, planets_graph, recall_ranking
):
    """`top` keeps the head of the ranking a named anchor lifts.

    The facts the topic lifts share one score, so only their ages tell them
    apart. A top:N recall must keep the match and the newest of them, a prefix
    of the uncut ranking, not whichever lifted facts the search came to first.
    """
    full = recall_ranking(f"recall@{planets_graph} mercury topic:planets")
    truncated = recall_ranking(
        f"recall@{planets_graph} mercury topic:planets top:{top}"
    )
    assert truncated == full[:top], (
        f"top:{top} must be the first {top} of the full ranking {full}; got {truncated}"
    )


# A fact's three times. A fact is written at one moment, was said at another,
# and states something that happened at a third. A remember gives the last two
# with said: and occurred:. A fact reads the next time down for one it was not
# given: happened falls back to said, and said to written. A time bound keeps
# the facts that happened inside its window. A when answers with the time of
# the fact it finds, on the clock it names, and a bound can read that answer.
# The dated timeline (conftest) holds one fact for each way of holding a time.


@pytest.mark.parametrize(
    ("clause", "keys"),
    [
        ("since:'2023-05-10'", ["briefing", "drills", "mooring", "cruise"]),
        ("until:'2023-05-08'", ["regatta", "drills"]),
        ("since:'2023-05-08' until:'2023-05-11'", ["protest", "cruise"]),
        ("since:'2023-05-16' until:'2023-05-17'", ["cruise"]),
        ("since:'2023-05-17' until:'2023-05-20'", []),
        ("since:'2023-05-29'", ["briefing", "mooring"]),
        ("until:'2023-05-02'", ["drills"]),
        ("until:'2023-05-01'", []),
        ("since:'2023-05-12' until:'2023-05-12'", ["cruise"]),
        ("since:'2023-05-10' until:'2023-05-10'", []),
        ("since:'2023-05-08' until:'2023-05-08'", []),
    ],
)
def test_a_time_bound_keeps_the_facts_that_happened_in_its_window(
    clause, keys, timeline_graph, timeline_keyword, timeline_facts, recall_ranking
):
    """since: and until: read when a fact happened, not when it was written.

    Every fact here was written moments ago, so a window in May 2023 would hold
    none of them on the written time. A fact is in the window when one of its
    spans overlaps it, and the window runs from since: up to until:, which it
    leaves out. So the regatta of the 7th is before the 8th, and the protest
    said on the 8th is not. The cruise's last day is the 16th. The mooring is
    still held, so it is in every window that opens after it was taken. The
    drills happened twice and need one of their two days inside. The protest
    was given no occurred:, so it happened when it was said. The briefing was
    given no time at all, so it happened when it was written.

    until: leaves its moment out when since: names the same one too. Such a
    window holds the cruise on the 12th, when it was under way, and not on the
    10th, the day it began, nor the protest on the 8th, the moment it was said.
    Each of those until: alone leaves out, and a since: beside it only narrows.
    """
    hits = recall_ranking(f"recall@{timeline_graph} {timeline_keyword} {clause}")
    assert sorted(hits) == sorted(timeline_facts[key] for key in keys), (
        f"{clause}: want {keys}, got {hits}"
    )


@pytest.mark.parametrize(
    ("clause", "keys"),
    [
        (
            "since:'2023-05'",
            ["briefing", "protest", "regatta", "drills", "mooring", "cruise"],
        ),
        ("until:'2023-05'", []),
        ("since:'2023-06'", ["briefing", "mooring"]),
        ("until:'2023-06'", ["protest", "regatta", "drills", "mooring", "cruise"]),
        (
            "since:'2023' until:'2024'",
            ["protest", "regatta", "drills", "mooring", "cruise"],
        ),
        ("since:'2024'", ["briefing", "mooring"]),
        (
            "since:'2023-05-07'..'2023-05-09'",
            ["briefing", "protest", "regatta", "drills", "mooring", "cruise"],
        ),
        ("until:'2023-05-07'..'2023-05-09'", ["drills"]),
    ],
)
def test_a_dated_bound_sits_where_the_time_it_names_starts(
    clause, keys, timeline_graph, timeline_keyword, timeline_facts, recall_ranking
):
    """A bound written as a month, a year or a range is where that time starts.

    '2023-05' names the whole of May, and a bound sits at its first moment:
    since: opens the window there and until: closes it there. So until:'2023-05'
    holds nothing of May, and until:'2023-06' holds all of it. A range reads
    the same way, from its first date.
    """
    hits = recall_ranking(f"recall@{timeline_graph} {timeline_keyword} {clause}")
    assert sorted(hits) == sorted(timeline_facts[key] for key in keys), (
        f"{clause}: want {keys}, got {hits}"
    )


@pytest.mark.parametrize(
    ("clause", "keys"),
    [
        ("until:(when regatta)", ["drills"]),
        (
            "since:(when regatta)",
            ["briefing", "protest", "drills", "mooring", "cruise"],
        ),
        ("since:(when regatta) until:(when cruise)", ["protest"]),
        ("until:(when:said cruise)", ["protest", "regatta"]),
        ("since:(when:said cruise)", ["briefing", "drills", "mooring", "cruise"]),
        ("until:(when:written regatta)", ["briefing", "protest"]),
        ("since:(when:written regatta)", ["regatta", "drills", "mooring", "cruise"]),
        ("since:(when mooring)", ["briefing", "drills", "mooring"]),
        ("until:(when mooring)", ["protest", "regatta", "drills", "cruise"]),
        ("since:(when drills)", ["briefing", "mooring"]),
        ("until:(when drills)", []),
        ("since:(when regatta cruise top:2)", ["briefing", "drills", "mooring"]),
        ("until:(when regatta cruise top:2)", ["drills"]),
    ],
)
def test_a_bound_reads_the_time_a_when_answers_with(
    clause, keys, timeline_graph, timeline_keyword, timeline_facts, recall_ranking
):
    """A when in a bound stands for the time its fact holds, on the when's clock.

    until: closes the window where that time starts, and since: opens it where
    the time ends. "Before the regatta" stops short of it, and "after the
    regatta" starts once it is over. The window then compares on the clock the
    when read. On said, the facts told before the cruise was reported are the
    two said on the 8th, whatever day they happened. On written, the window
    follows the order the facts were stored in. The mooring has no end, so
    since: opens where it starts. The drills happened twice, and their time
    runs from the first day to the last. So does the time of a when that keeps
    two facts: it runs from the regatta to the end of the cruise.
    """
    hits = recall_ranking(f"recall@{timeline_graph} {timeline_keyword} {clause}")
    assert sorted(hits) == sorted(timeline_facts[key] for key in keys), (
        f"{clause}: want {keys}, got {hits}"
    )


def test_a_when_in_a_bound_reads_the_graph_of_its_recall(
    timeline_graph, harbour_graph, harbour_keyword, harbour_facts, recall_ranking
):
    """A when in a bound reads the graph its recall selected.

    The when takes no selector of its own. Both graphs hold a regatta: the
    harbour's in March 2024, the default graph's in May 2023. Read from the
    harbour's graph, each bound keeps the one fact on its side of the harbour's
    regatta. Read from the default graph, until: would hold nothing and since:
    all three.
    """
    recall = f"recall@{harbour_graph} {harbour_keyword}"
    assert recall_ranking(f"{recall} until:(when regatta)") == [harbour_facts["shoals"]]
    assert recall_ranking(f"{recall} since:(when regatta)") == [harbour_facts["buoys"]]


@pytest.mark.parametrize("bound", ["since", "until"])
def test_a_bound_whose_when_matched_nothing_empties_the_recall(
    bound, timeline_graph, timeline_keyword, query
):
    """A when that finds no fact leaves its bound unread, and the recall empty.

    Run without the bound, the recall would answer a wider question than the
    one asked, and nothing would say so. So it returns no hits, and the warning
    names the bound and the reason. The recall's own term matches six facts.
    """
    status, body = query(
        f"recall@{timeline_graph} {timeline_keyword} {bound}:(when zzznomatchzzz)"
    )
    assert status == 200, body.get("error")
    assert body == {
        "results": {"count": 0, "hits": []},
        "warnings": [
            f"{bound}: the when matched no fact, so it has no time to answer with; "
            "the recall has no bound to read and returns no hits"
        ],
    }


@pytest.mark.parametrize(
    ("text", "key", "start", "end"),
    [
        ("when regatta", "regatta", "2023-05-07T00:00:00Z", "2023-05-08T00:00:00Z"),
        (
            "when:happened regatta",
            "regatta",
            "2023-05-07T00:00:00Z",
            "2023-05-08T00:00:00Z",
        ),
        (
            "when:said regatta",
            "regatta",
            "2023-05-08T00:00:00Z",
            "2023-05-08T00:00:00Z",
        ),
        ("when cruise", "cruise", "2023-05-10T00:00:00Z", "2023-05-17T00:00:00Z"),
        ("when:said cruise", "cruise", "2023-05-25T00:00:00Z", "2023-05-25T00:00:00Z"),
        ("when drills", "drills", "2023-05-01T00:00:00Z", "2023-05-29T00:00:00Z"),
        ("when protest", "protest", "2023-05-08T00:00:00Z", "2023-05-08T00:00:00Z"),
        ("when mooring", "mooring", "2023-05-20T00:00:00Z", None),
    ],
)
def test_a_when_answers_with_the_time_of_the_fact_it_finds(
    text, key, start, end, timeline_graph, timeline_facts, query
):
    """A when returns one fact and the time it holds on the clock asked for.

    With no clock, a when reads when the fact happened. A day runs from its
    midnight to the next, so the regatta of the 7th ends on the 8th, and the
    cruise, whose last day is the 16th, ends on the 17th. A moment starts and
    ends at once, as the day a fact was said does. The drills happened twice,
    and their time runs from the first day to the end of the last. The protest
    was given no occurred:, so it happened when it was said. The mooring is
    still held: its time has a start and no end.

    A when on its own reads the default graph, which is the timeline's.
    """
    status, body = query(text)
    assert status == 200, body.get("error")
    results = body["results"]
    assert [hit["value"] for hit in results["hits"]] == [timeline_facts[key]]
    assert (results.get("start"), results.get("end")) == (start, end), results


@pytest.mark.parametrize(
    "text", ["when:written regatta", "when briefing", "when:said briefing"]
)
def test_a_when_answers_with_the_written_time_when_it_is_the_one_to_read(
    text, timeline_graph, query
):
    """The moment a fact was written answers for a clock it has no time on.

    when:written asks for that moment by name, whatever times the fact was
    given. The briefing was given none, so it was said, and it happened, at the
    moment it was stored. Each answer is that one moment, the hit's own
    timestamp, as start and as end.
    """
    status, body = query(text)
    assert status == 200, body.get("error")
    results = body["results"]
    [hit] = results["hits"]
    assert results["start"] == results["end"] == hit["timestamp"], results


def test_a_when_keeps_one_fact_unless_top_asks_for_more(
    timeline_graph, timeline_keyword, timeline_facts, recall_count, query
):
    """A when keeps its best fact, and top: widens its time over several.

    A recall of the timeline's keyword returns all six facts, and a when of it
    returns one, with that fact's time. With top:2 over the regatta and the
    cruise, the when keeps both, and its time runs from the start of the first
    to the end of the last. Over the whole timeline, the time opens with the
    first drills and has no end, because the mooring has none.
    """
    assert recall_count(f"recall@{timeline_graph} {timeline_keyword}") == len(
        timeline_facts
    )
    assert recall_count(f"when {timeline_keyword}") == 1

    status, body = query("when regatta cruise top:2")
    assert status == 200, body.get("error")
    results = body["results"]
    assert sorted(hit["value"] for hit in results["hits"]) == sorted(
        [timeline_facts["regatta"], timeline_facts["cruise"]]
    )
    assert (results["start"], results["end"]) == (
        "2023-05-07T00:00:00Z",
        "2023-05-17T00:00:00Z",
    )

    status, body = query(f"when {timeline_keyword} top:{len(timeline_facts)}")
    assert status == 200, body.get("error")
    results = body["results"]
    assert results["count"] == len(timeline_facts)
    assert results["start"] == "2023-05-01T00:00:00Z"
    assert "end" not in results, results


def test_a_when_that_matches_nothing_has_no_time_and_says_so(timeline_graph, query):
    """A when that finds no fact answers with no hits, no time and a warning.

    The graph is populated, so the answer is a 200. It is the empty result of a
    recall that missed, with no start and no end, and the warning says the when
    has no time to give.
    """
    status, body = query("when zzznomatchzzz")
    assert status == 200, body.get("error")
    assert body == {
        "results": {"count": 0, "hits": []},
        "warnings": ["the when matched no fact, so it has no time to answer with"],
    }


def test_remembering_a_fact_again_replaces_its_times(query):
    """A fact stated again takes the times of the last statement.

    A fact is keyed by its value, so each remember here is the same fact. Said
    on the 8th about the 7th, then said on the 9th with no occurred:, it
    carries the 9th alone. Stated a third time with no time at all, it carries
    none. The fact sits among graph 1's loose remembers, and the last write
    leaves it as a rerun finds it.
    """
    fact = "the reassertprobe ferry was repainted"
    statements = [
        (
            "said:'2023-05-08' occurred:'2023-05-07'",
            {
                "said": "2023-05-08T00:00:00Z",
                "occurred": [
                    {"start": "2023-05-07T00:00:00Z", "end": "2023-05-08T00:00:00Z"}
                ],
            },
        ),
        ("said:'2023-05-09'", {"said": "2023-05-09T00:00:00Z"}),
        ("", {}),
    ]
    for clauses, want in statements:
        status, body = query(f"remember@1 '{fact}' {clauses}".rstrip())
        assert status == 200, body.get("error")

        status, body = query("recall@1 reassertprobe")
        assert status == 200, body.get("error")
        [hit] = body["results"]["hits"]
        got = {key: hit[key] for key in ("said", "occurred") if key in hit}
        assert got == want, f"after {clauses!r}: want {want}, got {got}"
