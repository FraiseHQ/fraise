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

"""Parsing of the server's JSON envelope into the typed models.

The unit tests parse recorded and hand-written bodies with no I/O; the tests
marked ``integration`` parse what a live server sends.
"""

from datetime import datetime

import pytest
from fraise_sdk.models import Contribution, GraphStats, Hit, RecallResult


def test_graph_stats_names_the_counts_for_what_they_count(stats_response):
    """The wire's graph-theory names land on the fields named for their meaning.

    ``order`` counts vertices and ``size`` edges; reading either into the
    other, or into ``nodes``, would misreport a graph's shape without failing.
    """
    row = GraphStats.from_json(stats_response["graphs"][0])

    assert row == GraphStats(
        id=0,
        vertices=4,
        edges=3,
        nodes=7,
        vectors=0,
        forest_entries=0,
        relevance="bm25",
    )


def test_graph_stats_without_relevance_is_none(stats_response):
    """A row from a server that predates ``relevance`` still parses.

    The SDK supports servers that never send the field; reading it as required
    would turn every ``stats()`` against them into a ``KeyError``.
    """
    row = {k: v for k, v in stats_response["graphs"][0].items() if k != "relevance"}

    assert GraphStats.from_json(row).relevance is None


def test_from_json_defaults_to_no_warnings():
    """Omitting warnings yields an empty list, never None.

    This is the shape a clean response — or a pre-warnings server — produces,
    and a caller iterating ``result.warnings`` must not have to guard it.
    It is also the backward-compatible path for direct ``from_json`` callers
    that still pass only the results dict.
    """
    result = RecallResult.from_json({"count": 0, "hits": []})

    assert result.warnings == []


def test_from_json_carries_warnings_beside_the_hits():
    """Warnings pass through as a plain list of strings, and the hits they
    arrived beside parse exactly as they would without them.
    """
    result = RecallResult.from_json(
        {"count": 1, "hits": [{"value": "since the storm", "score": 1.0}]},
        warnings=["parse warning at column 15: ..."],
    )

    assert result.warnings == ["parse warning at column 15: ..."]
    assert result.count == 1
    assert [hit.value for hit in result] == ["since the storm"]


def test_from_json_defaults_to_a_populated_graph():
    """``empty`` is False unless the caller says otherwise.

    It rides the status line (204), not the body, so ``from_json`` cannot
    infer it from the payload and must not try: an empty result set is the
    ordinary miss until the response says the graph itself was empty.
    """
    result = RecallResult.from_json({"count": 0, "hits": []})

    assert result.empty is False


def test_from_json_carries_the_empty_graph_flag():
    """A 204 has no body, so the empty result is paired with the flag by hand.

    The two halves of an empty answer come from different places — the body
    (or its absence) and the status — and this is where they are joined.
    """
    result = RecallResult.from_json({}, empty=True)

    assert result.empty is True
    assert result.count == 0
    assert result.hits == []
    assert bool(result) is False


def test_an_explained_hit_carries_its_contributions(explain_response):
    """Each hit parses its breakdown into typed contributions, in the order sent.

    The first hit matched the text and was also reached through the polly
    anchor; ``via`` and ``degree`` ride only on the graph sighting, and a text
    sighting leaves them unset rather than zero, since no anchor was involved.
    """
    result = RecallResult.from_json(explain_response["results"], explain=True)

    assert result.hits[0].contributions == [
        Contribution(source="text", score=1.0499527612542656, rank=0, count=1),
        Contribution(
            source="graph",
            score=1.9820803996745116,
            rank=0,
            count=2,
            via="polly",
            degree=3,
        ),
    ]


def test_a_hit_reached_through_the_graph_alone_says_so(explain_response):
    """A graph-only hit matched nothing itself: its one sighting is the graph's.

    This is the case explain exists to separate — a fact the graph reached and
    ranked low, rather than one the search never reached at all.
    """
    result = RecallResult.from_json(explain_response["results"], explain=True)

    graph_only = result.hits[2]
    assert graph_only.value == "the parrot sleeps in the kitchen"
    assert [c.source for c in graph_only.contributions] == ["graph"]
    assert graph_only.contributions[0].via == "polly"


def test_an_explained_result_carries_the_background_rate(explain_response):
    """The query-level background rate lands on the result, as a float."""
    result = RecallResult.from_json(explain_response["results"], explain=True)

    assert result.background == 0.4857013396824596


def test_an_omitted_background_is_zero_on_an_explained_result(
    anchor_explain_response,
):
    """The server omits a background rate of zero, and explain reads it as 0.0.

    An anchor-only recall runs no traversal, so its background rate is zero
    and the key is absent. On an explained result that absence is a value, not
    a missing one — ``None`` would read as "not asked for".
    """
    result = RecallResult.from_json(anchor_explain_response["results"], explain=True)

    assert result.background == 0.0
    assert {c.source for hit in result for c in hit.contributions} == {"anchor"}


