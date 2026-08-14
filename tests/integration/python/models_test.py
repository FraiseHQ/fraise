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

"""Integration tests for fraise_sdk.models against a live Fraise server.

``Hit.from_json`` and ``RecallResult.from_json`` are the SDK's only reading of
the server's response envelope, and in the unit suite they are handed dicts a
test author wrote — so the shape they parse is asserted against itself. Here the
dicts come from the server, which is the only way a field being renamed, dropped
or retyped can fail a test.
"""

from datetime import datetime

from fraise_sdk.models import Hit, RecallResult


def test_the_response_parses_into_the_declared_types(tide_result):
    """Every hit the server sent becomes a Hit with the declared field types."""
    assert isinstance(tide_result, RecallResult)
    assert tide_result.hits
    for hit in tide_result.hits:
        assert isinstance(hit, Hit)
        # `score` is float() in from_json, so this would pass on a numeric
        # string too — the point is that the server keeps sending a number.
        assert isinstance(hit.value, str)
        assert isinstance(hit.score, float)


def test_the_server_count_agrees_with_the_hits_it_sent(tide_result):
    """The reported count matches the hits beside it.

    from_json prefers the server's count and only falls back to len(hits), so
    the fallback would hide a disagreement between the two.
    """
    assert tide_result.count == len(tide_result.hits)


def test_hits_arrive_ranked_by_descending_score(tide_result):
    """Hits come back in ranked order.

    RecallResult documents "in ranked order" and nothing in the SDK sorts, so
    this is a claim about the server that the SDK passes through unchanged.
    """
    scores = [hit.score for hit in tide_result]
    assert scores == sorted(scores, reverse=True)


def test_timestamps_are_populated_and_rfc3339(tide_result):
    """Every hit carries a parseable RFC 3339 instant.

    ``timestamp`` is Optional on the dataclass, so every unit test would still
    pass if the server stopped sending it. This is what notices. The server's
    nanosecond precision is truncated to microseconds by fromisoformat rather
    than rejected, and the trailing Z is accepted, so parsing is an honest
    check that the field is a real instant.
    """
    for hit in tide_result.hits:
        assert hit.timestamp is not None
        assert isinstance(datetime.fromisoformat(hit.timestamp), datetime)


def test_len_and_iteration_agree_with_the_count(tide_result):
    """The container protocol RecallResult implements matches its own count."""
    assert len(tide_result) == tide_result.count
    assert len(list(tide_result)) == tide_result.count
    assert bool(tide_result) is True


def test_an_empty_result_set_parses(client, models_graph, no_match):
    """A recall that matches nothing parses into an empty, falsey result."""
    empty = client.recall(no_match, graph=models_graph)
    assert empty.count == 0
    assert empty.hits == []
    assert len(empty) == 0
    assert list(empty) == []
    assert bool(empty) is False


def test_a_vector_recall_parses_the_same_shape(vector_tide_result):
    """A vector-seeded result arrives in the envelope models.py knows to read.

    Vector-seeded results travel a different path through the engine, so the
    shape they come back in is worth parsing separately from the text one.
    """
    assert isinstance(vector_tide_result, RecallResult)
    assert all(isinstance(hit.score, float) for hit in vector_tide_result)
    assert all(hit.timestamp for hit in vector_tide_result)


def test_hit_values_come_back_exactly_as_written(client, models_graph):
    """A stored value is returned byte for byte, not re-tokenised or trimmed."""
    stored = "the mudflats are exposed at low water"
    client.remember(stored, graph=models_graph)
    result = client.recall("mudflats", graph=models_graph, depth=1)
    assert stored in [hit.value for hit in result]


def test_scores_are_not_clamped_to_one(models_graph, client):
    """Scores are positive, additive mass — BM25 text plus vector similarity —
    on no fixed scale. A fact recalled by its own exact embedding carries
    similarity 1/(1+0) = 1 on top of any text mass at all, so its score
    exceeds 1.0 — which the obvious 0.0 <= score <= 1.0 assertion would get
    wrong.
    """
    value = "the spring tide reached the old sea wall"
    embedding = [0.5, -0.25, 0.75, 0.0, 0.5, -0.5]
    client.remember(value, graph=models_graph, vector=embedding)

    result = client.recall("tide", graph=models_graph, vector=embedding, embed=False)
    assert all(hit.score > 0 for hit in result)
    scores = {hit.value: hit.score for hit in result}
    assert scores[value] > 1.0, (
        f"text mass plus exact-vector similarity must exceed 1.0: {scores}"
    )
