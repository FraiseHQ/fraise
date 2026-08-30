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


def test_an_error_with_a_non_json_body_still_raises(session, respond):
    """An error status whose body is not JSON still raises FraiseAPIError,
    surfacing the raw text — a proxy's HTML error page reaches the caller
    instead of being swallowed into silence.
    """
    response = respond(session, {}, status_code=502)
    response.json.side_effect = ValueError("not json")
    response.text = "<html>bad gateway</html>"
    with pytest.raises(FraiseAPIError) as excinfo:
        FraiseClient().recall("anything")
    assert excinfo.value.status_code == 502
    assert "bad gateway" in excinfo.value.message


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


def test_health_is_true_on_200(session, respond_get):
    """A 200 from the health endpoint answers True."""
    respond_get(session, {"status": "ok"})
    assert FraiseClient().health() is True


def test_health_is_false_on_a_non_200(session, respond_get):
    """A non-200 answers False: degraded is not healthy."""
    respond_get(session, {}, status_code=503)
    assert FraiseClient().health() is False


def test_health_is_false_when_unreachable(session):
    """A transport error is swallowed into False, never raised.

    health() is the probe callers use to decide whether to bother, so it has
    to answer even when nothing is listening.
    """
    session.get.side_effect = requests.ConnectionError("refused")
    assert FraiseClient().health() is False


def test_server_version_reads_the_health_field(session, respond_get):
    """The version travels in the health endpoint's ``version`` field."""
    respond_get(session, {"status": "ok", "version": "0.1.0"})
    assert FraiseClient().server_version() == "0.1.0"


def test_server_version_is_none_when_unreachable(session):
    """An unreachable server reports no version rather than raising."""
    session.get.side_effect = requests.ConnectionError("refused")
    assert FraiseClient().server_version() is None


def test_server_version_is_none_on_a_non_200(session, respond_get):
    """An error status yields None even when the body carries a version:
    an error page's claims are not the server's."""
    respond_get(session, {"version": "0.1.0"}, status_code=500)
    assert FraiseClient().server_version() is None


def test_server_version_is_none_on_a_non_json_body(session, respond_get):
    """A body that fails to decode yields None — a proxy's HTML error page
    is not a version."""
    response = respond_get(session, {})
    response.json.side_effect = ValueError("not json")
    assert FraiseClient().server_version() is None


@pytest.mark.parametrize("body", [{}, {"version": ""}, {"version": 3}, []])
def test_server_version_is_none_without_a_usable_version_field(
    session, respond_get, body
):
    """A decoded body without a non-empty string version yields None —
    servers predating version reporting and shape surprises alike."""
    respond_get(session, body)
    assert FraiseClient().server_version() is None


def test_check_compatibility_warns_when_the_version_is_unknown(session):
    """No readable version warns and answers False instead of guessing."""
    session.get.side_effect = requests.ConnectionError("refused")
    with pytest.warns(UserWarning, match="could not determine"):
        assert FraiseClient().check_compatibility() is False


def test_check_compatibility_strict_raises_when_the_version_is_unknown(session):
    """strict=True turns the unknown-version warning into a FraiseError."""
    session.get.side_effect = requests.ConnectionError("refused")
    with pytest.raises(FraiseError, match="could not determine"):
        FraiseClient().check_compatibility(strict=True)


@pytest.mark.parametrize("version", ["0.2.0", "99.0.0", "0.0.9", "0.1", "abc", "x.y.z"])
def test_check_compatibility_warns_outside_the_supported_range(
    session, respond_get, version
):
    """A version outside SUPPORTED_SERVER — above it, below it, or too
    mangled to parse at all — warns and answers False: the SDK keeps
    working, eyes open."""
    respond_get(session, {"version": version})
    with pytest.warns(UserWarning, match="outside this SDK's supported range"):
        assert FraiseClient().check_compatibility() is False


def test_check_compatibility_strict_raises_outside_the_supported_range(
    session, respond_get
):
    """strict=True turns the out-of-range warning into a FraiseError."""
    respond_get(session, {"version": "99.0.0"})
    with pytest.raises(FraiseError, match="outside this SDK's supported range"):
        FraiseClient().check_compatibility(strict=True)


@pytest.mark.parametrize("version", ["0.1.0", "v0.1.5", "0.1.9-beta.2"])
def test_check_compatibility_accepts_a_supported_version(session, respond_get, version):
    """An in-range version answers True with no warning — a ``v`` prefix and
    a pre-release suffix are spelling, not incompatibility. The literals sit
    inside SUPPORTED_SERVER (>=0.1.0,<0.2.0) and rot with it on purpose,
    like the integration suite's live pin."""
    respond_get(session, {"version": version})
    with warnings.catch_warnings():
        warnings.simplefilter("error")
        assert FraiseClient().check_compatibility() is True


# -- lifecycle ----------------------------------------------------------------


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