def test_a_plain_recall_carries_no_breakdown():
    """Without explain there is no breakdown: no contributions, no background.

    ``background`` is ``None`` rather than 0.0 so a caller can tell a result
    that was never explained from one whose background rate was zero.
    """
    result = RecallResult.from_json(
        {"count": 1, "hits": [{"value": "polly whistles at dawn", "score": 1.0}]}
    )

    assert result.hits[0].contributions == []
    assert result.background is None


@pytest.mark.integration
def test_the_response_parses_into_the_declared_types(tide_result):
    """Every hit the server sent becomes a Hit with the declared field types."""
    assert isinstance(tide_result, RecallResult)
    assert tide_result.hits
    for hit in tide_result.hits:
        assert isinstance(hit, Hit)
        # from_json applies float() to `score`, so the score check below would
        # pass on a numeric string too: it pins the parsed type, not the wire
        # type.
        assert isinstance(hit.value, str)
        assert isinstance(hit.score, float)


@pytest.mark.integration
def test_the_server_count_agrees_with_the_hits_it_sent(tide_result):
    """The reported count matches the hits beside it.

    from_json prefers the server's count and only falls back to len(hits), so
    the fallback would hide a disagreement between the two.
    """
    assert tide_result.count == len(tide_result.hits)


@pytest.mark.integration
def test_hits_arrive_ranked_by_descending_score(tide_result):
    """Hits come back in ranked order.

    RecallResult documents "in ranked order" and nothing in the SDK sorts, so
    this is a claim about the server that the SDK passes through unchanged.
    """
    scores = [hit.score for hit in tide_result]
    assert scores == sorted(scores, reverse=True)


@pytest.mark.integration
def test_timestamps_are_populated_and_rfc3339(tide_result):
    """Every hit carries a parseable RFC 3339 instant.

    ``timestamp`` is Optional on the dataclass, so every unit test would still
    pass if the server stopped sending it. This is what notices. The server's
    nanosecond precision is truncated to microseconds by fromisoformat rather
    than rejected, and the trailing Z is accepted, so parsing checks that the
    field is a real instant.
    """
    for hit in tide_result.hits:
        assert hit.timestamp is not None
        assert isinstance(datetime.fromisoformat(hit.timestamp), datetime)


@pytest.mark.integration
def test_len_and_iteration_agree_with_the_count(tide_result):
    """The container protocol RecallResult implements matches its own count."""
    assert len(tide_result) == tide_result.count
    assert len(list(tide_result)) == tide_result.count
    assert bool(tide_result) is True


@pytest.mark.integration
def test_an_empty_result_set_parses(client, models_graph, no_match):
    """A recall that matches nothing parses into an empty, falsey result."""
    empty = client.recall(no_match, graph=models_graph)
    assert empty.count == 0
    assert empty.hits == []
    assert len(empty) == 0
    assert list(empty) == []
    assert bool(empty) is False


@pytest.mark.integration
def test_a_warned_response_carries_the_warning_verbatim(tide_result):
    """A recall the server ran with a warning surfaces it as a list of strings.

    The fixture asks for depth:2 with no topic or entity, which the server
    answers from the text and vector indices alone and flags. The exact text is
    pinned so a client can show it unchanged; the empty-list shape of a clean
    response is a unit concern, covered by the ``from_json`` tests above.
    """
    assert tide_result.warnings == [
        "parse warning at column 19: depth:2 has no effect: the graph is searched "
        "only through a topic:/entity: anchor and this recall names none, so it "
        "runs on the text and vector indices alone"
    ]


@pytest.mark.integration
def test_a_vector_recall_parses_the_same_shape(vector_tide_result):
    """A vector-seeded result arrives in the envelope models.py knows to read.

    Vector-seeded results travel a different path through the engine, so the
    shape they come back in is worth parsing separately from the text one.
    """
    assert isinstance(vector_tide_result, RecallResult)
    assert all(isinstance(hit.score, float) for hit in vector_tide_result)
    assert all(hit.timestamp for hit in vector_tide_result)


@pytest.mark.integration
def test_hit_values_come_back_exactly_as_written(client, models_graph):
    """A stored value is returned byte for byte, not re-tokenised or trimmed."""
    stored = "the mudflats are exposed at low water"
    client.remember(stored, graph=models_graph)
    result = client.recall("mudflats", graph=models_graph, depth=0)
    assert stored in [hit.value for hit in result]


@pytest.mark.integration
def test_scores_are_raw_fused_quantities(tide_result):
    """Scores are positive and arrive raw, on the scorer's own scale.

    The default excess scorer adds a hit's seed mass and any surplus its
    anchors transmitted, in raw seed units, and nothing normalises the sum, so
    a score at or above 1.0 is ordinary. A score is not a similarity in
    [0, 1]: clients must treat scores as an ordering, not a probability.
    """
    assert all(hit.score > 0 for hit in tide_result)
