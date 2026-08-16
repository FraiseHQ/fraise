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

"""Client tests with requests.Session patched out — no server required.

Setup lives in conftest.py; this file is assertions.
"""

import warnings
from unittest.mock import MagicMock

import pytest
import requests
from fraise_sdk import FraiseAPIError, FraiseClient, FraiseError, FraiseWarning
from fraise_sdk.client import DEFAULT_TIMEOUT_SECONDS


def test_remember_posts_expected_query(session, query_url):
    FraiseClient().remember("the parrot is turquoise", graph=3, topics=["color"])
    session.post.assert_called_once_with(
        query_url,
        json={"query": "remember@3 'the parrot is turquoise' topic:color"},
        timeout=DEFAULT_TIMEOUT_SECONDS,
    )


def test_remember_with_vector_sends_parameters(session, sent):
    FraiseClient().remember("kingfisher is blue", graph=6, vector=[0.5, 0.5])
    assert sent(session) == {
        "query": "remember@6 'kingfisher is blue' vec:$v",
        "parameters": {"v": [0.5, 0.5]},
    }


def test_recall_parses_hits(session, respond, sent):
    respond(
        session,
        {
            "results": {
                "count": 2,
                "hits": [
                    {
                        "value": "mars is the red planet",
                        "score": 1.0,
                        "timestamp": "2026-01-01T00:00:00Z",
                    },
                    {
                        "value": "venus is hot",
                        "score": 0.42,
                        "timestamp": "2026-01-01T00:00:00Z",
                    },
                ],
            }
        },
    )
    result = FraiseClient().recall("mars", "venus", graph=7, top=10)

    assert sent(session)["query"] == "recall@7 mars venus top:10"
    assert result.count == 2
    assert [h.value for h in result] == ["mars is the red planet", "venus is hot"]
    assert result.hits[0].score == 1.0
    assert bool(result) is True


def test_recall_empty_results(session):
    result = FraiseClient().recall("nothingindexed")
    assert result.count == 0
    assert list(result) == []
    assert bool(result) is False


def test_recall_surfaces_server_warnings(session, respond, server_warning):
    """A server warning reaches the caller on both channels: listed on the
    result for programmatic use, and emitted as a FraiseWarning so it is
    visible by default without any code changes.

    The armed response mimics ``recall since 7d``: the query ran — hits and
    all — while the server flagged that it is one ':' away from a since
    clause. Warnings ride beside the results, they do not replace them.
    """
    respond(
        session,
        {
            "results": {
                "count": 1,
                "hits": [{"value": "since the storm", "score": 1.0}],
            },
            "warnings": [server_warning],
        },
    )

    with pytest.warns(FraiseWarning, match="also a keyword"):
        result = FraiseClient().recall("since", "7d")

    assert result.warnings == [server_warning]
    assert result.count == 1


def test_recall_without_warnings_is_silent(session):
    """A response with no warnings field yields an empty list and emits
    nothing — the common, unambiguous path must stay quiet, so a caller who
    escalates warnings to errors is not tripped by clean queries.
    """
    with warnings.catch_warnings():
        warnings.simplefilter("error")
        result = FraiseClient().recall("zebras")

    assert result.warnings == []


def test_raw_query_emits_server_warnings(session, respond, server_warning):
    """The raw query() escape hatch emits FraiseWarning too: every operation
    funnels through it, so remember() and any future typed helper inherit the
    channel without plumbing of their own.
    """
    respond(
        session, {"results": {"count": 0, "hits": []}, "warnings": [server_warning]}
    )

    with pytest.warns(FraiseWarning, match="also a keyword"):
        body = FraiseClient().query("recall@0 since 7d")

    assert body["warnings"] == [server_warning]


def test_api_error_surfaces_server_message(session, respond):
    respond(session, {"error": "could not parse query"}, status_code=400)
    with pytest.raises(FraiseAPIError) as excinfo:
        FraiseClient().recall("bogus")
    assert excinfo.value.status_code == 400
    assert "could not parse query" in excinfo.value.message


