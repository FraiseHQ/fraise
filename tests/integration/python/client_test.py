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

"""Integration tests for fraise_sdk.client against a live Fraise server.

The unit suite patches ``requests.Session`` out entirely, so everything the
client does *through* the wire — the query it builds, the vector it sends out of
band, the exception it raises for a real failure — is only asserted against a
mock there. This file is where those meet a server.
"""

import warnings

import pytest
import requests
from fraise_sdk import FraiseAPIError, FraiseClient, FraiseError


def test_client_works_as_a_context_manager(fraise_url):
    """A client built by ``with`` reaches the server inside the block."""
    with FraiseClient(fraise_url) as fraise:
        assert fraise.health() is True


def test_an_injected_session_is_left_open(fraise_url):
    """Closing the client leaves a caller-supplied session usable.

    The caller owns what the caller passed in. Asserted by using the session
    after the client is closed, rather than by inspecting a mock's close call.
    """
    session = requests.Session()
    with FraiseClient(fraise_url, session=session) as fraise:
        assert fraise.health() is True
    assert session.get(f"{fraise_url}/", timeout=10).status_code == 200
    session.close()


def test_client_connects(client):
    """The client reaches the server and reads back a version string."""
    assert client.health() is True
    assert client.server_version() is not None


def test_health_is_false_when_the_server_is_unreachable(dead_url):
    """health() swallows the transport error instead of raising.

    It is the probe callers use to decide whether to bother, so it has to
    answer even when nothing is listening.
    """
    assert FraiseClient(dead_url).health() is False


def test_server_version_is_none_when_the_server_is_unreachable(dead_url):
    """An unreachable server reports no version rather than raising."""
    assert FraiseClient(dead_url).server_version() is None


def test_check_compatibility_accepts_the_live_server(client):
    """The running server falls inside SUPPORTED_SERVER, silently.

    The one assertion here that rots on its own: SUPPORTED_SERVER is a
    hardcoded range, and this fails the day the image moves outside it.
    """
    with warnings.catch_warnings(record=True) as caught:
        warnings.simplefilter("always")
        assert client.check_compatibility() is True
    assert caught == []


def test_check_compatibility_strict_does_not_raise(client):
    """Strict mode passes against a supported server instead of raising."""
    assert client.check_compatibility(strict=True) is True


def test_remember_then_recall_returns_the_fact(client, round_trip_graph):
    """The round trip: a fact stored through the SDK is found through the SDK."""
    client.remember("the kettle whistles when the water boils", graph=round_trip_graph)
    result = client.recall("kettle", graph=round_trip_graph)
    assert "the kettle whistles when the water boils" in [h.value for h in result]


def test_a_lone_seed_hub_stays_silent(instrument_graph, client):
    """A single seed's topic hub holds exactly the background rate, so its
    siblings never surface on reachability alone — at any depth, since depth
    is inert (one anchor-mediated step; larger values reserved).

    This is the excess-transmission contract through the SDK: an anchor is
    heard only when its members matched better than its size predicts.
    """
    assert client.recall("cello", graph=instrument_graph, depth=1).count == 1
    assert client.recall("cello", graph=instrument_graph, depth=2).count == 1


def test_top_caps_the_number_of_results(instrument_graph, instrument_facts, client):
    """``top`` truncates a recall that would otherwise return every match.

    Every instrument fact contains "is", so the text index matches all of
    them directly.
    """
    capped = client.recall("is", graph=instrument_graph, top=2)
    assert capped.count == 2
    assert client.recall("is", graph=instrument_graph, top=10).count == len(
        instrument_facts
    )


def test_a_topic_filter_narrows_a_keyword_recall(
    instrument_graph, instrument_topic, client
):
    """A topic filter alongside a keyword is accepted and narrows the result
    to the tagged match itself.
    """
    filtered = client.recall(
        "cello", graph=instrument_graph, topics=[instrument_topic], depth=2
    )
    assert filtered.count == 1


def test_recall_without_a_match_is_empty(client, round_trip_graph, no_match):
    """A keyword no fact contains yields an empty, falsey result."""
    result = client.recall(no_match, graph=round_trip_graph)
    assert result.count == 0
    assert bool(result) is False


def test_a_configured_embedder_stores_and_finds_a_fact_by_vector(
    vector_graph, embedding_client
):
    """The implicit-vector path end to end.

    ``remember`` encodes the value, ``recall`` encodes the query phrase, and
    the server matches the two.
    """
    fact = "the barometer falls before a storm"
    embedding_client.remember(fact, graph=vector_graph)
    result = embedding_client.recall("barometer", graph=vector_graph, query=fact)
    assert fact in [h.value for h in result]


def test_an_explicit_vector_overrides_the_embedder(
    vector_graph, vector_dim, embedding_client
):
    """An explicit ``vector`` is what reaches the server, not the embedder's.

    Made observable by handing over a vector of the wrong width: the server
    refuses it. Had the client fallen back to encoding the value, the vector
    sent would have been the graph's width and the write would have succeeded,
    so this failing is what proves the override.
    """
    with pytest.raises(FraiseAPIError):
        embedding_client.remember(
            "the sundial needs no winding",
            graph=vector_graph,
            vector=[0.5] * (vector_dim // 2),
        )


def test_embed_false_stores_a_fact_without_a_vector(
    vector_graph, client, recalled_values
):
    """Opting out of embedding still produces a valid write.

    Read back through the plain client so the assertion is a pure keyword
    recall — an embedding client would also vector-match its way to every other
    fact in the graph.
    """
    fact = "the hourglass measures three minutes"
    client.remember(fact, graph=vector_graph, embed=False)
    assert fact in recalled_values("hourglass", vector_graph)


def test_a_mismatched_vector_dimension_is_rejected(vector_graph, vector_dim, client):
    """A vector of the wrong width is a 400 naming the expected and supplied
    dimensions: a client error the caller can correct, not a server fault to
    retry.
    """
    with pytest.raises(FraiseAPIError) as excinfo:
        client.remember(
            "a vector of the wrong width",
            graph=vector_graph,
            vector=[0.5] * (vector_dim // 2),
        )
    assert excinfo.value.status_code == 400
    assert f"expects {vector_dim}, got {vector_dim // 2}" in excinfo.value.message


def test_query_returns_the_decoded_body(client, round_trip_graph, no_match):
    """The raw escape hatch returns the server's decoded JSON body."""
    body = client.query(f"recall@{round_trip_graph} {no_match}")
    assert body["results"] == {"count": 0, "hits": []}


def test_query_surfaces_a_server_error_as_an_api_error(client):
    """A rejected query becomes FraiseAPIError carrying the server's message.

    The error envelope the client unpacks is the server's, not a fixture's, so
    this fails if the server stops reporting failures as ``{"error": ...}``.
    """
    with pytest.raises(FraiseAPIError) as excinfo:
        client.query("this is not a query")
    assert excinfo.value.status_code == 400
    assert "parse error" in excinfo.value.message


def test_an_unreachable_server_raises_fraise_error(dead_url, round_trip_graph):
    """A transport failure arrives as FraiseError, not a requests exception.

    A refused connection is the deterministic way to reach that branch; a
    timeout races a local server that answers faster than the clock.
    """
    with pytest.raises(FraiseError, match="could not reach fraise"):
        FraiseClient(dead_url).query(f"recall@{round_trip_graph} anything")
