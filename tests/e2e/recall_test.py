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

"""Remember/recall semantics: the store-then-find round trip, how anchors
transmit (and stay silent), how top shapes a recall's results, text-index
matching across facts, and recall through topic:/entity: anchors — including
anchors whose value is a grammar keyword, and the case-folding of terms and
anchor values.
"""

import pytest


def test_recall_on_empty_graph(query):
    status, body = query("recall nothingindexedyet")
    assert status == 200
    results = body["results"]
    assert results is not None
    assert results["count"] == 0
    assert results["hits"] == []


def test_recall_with_clauses(query):
    status, body = query("recall@2 anna bob entity:alice topic:job top:10 depth:2")
    assert status == 200
    assert body["results"] is not None


def test_remember_is_accepted(query):
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

    Inside a quoted phrase every character is literal: reserved words and
    symbols carry no meaning, a doubled quote is the escape for an apostrophe,
    and newlines, control characters (a NUL included — JSON delivers it as
    \\u0000), emoji and backslashes are all data. The same battery the
    integration suite parses is asserted harder here: each value is recalled
    by the marker word stored beside it and must come back exactly as written.

    Facts are keyed by their value, so the writes stay idempotent across
    reruns against a long-lived server.
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
    """The wire shape for a natural-language question: the whole question is
    one quoted phrase term — reserved words like "topic" stay literal inside
    the quotes — with clauses following. Bare unquoted question words would
    collide with the grammar's keywords and are rejected; the quoted form is
    the supported shape.
    """
    status, body = query(
        "recall@1 'what topic has jules been blogging about recently' top:10"
    )
    assert status == 200, body.get("error")
    assert body["results"] is not None


def _recall_count(query, text):
    status, body = query(text)
    assert status == 200, body.get("error")
    return body["results"]["count"]


def test_recall_depth_selects_a_lane(planets_graph, query):
    """depth picks the retrieval lane, and both lanes answer this query the
    same way — for different reasons.

    depth:0 is the floor: the anchor traversal never runs, so only the seed
    itself can be returned. depth:1 and depth:2 both run the anchor-mediated
    round — the star's topic is named, which is what opens the graph — and
    still return the seed alone, because a lone seed's topic hub sits exactly
    at the background rate and so transmits nothing at either admission bar.
    Omitting the clause is the configured default. Asserting all of them is
    what separates "the hub was silent" from "the graph channel was never
    consulted" — a regression that broke transmission entirely would still pass
    a floor-only test. The lantern cluster below is what distinguishes the
    floor from the graph lanes by their results.
    """
    g = planets_graph
    for clause in ("depth:0", "depth:1", "depth:2", ""):
        text = f"recall@{g} mercury topic:planets {clause}".strip()
        assert _recall_count(query, text) == 1, (
            f"{text}: a fair-share hub must not transmit"
        )


def test_floor_lane_returns_only_what_the_text_index_matched(
    lantern_graph, lantern_silent, query
):
    """depth:0 is the floor lane: seeds only, no transmission.

    The traversal never runs, so every hit contains the query term and the
    cluster's silent member — which contains none — cannot appear. depth:0 is
    also the explicit spelling of the shipped default, and must be honoured
    rather than collapsing back to the configured default, which is what made
    it a bug.
    """
    clause = "depth:0"
    status, body = query(
        f"recall@{lantern_graph} lantern topic:lanterns topic:almanac {clause} top:20"
    )
    assert status == 200, body.get("error")

    values = [hit["value"] for hit in body["results"]["hits"]]
    assert values, f"{clause}: the text index still matches the lantern facts"
    assert lantern_silent not in values, (
        f"{clause} surfaced {lantern_silent!r}: it carries no query term, so "
        "the floor lane must not be able to reach it"
    )


