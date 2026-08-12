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

"""Client tests with requests.Session patched out — no server required."""

import json
from unittest.mock import MagicMock, patch

import pytest
import requests
from fraise_sdk import FraiseAPIError, FraiseClient, FraiseError
from fraise_sdk.client import DEFAULT_BASE_URL, DEFAULT_TIMEOUT_SECONDS

QUERY_URL = f"{DEFAULT_BASE_URL}/api/v1/q"
NO_HITS = {"results": {"count": 0, "hits": []}}


@pytest.fixture
def session():
    """The session the client builds for itself, patched at its import site.

    Patching rather than injecting keeps the client's own construction path —
    the one every caller takes — under test.

    Yields:
        The mock session every FraiseClient built in the test will use, armed
        to answer with an empty recall result.
    """
    with patch("fraise_sdk.client.requests.Session") as session_class:
        session = session_class.return_value
        _respond(session, NO_HITS)
        yield session


def _respond(session, body: dict, status_code: int = 200) -> MagicMock:
    """Arm the session to answer the next POST with ``body``."""
    response = MagicMock(
        status_code=status_code,
        ok=200 <= status_code < 300,
        text=json.dumps(body),
    )
    response.json.return_value = body
    session.post.return_value = response
    return response


def _sent(session) -> dict:
    """The JSON payload of the single POST the client made."""
    session.post.assert_called_once()
    return session.post.call_args.kwargs["json"]


def _encode(text: str) -> list[float]:
    """A bare ``callable(text) -> vector`` embedder: the text's length, 4 times."""
    return [float(len(text))] * 4


def _callable_embedder() -> MagicMock:
    """A mock of the bare ``callable(text) -> vector`` embedder shape."""
    embedder = MagicMock(side_effect=_encode)
    # A plain callable has no .embed — deleting it is what sends
    # resolve_embedder down the callable branch instead of the Embedder one.
    del embedder.embed
    return embedder


def test_remember_posts_expected_query(session):
    FraiseClient().remember("the parrot is turquoise", graph=3, topics=["color"])
    session.post.assert_called_once_with(
        QUERY_URL,
        json={"query": "remember@3 'the parrot is turquoise' topic:color"},
        timeout=DEFAULT_TIMEOUT_SECONDS,
    )


def test_remember_with_vector_sends_parameters(session):
    FraiseClient().remember("kingfisher is blue", graph=6, vector=[0.5, 0.5])
    assert _sent(session) == {
        "query": "remember@6 'kingfisher is blue' vec:$v",
        "parameters": {"v": [0.5, 0.5]},
    }


def test_recall_parses_hits(session):
    _respond(
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

    assert _sent(session)["query"] == "recall@7 mars venus top:10"
    assert result.count == 2
    assert [h.value for h in result] == ["mars is the red planet", "venus is hot"]
    assert result.hits[0].score == 1.0
    assert bool(result) is True


def test_recall_empty_results(session):
    result = FraiseClient().recall("nothingindexed")
    assert result.count == 0
    assert list(result) == []
    assert bool(result) is False


def test_api_error_surfaces_server_message(session):
    _respond(session, {"error": "could not parse query"}, status_code=400)
    with pytest.raises(FraiseAPIError) as excinfo:
        FraiseClient().recall("bogus")
    assert excinfo.value.status_code == 400
    assert "could not parse query" in excinfo.value.message


def test_unreachable_server_raises_fraise_error(session):
    session.post.side_effect = requests.ConnectionError("refused")
    with pytest.raises(FraiseError, match="could not reach fraise"):
        FraiseClient().recall("anything")


def test_closing_closes_the_session_the_client_owns(session):
    with FraiseClient():
        pass
    session.close.assert_called_once_with()


def test_an_injected_session_is_left_open():
    # The caller owns what the caller passed in; closing it would be rude.
    injected = MagicMock()
    _respond(injected, NO_HITS)
    with FraiseClient(session=injected):
        pass
    injected.close.assert_not_called()


# -- embedding --------------------------------------------------------------


def test_configured_embedder_encodes_remember_value(session):
    embedder = _callable_embedder()
    FraiseClient(embedder=embedder).remember("the parrot is turquoise", graph=6)
    assert _sent(session) == {
        "query": "remember@6 'the parrot is turquoise' vec:$v",
        "parameters": {"v": [23.0] * 4},  # len("the parrot is turquoise")
    }
    embedder.assert_called_once_with("the parrot is turquoise")


def test_configured_embedder_encodes_recall_keywords(session):
    embedder = _callable_embedder()
    FraiseClient(embedder=embedder).recall("kingfisher", "blue", graph=6)
    assert _sent(session)["query"] == "recall@6 kingfisher blue vec:$v"
    # Defaults to the space-joined keywords when no explicit query phrase is given.
    embedder.assert_called_once_with("kingfisher blue")


def test_recall_query_phrase_overrides_keywords_for_embedding(session):
    embedder = _callable_embedder()
    FraiseClient(embedder=embedder).recall(
        "zzznomatch", graph=6, query="a sleepy kitten in the sun"
    )
    embedder.assert_called_once_with("a sleepy kitten in the sun")
    # The question itself travels as one quoted phrase term, ahead of the
    # bare keywords — never as unquoted words the grammar could claim.
    assert (
        _sent(session)["query"]
        == "recall@6 'a sleepy kitten in the sun' zzznomatch vec:$v"
    )


def test_explicit_vector_wins_over_embedder(session):
    embedder = _callable_embedder()
    FraiseClient(embedder=embedder).remember("x is y", graph=6, vector=[0.1, 0.2])
    assert _sent(session)["parameters"] == {"v": [0.1, 0.2]}
    embedder.assert_not_called()


def test_embed_false_skips_a_configured_embedder(session):
    embedder = _callable_embedder()
    FraiseClient(embedder=embedder).remember("x is y", graph=6, embed=False)
    assert "parameters" not in _sent(session)
    embedder.assert_not_called()


def test_embed_true_without_embedder_raises(session):
    with pytest.raises(FraiseError, match="no embedder"):
        FraiseClient().remember("x is y", embed=True)


def test_no_embedder_sends_no_vector(session):
    FraiseClient().remember("x is y", graph=6)
    assert "parameters" not in _sent(session)


def test_embedder_object_is_called_through_its_embed_method(session):
    # An Embedder exposes both .embed and __call__; the client must take the
    # named method, or __call__ would recurse straight back into it.
    embedder = MagicMock()
    embedder.embed.return_value = [1.0, 2.0, 3.0]
    FraiseClient(embedder=embedder).remember("hello world", graph=6)
    assert _sent(session)["parameters"] == {"v": [1.0, 2.0, 3.0]}
    embedder.embed.assert_called_once_with("hello world")
    embedder.assert_not_called()