def test_unreachable_server_raises_fraise_error(session):
    session.post.side_effect = requests.ConnectionError("refused")
    with pytest.raises(FraiseError, match="could not reach fraise"):
        FraiseClient().recall("anything")


def test_timed_out_server_raises_a_distinct_fraise_error(session):
    """A timeout gets its own message, naming the timeout, so it reads
    differently from a plain connection failure and points at the fix
    (raise ``timeout=``) instead of "could not reach fraise".
    """
    session.post.side_effect = requests.Timeout("timed out")
    with pytest.raises(FraiseError, match="timed out") as excinfo:
        FraiseClient(timeout=5.0).recall("anything")
    assert "could not reach fraise" not in str(excinfo.value)
    assert "5.0s" in str(excinfo.value)


def test_closing_closes_the_session_the_client_owns(session):
    with FraiseClient():
        pass
    session.close.assert_called_once_with()


def test_an_injected_session_is_left_open(respond, no_hits):
    # The caller owns what the caller passed in; closing it would be rude.
    injected = MagicMock()
    respond(injected, no_hits)
    with FraiseClient(session=injected):
        pass
    injected.close.assert_not_called()


# -- embedding --------------------------------------------------------------


def test_configured_embedder_encodes_remember_value(session, sent, callable_embedder):
    embedder = callable_embedder()
    FraiseClient(embedder=embedder).remember("the parrot is turquoise", graph=6)
    assert sent(session) == {
        "query": "remember@6 'the parrot is turquoise' vec:$v",
        "parameters": {"v": [23.0] * 4},  # len("the parrot is turquoise")
    }
    embedder.assert_called_once_with("the parrot is turquoise")


def test_configured_embedder_encodes_recall_keywords(session, sent, callable_embedder):
    embedder = callable_embedder()
    FraiseClient(embedder=embedder).recall("kingfisher", "blue", graph=6)
    assert sent(session)["query"] == "recall@6 kingfisher blue vec:$v"
    # Defaults to the space-joined keywords when no explicit query phrase is given.
    embedder.assert_called_once_with("kingfisher blue")


def test_recall_query_phrase_overrides_keywords_for_embedding(
    session, sent, callable_embedder
):
    embedder = callable_embedder()
    FraiseClient(embedder=embedder).recall(
        "zzznomatch", graph=6, query="a sleepy kitten in the sun"
    )
    embedder.assert_called_once_with("a sleepy kitten in the sun")
    # The question itself travels as one quoted phrase term, ahead of the
    # bare keywords — never as unquoted words the grammar could claim.
    assert (
        sent(session)["query"]
        == "recall@6 'a sleepy kitten in the sun' zzznomatch vec:$v"
    )


def test_explicit_vector_wins_over_embedder(session, sent, callable_embedder):
    embedder = callable_embedder()
    FraiseClient(embedder=embedder).remember("x is y", graph=6, vector=[0.1, 0.2])
    assert sent(session)["parameters"] == {"v": [0.1, 0.2]}
    embedder.assert_not_called()


def test_embed_false_skips_a_configured_embedder(session, sent, callable_embedder):
    embedder = callable_embedder()
    FraiseClient(embedder=embedder).remember("x is y", graph=6, embed=False)
    assert "parameters" not in sent(session)
    embedder.assert_not_called()


def test_embed_true_without_embedder_raises(session):
    with pytest.raises(FraiseError, match="no embedder"):
        FraiseClient().remember("x is y", embed=True)


def test_no_embedder_sends_no_vector(session, sent):
    FraiseClient().remember("x is y", graph=6)
    assert "parameters" not in sent(session)


def test_embedder_object_is_called_through_its_embed_method(session, sent):
    # An Embedder exposes both .embed and __call__; the client must take the
    # named method, or __call__ would recurse straight back into it.
    embedder = MagicMock()
    embedder.embed.return_value = [1.0, 2.0, 3.0]
    FraiseClient(embedder=embedder).remember("hello world", graph=6)
    assert sent(session)["parameters"] == {"v": [1.0, 2.0, 3.0]}
    embedder.embed.assert_called_once_with("hello world")
    embedder.assert_not_called()