@pytest.mark.parametrize("clause", ["depth:1", "depth:2"])
def test_graph_lanes_transmit_to_a_fact_the_floor_cannot_reach(
    lantern_graph, lantern_silent, query, clause
):
    """depth:1 and depth:2 both run the round, and the difference from the
    floor is visible in the hits.

    The silent member arrives funded by its cluster's surplus alone, while the
    almanac hub — holding a fair share of the query's mass across eight
    members — transmits nothing, so its memos stay out. Same query, same graph,
    one clause apart from the floor test above: this is the pair that proves
    the lane switch reaches the engine rather than being parsed and dropped.
    Both topics are named because the graph is entered only through an anchor
    the recall names; naming the hub too keeps its memos in the candidate set,
    so their absence is the hub's silence and not the filter's doing.

    This cluster is strongly above chance, so it clears both admission bars.
    What separates depth:1 (precision) from depth:2 (max recall) is an anchor
    between one and two times its fair share, which needs mass arithmetic too
    delicate to pin over HTTP — TestSearchDepthOnePrecisionBar covers it.
    """
    status, body = query(
        f"recall@{lantern_graph} lantern topic:lanterns topic:almanac {clause} top:20"
    )
    assert status == 200, body.get("error")

    values = [hit["value"] for hit in body["results"]["hits"]]
    assert lantern_silent in values, (
        f"{clause} did not fund {lantern_silent!r}; got {values}"
    )
    assert not any(v.startswith("unrelated almanac entry") for v in values), (
        f"a fair-share hub transmitted to its members: {values}"
    )


def test_recall_top_truncates_results(planets_graph, planet_facts, query):
    """Top caps the number of ranked results returned, never pads. All four
    facts contain "planet", so the text index matches every one directly.
    """
    g = planets_graph
    n = len(planet_facts)

    assert _recall_count(query, f"recall@{g} planet top:1") == 1
    assert _recall_count(query, f"recall@{g} planet top:2") == 2
    assert _recall_count(query, f"recall@{g} planet top:3") == 3
    # top larger than the number available returns everything, not padding.
    assert _recall_count(query, f"recall@{g} planet top:10") == n


def test_recall_unique_keyword_returns_only_its_fact(
    planets_graph, planet_facts, query
):
    """The recall for one fact's unique keyword is exactly that fact, by
    value — its silent hub adds nothing and its siblings stay out.
    """
    g = planets_graph
    status, body = query(f"recall@{g} mercury")
    assert status == 200, body.get("error")
    values = [hit["value"] for hit in body["results"]["hits"]]
    assert values == [planet_facts["mercury"]]


# Three facts that all contain the keyword "comet" but are otherwise unrelated:
# each carries a *different* topic, so nothing connects them in the graph except
# the shared word. A recall for that word must therefore surface all three
# purely through the text index.
#
# Every other recall test uses a keyword unique to a single fact, so the text
# search there always matches exactly one document — a regression that capped
# text search at one hit would pass the whole rest of the suite unnoticed. This
# test is the one that would catch it, on its own graph (0) so the count is
# fully determined here.
COMET_FACTS = {
    "the comet streaked past mars": "astronomy",
    "children watched the comet at dawn": "memory",
    "the comet will not return for centuries": "time",
}


def test_recall_returns_every_document_sharing_a_keyword(query):
    graph = 0
    for phrase, topic in COMET_FACTS.items():
        status, body = query(f"remember@{graph} '{phrase}' topic:{topic}")
        assert status == 200, body.get("error")

    status, body = query(f"recall@{graph} comet top:10")
    assert status == 200, body.get("error")

    values = {hit["value"] for hit in body["results"]["hits"]}
    assert set(COMET_FACTS) <= values, (
        "recall by a shared keyword must return every matching fact, not just "
        f"one; want all of {set(COMET_FACTS)}, got {values}"
    )


def test_recall_unions_matches_across_keywords(query):
    """A recall naming several keywords returns the union of their matches, so a
    single query yields multiple results.

    The two facts share no word and carry different topics, so nothing links
    them in the graph. "saturn" matches only the first, "neptune" only the
    second; recalling both keywords must return both facts. Graph 2 is only ever
    recalled elsewhere in the suite, never written, so these are its only facts.
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


def test_recall_with_anchor_filters_returns_tagged_fact(query):
    """A fact written with topic:/entity: anchors must be recallable through
    those anchors — the ticket repro. Regression: Commit created the anchor
    edges but never stored the Topic/NamedEntity nodes, so every anchored
    recall filtered everything out and returned count 0.
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


# A fact whose anchors are grammar keywords. "top" is also an ordinary English
# word, and an LLM extracting entities from prose will eventually emit it bare
# ("she reached the top" -> entities=["top"]) — a certainty at corpus size.
# It used to kill the whole ingestion run with a 400 the client could not
# anticipate, because the parser typed the anchor value by spelling alone. The
# invented marker "cairnprobe" is this fact's only link to the recalls below,
# so each assertion is scoped to exactly this fact whatever else graph 5 holds.
CAIRN_FACT = "the cairnprobe marks the top of the pass"


