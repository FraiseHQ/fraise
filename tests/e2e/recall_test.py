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


# Four facts sharing a single topic, each with a unique keyword. This is a
# star: every fact hangs off the same "planets" hub. Recall returns facts, not
# the hub, so from a seed fact the hub is one hop away (depth 1, invisible) and
# the sibling facts are two hops away (depth 2). That makes the exact result
# counts a clean function of depth and top.
PLANET_TOPIC = "planets"
PLANET_FACTS = {
    "mercury": "mercury is the smallest planet",
    "venus": "venus is the hottest planet",
    "mars": "mars is the red planet",
    "jupiter": "jupiter is the largest planet",
}


@pytest.fixture(scope="module")
def planets_graph(query):
    """Populate a dedicated graph with the planet star and return its id.

    A fact is keyed by its value, so these writes are idempotent: re-running the
    suite against a long-lived server leaves the counts unchanged. Graph 7 is
    used by no other test, so its contents are fully known here.
    """
    graph = 7
    for phrase in PLANET_FACTS.values():
        status, body = query(f"remember@{graph} '{phrase}' topic:{PLANET_TOPIC}")
        assert status == 200, body.get("error")
    return graph


def _recall_count(query, text):
    status, body = query(text)
    assert status == 200, body.get("error")
    return body["results"]["count"]


def test_recall_depth_selects_a_lane(planets_graph, query):
    """depth picks the retrieval lane, and both lanes answer this query the
    same way — for different reasons.

    depth:1 is the BM25 floor: the anchor traversal never runs, so only the
    seed itself can be returned. depth:2 runs the excess round, and returns
    the seed alone anyway because a lone seed's topic hub sits exactly at the
    background rate and transmits nothing. Omitting the clause is depth:1 (the
    default lane). Asserting both is what separates "the hub was silent" from
    "the graph channel was never consulted" — a regression that broke
    transmission entirely would still pass a depth:1-only test.
    """
    g = planets_graph
    for clause in ("depth:1", "depth:2", ""):
        assert _recall_count(query, f"recall@{g} mercury {clause}".strip()) == 1, (
            f"recall mercury {clause}: a fair-share hub must not transmit"
        )


def test_recall_rejects_depth_past_the_ceiling(query):
    """depth:3 is refused rather than silently treated as depth:2.

    The scorer does not iterate past one anchor-mediated round, so a depth
    above 2 has no meaning. Answering it as though it did would tell an agent
    its request was honoured when it was quietly downgraded; the error names
    the ceiling so the agent can correct.
    """
    status, body = query("recall@7 mercury depth:3")
    assert status == 400, f"expected a ceiling error, got {status}: {body}"
    assert "exceeds max 2" in body.get("error", ""), body.get("error")


def test_recall_top_truncates_results(planets_graph, query):
    """Top caps the number of ranked results returned, never pads. All four
    facts contain "planet", so the text index matches every one directly.
    """
    g = planets_graph
    n = len(PLANET_FACTS)

    assert _recall_count(query, f"recall@{g} planet top:1") == 1
    assert _recall_count(query, f"recall@{g} planet top:2") == 2
    assert _recall_count(query, f"recall@{g} planet top:3") == 3
    # top larger than the number available returns everything, not padding.
    assert _recall_count(query, f"recall@{g} planet top:10") == n


def test_recall_unique_keyword_returns_only_its_fact(planets_graph, query):
    """The recall for one fact's unique keyword is exactly that fact, by
    value — its silent hub adds nothing and its siblings stay out.
    """
    g = planets_graph
    status, body = query(f"recall@{g} mercury")
    assert status == 200, body.get("error")
    values = [hit["value"] for hit in body["results"]["hits"]]
    assert values == [PLANET_FACTS["mercury"]]


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

    # depth:1 holds the walk to the seeded facts, so this is exactly the five.
    text = f"recall@{graph} quasar depth:1"
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

    full = _recall_ranking(query, f"recall@{graph} quasar depth:1")
    truncated = _recall_ranking(query, f"recall@{graph} quasar depth:1 top:{top}")
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

    Recall returns facts, never the hubs it walks through, so depth:1 is exactly
    the seed here. When the "ledgerprobe" fact *was* the hub, the bystander sat
    one hop from a fact rather than one hop from a topic, and this recall handed
    back an unrelated memory as a second hit. Graph 5's other facts carry
    different topics, so nothing else is one hop from the seed.
    """
    graph = 5
    for phrase in LEDGER_FACTS:
        status, body = query(f"remember@{graph} '{phrase}' topic:{LEDGER_TOPIC}")
        assert status == 200, body.get("error")

    hits = _recall_ranking(query, f"recall@{graph} quarterly depth:1")
    assert hits == ["acme settles invoices quarterly"], (
        f"depth:1 must return the seed alone; {LEDGER_FACTS[0]!r} reached it as a "
        f"neighbour, so it is serving as the {LEDGER_TOPIC!r} topic node: {hits}"
    )
