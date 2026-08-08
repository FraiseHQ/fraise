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

"""Remember/recall semantics: the store-then-find round trip, how depth and
top shape a recall's results, text-index matching across facts, and recall
through topic:/entity: anchors.
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
    status, body = query("recall@2 anna bob entity:alice topic:job top:10 depth:5")
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


def test_recall_depth_controls_reach(planets_graph, query):
    """Depth bounds how far the walk leaves the seed, and thus the count.

    Note: depth:0 is not exercised — the query parser treats a 0 as "unset" and
    substitutes the configured default, so it cannot be expressed.
    """
    g = planets_graph
    n = len(PLANET_FACTS)

    # depth 1: only the seed fact. The shared topic hub is one hop away, but the
    # hub is graph structure, not a result, so nothing else surfaces.
    assert _recall_count(query, f"recall@{g} mercury depth:1") == 1
    # depth 2: the walk crosses the hub and reaches every sibling fact.
    assert _recall_count(query, f"recall@{g} mercury depth:2") == n
    # A single-topic star has nothing beyond two hops, so deeper adds nothing.
    assert _recall_count(query, f"recall@{g} mercury depth:3") == n
    # With no depth clause the configured default (2) applies, so a bare recall
    # already reaches the whole star. This guards the default-depth wiring.
    assert _recall_count(query, f"recall@{g} mercury") == n


def test_recall_top_truncates_results(planets_graph, query):
    """Top caps the number of ranked results returned, never pads."""
    g = planets_graph
    n = len(PLANET_FACTS)

    # At depth 2 all facts are reachable; top decides how many come back.
    assert _recall_count(query, f"recall@{g} mercury depth:2 top:1") == 1
    assert _recall_count(query, f"recall@{g} mercury depth:2 top:2") == 2
    assert _recall_count(query, f"recall@{g} mercury depth:2 top:3") == 3
    # top larger than the number available returns everything, not padding.
    assert _recall_count(query, f"recall@{g} mercury depth:2 top:10") == n


def test_recall_depth_one_returns_only_the_seed(planets_graph, query):
    """The depth:1 result is exactly the seed fact, by value."""
    g = planets_graph
    status, body = query(f"recall@{g} mercury depth:1")
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