def test_keyword_anchor_values_round_trip(query):
    """A fact filed under entity:top and topic:top is reachable through both
    anchors — the bug-report repro, taken all the way through the store.

    The remember alone proves the parse; the anchored recalls prove the
    anchors actually landed as the word "top" rather than being dropped or
    misread as a result-limit clause. Each filter must narrow the marker
    recall to exactly this fact.
    """
    status, body = query(f"remember@5 '{CAIRN_FACT}' topic:top entity:top")
    assert status == 200, body.get("error")

    for q in (
        "recall@5 cairnprobe entity:top",
        "recall@5 cairnprobe topic:top",
    ):
        status, body = query(q)
        assert status == 200, body.get("error")
        hits = body["results"]["hits"]
        assert len(hits) == 1, f"{q!r} -> {body['results']}"
        assert hits[0]["value"] == CAIRN_FACT


def test_keyword_recalls_as_a_leading_term(query):
    """In `recall top top:10`, the first "top" is a search term and the second
    is the result-limit clause.

    The leading term is the one position where a bare keyword reads as a
    word: a recall must start with a term, so no clause can begin there. What
    tells the two "top"s apart is the ':' — a keyword immediately followed by
    ':' is always a field. The term must then reach the cairnprobe fact
    through the text index like any other word, since its text contains "top".
    """
    status, body = query(f"remember@5 '{CAIRN_FACT}' topic:top entity:top")
    assert status == 200, body.get("error")

    status, body = query("recall@5 top top:10")
    assert status == 200, body.get("error")
    values = [hit["value"] for hit in body["results"]["hits"]]
    assert CAIRN_FACT in values, (
        f"the leading term 'top' should match the cairnprobe fact; got {values}"
    )


# Case folding. Terms and anchor values are identity, not prose: the parser
# folds them to lower case on the way in, so however a client capitalises an
# anchor, a single node accrues in the graph. The quoted fact is prose and is
# the one exception — it comes back spelled exactly as written.
CASEPROBE_FACT = "The Caseprobe Expedition Reached the Summit in April."


def test_anchor_case_folds_while_the_fact_keeps_its_spelling(query):
    """topic:Mountaineering, topic:MOUNTAINEERING and topic:mountaineering are
    one anchor, and the stored fact keeps its capitalisation.

    The write uses mixed-case anchors; the recalls each use a different
    casing of the term, the topic and the entity, and every one must land on
    the same single fact. If the fold regressed, one anchor would exist under
    as many nodes as clients have capitalisations, and an anchored recall
    would silently miss facts filed under another spelling — no error, just
    absent memories.
    """
    status, body = query(
        f"remember@5 '{CASEPROBE_FACT}' topic:Mountaineering entity:Karakoram"
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
        assert hits[0]["value"] == CASEPROBE_FACT, (
            f"{q!r}: the fact must come back spelled exactly as written; "
            f"got {hits[0]['value']!r}"
        )


# Five facts that all contain "quasar" exactly once and carry no topic:/entity:
# anchor at all, so each is an isolated node: nothing links them to each other,
# and no walk from another graph-0 fact can reach them. The text index scores all
# five identically — one keyword match each — which leaves their order decided
# entirely by the tiebreak on fact key. They share graph 0 with the comet facts
# above, which contain no "quasar".
QUASAR_FACTS = (
    "the quasar catalogue was revised",
    "a quasar outshines its host galaxy",
    "radio astronomers logged the quasar",
    "the quasar sits behind a lensing cluster",
    "the quasar faded from the survey",
)


def _recall_ranking(query, text):
    """The hit values of a recall, best-ranked first.

    Values, not scores: a score decays with the fact's age at the instant the
    search runs, so two identical recalls a millisecond apart legitimately score
    the same fact differently. It is the ranking those scores produce that has to
    be reproducible.
    """
    status, body = query(text)
    assert status == 200, body.get("error")
    return [hit["value"] for hit in body["results"]["hits"]]


def test_identical_recalls_return_identically_ranked_hits(query):
    """The same recall, issued repeatedly, must rank the same facts the same way.

    The facts tie on relevance, so the ranking is settled by the tiebreak on fact
    key. Candidates are pooled out of maps, whose iteration order changes per
    call, so before the tiebreak this query returned the same facts in a
    different order each time. The expected order is deliberately not written
    down: it follows from the configured hash function, and what callers are
    promised is that it does not move.
    """
    graph = 0
    for phrase in QUASAR_FACTS:
        status, body = query(f"remember@{graph} '{phrase}'")
        assert status == 200, body.get("error")

    # The facts carry no anchor and the query names none, so this is exactly
    # the five the text index matches.
    text = f"recall@{graph} quasar"
    ranking = _recall_ranking(query, text)
    assert set(ranking) == set(QUASAR_FACTS), (
        f"{text!r} must match every quasar fact and nothing else; got {ranking}"
    )

    for call in range(2, 11):
        assert _recall_ranking(query, text) == ranking, (
            f"call {call} of {text!r} ranked the same facts differently; "
            "recall is not reproducible"
        )


@pytest.mark.parametrize("top", [1, 2, 3])
def test_recall_top_keeps_the_head_of_the_ranking(top, query):
    """`top` truncates the ranking; it must keep its head, not an arbitrary slice.

    Truncation applied before the order is total drops whichever tied facts the
    candidate map happened to yield last, so a top:1 answer need not name the
    fact the untruncated recall ranked first — that was the user-visible symptom.
    The untruncated recall is the reference the truncated one has to prefix.
    """
    graph = 0
    for phrase in QUASAR_FACTS:
        status, body = query(f"remember@{graph} '{phrase}'")
        assert status == 200, body.get("error")

    full = _recall_ranking(query, f"recall@{graph} quasar")
    truncated = _recall_ranking(query, f"recall@{graph} quasar top:{top}")
    assert truncated == full[:top], (
        f"top:{top} must be the first {top} of the full ranking {full}; got {truncated}"
    )


# A fact whose whole text is also the name of its topic, plus a bystander fact
# carrying the same topic. Keys derived from value alone gave the fact and the
# topic one key, so the topic node was never stored and the bystander's IsAbout
# edge landed on the fact instead of on a topic hub. The bystander is what makes
# that visible: the two facts have no word in common and belong together only
# through the topic.
LEDGER_TOPIC = "ledgerprobe"
LEDGER_FACTS = ("ledgerprobe", "acme settles invoices quarterly")


def test_recall_depth_one_is_not_polluted_by_a_fact_named_like_a_topic(query):
    """A fact whose text equals its topic's name must not stand in for the topic.

    Recall returns facts, never the hubs it walks through, so with the topic
    named to open the graph, depth:1 is exactly the seed here. When the
    "ledgerprobe" fact *was* the hub, the bystander sat one hop from a fact
    rather than one hop from a topic, and this recall handed back an unrelated
    memory as a second hit. Graph 5's other facts carry different topics, so
    nothing else is one hop from the seed.
    """
    graph = 5
    for phrase in LEDGER_FACTS:
        status, body = query(f"remember@{graph} '{phrase}' topic:{LEDGER_TOPIC}")
        assert status == 200, body.get("error")

    hits = _recall_ranking(
        query, f"recall@{graph} quarterly topic:{LEDGER_TOPIC} depth:1"
    )
    assert hits == ["acme settles invoices quarterly"], (
        f"depth:1 must return the seed alone; {LEDGER_FACTS[0]!r} reached it as a "
        f"neighbour, so it is serving as the {LEDGER_TOPIC!r} topic node: {hits}"
    )


# ---------------------------------------------------------------------------
# Anchor-seeded recall. A recall naming anchors and no term or vector is
# seeded by the anchors themselves: every fact filed under them enters the
# candidates instead of being filtered by them, and the ordinary ranking — a
# unit of mass per named anchor a fact is filed under, decayed by age — puts
# the newest first. The planet star is the single-anchor case; the tidepool
# probe (conftest) is the union.
# ---------------------------------------------------------------------------


def test_anchor_only_recall_returns_every_member_newest_first(
    planets_graph, planet_facts, query
):
    """`recall topic:planets` is seeded by the topic: the whole star, newest first.

    The fixture writes the facts in dict order, so the ranking is that order
    reversed. Before anchors seeded, this query parsed and returned nothing
    (no term, no seed), which a caller could not tell from an empty topic.
    """
    ranking = _recall_ranking(query, f"recall@{planets_graph} topic:planets top:10")
    assert ranking == list(reversed(planet_facts.values())), (
        f"want the star newest first, got {ranking}"
    )


def test_anchor_seeded_hits_are_scored_by_the_ordinary_ranking(planets_graph, query):
    """An anchor-seeded recall is ranked like any other: each hit carries the
    unit of mass its anchor lends, decayed by age, so scores are positive and
    strictly descend with the ranking rather than being absent or flat.

    Strictly, because a flat set would drain from the ranker in non-increasing
    order too. The star's writes are sequential round trips, so the decay
    factors differ by about a part in ten billion — resolvable because
    tests/fraise.config.toml runs the daemon at float64 precision.
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
    tidepool_graph, tidepool_anchors, tidepool_facts, query
):
    """`recall topic:tidepool entity:limpet` is everything under either anchor,
    each fact once.

    Alone, the anchors seed rather than filter — the same two clauses beside a
    term narrow to facts filed under both — so the result is the union. The
    fact filed under both anchors carries two units of seed mass and, written
    moments apart from the others, ranks first; the other two follow newest
    first, the entity's fact having been written after the topic's.
    """
    topic, entity = tidepool_anchors
    ranking = _recall_ranking(
        query, f"recall@{tidepool_graph} topic:{topic} entity:{entity} top:10"
    )
    assert ranking == [
        tidepool_facts["both"],
        tidepool_facts["entity"],
        tidepool_facts["topic"],
    ], f"want the union with the doubly-filed fact first, got {ranking}"


@pytest.mark.parametrize("top", [1, 2, 3])
def test_anchor_seeded_top_keeps_the_head_of_the_ranking(top, planets_graph, query):
    """`top` truncates an anchor-seeded recall as it truncates any other: the
    head, never an arbitrary slice.
    """
    full = _recall_ranking(query, f"recall@{planets_graph} topic:planets top:10")
    truncated = _recall_ranking(
        query, f"recall@{planets_graph} topic:planets top:{top}"
    )
    assert truncated == full[:top], (
        f"top:{top} must be the first {top} of {full}; got {truncated}"
    )


def test_anchor_seeded_recall_without_top_takes_the_configured_default(
    saltmarsh_graph, saltmarsh_facts, default_top, query
):
    """An anchor-seeded recall with no top: clause is capped at default-top.

    The saltmarsh anchor holds more facts than the configured default, so the
    uncapped recall must come back exactly default-top long — the head of the
    ranking — while a top: past the count returns every member.
    """
    assert len(saltmarsh_facts) > default_top
    full = _recall_ranking(
        query, f"recall@{saltmarsh_graph} topic:saltmarsh top:{len(saltmarsh_facts)}"
    )
    assert sorted(full) == sorted(saltmarsh_facts)
    capped = _recall_ranking(query, f"recall@{saltmarsh_graph} topic:saltmarsh")
    assert capped == full[:default_top], (
        f"want the first {default_top} of {full}; got {capped}"
    )


@pytest.mark.parametrize(
    ("clause", "count"),
    [("since:1d", 4), ("since:2999-01-01", 0), ("until:1d", 0)],
)
def test_anchor_seeded_recall_honours_time_bounds(clause, count, planets_graph, query):
    """since:/until: bound an anchor-seeded recall as they bound any other.

    The star was written moments ago: a window opening a day ago holds all of
    it, one opening in 2999 holds nothing, and one closing a day ago holds
    nothing either.
    """
    assert (
        _recall_count(query, f"recall@{planets_graph} topic:planets {clause}") == count
    )


@pytest.mark.parametrize("clause", ["depth:0", "depth:1", "depth:2"])
def test_anchor_seeded_recall_ignores_depth(clause, planets_graph, query):
    """depth: has no effect when the anchors seed: every lane returns the star.

    The members are already in hand, so nothing is expanded from them: the
    lantern and almanac facts share graph 7 and sit two hops from nothing
    here, and no lane can pull them in.
    """
    full = _recall_ranking(query, f"recall@{planets_graph} topic:planets top:10")
    assert (
        _recall_ranking(query, f"recall@{planets_graph} topic:planets {clause} top:10")
        == full
    )


def test_recall_of_an_unknown_anchor_is_empty(query):
    """An anchor nothing is filed under seeds nothing — the one genuinely
    empty anchored recall, and a 200, not an error: the question was
    well-formed and the answer is that the anchor is empty.
    """
    status, body = query("recall@7 topic:nosuchanchorprobe")
    assert status == 200, body.get("error")
    assert body["results"] == {"count": 0, "hits": []}


def test_a_recall_with_a_term_filters_by_the_anchor(planets_graph, planet_facts, query):
    """A term beside the anchor seeds from the text index with the anchor as a
    filter: `recall mercury topic:planets` is the mercury fact alone, not the
    star it is filed in.
    """
    hits = _recall_ranking(query, f"recall@{planets_graph} mercury topic:planets")
    assert hits == [planet_facts["mercury"]]
