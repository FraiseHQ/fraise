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

"""Client tests against a fake requests.Session — no server required."""

import json

import pytest

from fraise_sdk import FraiseAPIError, FraiseClient, FraiseError


class _FakeResponse:
    def __init__(self, status_code, body):
        self.status_code = status_code
        self.ok = 200 <= status_code < 300
        self._body = body
        self.text = json.dumps(body)

    def json(self):
        return self._body


class _FakeSession:
    """Records the last POST and replays a queued response."""

    def __init__(self, response):
        self._response = response
        self.last_post = None

    def post(self, url, json=None, timeout=None):
        self.last_post = {"url": url, "json": json, "timeout": timeout}
        return self._response

    def close(self):
        pass


def _client(response, **kwargs):
    return FraiseClient(session=_FakeSession(response), **kwargs)


def test_remember_posts_expected_query():
    client = _client(_FakeResponse(200, {"results": {"count": 0, "hits": []}}))
    client.remember("the parrot is turquoise", graph=3, topics=["color"])
    sent = client._session.last_post
    assert sent["url"].endswith("/api/v1/q")
    assert sent["json"] == {"query": "remember@3 'the parrot is turquoise' topic:color"}


def test_remember_with_vector_sends_parameters():
    client = _client(_FakeResponse(200, {"results": {"count": 0, "hits": []}}))
    client.remember("kingfisher is blue", graph=6, vector=[0.5, 0.5])
    sent = client._session.last_post["json"]
    assert sent["query"] == "remember@6 'kingfisher is blue' vec:$v"
    assert sent["parameters"] == {"v": [0.5, 0.5]}


def test_recall_parses_hits():
    body = {
        "results": {
            "count": 2,
            "hits": [
                {"value": "mars is the red planet", "score": 1.0, "timestamp": "2026-01-01T00:00:00Z"},
                {"value": "venus is hot", "score": 0.42, "timestamp": "2026-01-01T00:00:00Z"},
            ],
        }
    }
    client = _client(_FakeResponse(200, body))
    result = client.recall("mars", "venus", graph=7, top=10)

    assert client._session.last_post["json"]["query"] == "recall@7 mars venus top:10"
    assert result.count == 2
    assert [h.value for h in result] == ["mars is the red planet", "venus is hot"]
    assert result.hits[0].score == 1.0
    assert bool(result) is True


def test_recall_empty_results():
    client = _client(_FakeResponse(200, {"results": {"count": 0, "hits": []}}))
    result = client.recall("nothingindexed")
    assert result.count == 0
    assert list(result) == []
    assert bool(result) is False


def test_api_error_surfaces_server_message():
    client = _client(_FakeResponse(400, {"error": "could not parse query"}))
    with pytest.raises(FraiseAPIError) as excinfo:
        client.recall("bogus")
    assert excinfo.value.status_code == 400
    assert "could not parse query" in excinfo.value.message


# -- embedding --------------------------------------------------------------


def _fixed_embedder(dim=4):
    """A deterministic stand-in embedder: records calls, returns a fixed vector."""
    calls: list[str] = []

    def embed(text: str) -> list[float]:
        calls.append(text)
        return [float(len(text))] * dim

    return embed, calls


def test_configured_embedder_encodes_remember_value():
    embed, calls = _fixed_embedder()
    client = _client(_FakeResponse(200, {"results": {"count": 0, "hits": []}}), embedder=embed)
    client.remember("the parrot is turquoise", graph=6)
    sent = client._session.last_post["json"]
    assert sent["query"] == "remember@6 'the parrot is turquoise' vec:$v"
    assert sent["parameters"] == {"v": [23.0, 23.0, 23.0, 23.0]}  # len("the parrot is turquoise")
    assert calls == ["the parrot is turquoise"]


def test_configured_embedder_encodes_recall_keywords():
    embed, calls = _fixed_embedder()
    client = _client(_FakeResponse(200, {"results": {"count": 0, "hits": []}}), embedder=embed)
    client.recall("kingfisher", "blue", graph=6)
    sent = client._session.last_post["json"]
    assert sent["query"] == "recall@6 kingfisher blue vec:$v"
    # Defaults to the space-joined keywords when no explicit query phrase is given.
    assert calls == ["kingfisher blue"]


def test_recall_query_phrase_overrides_keywords_for_embedding():
    embed, calls = _fixed_embedder()
    client = _client(_FakeResponse(200, {"results": {"count": 0, "hits": []}}), embedder=embed)
    client.recall("zzznomatch", graph=6, query="a sleepy kitten in the sun")
    assert calls == ["a sleepy kitten in the sun"]


def test_explicit_vector_wins_over_embedder():
    embed, calls = _fixed_embedder()
    client = _client(_FakeResponse(200, {"results": {"count": 0, "hits": []}}), embedder=embed)
    client.remember("x is y", graph=6, vector=[0.1, 0.2])
    assert client._session.last_post["json"]["parameters"] == {"v": [0.1, 0.2]}
    assert calls == []  # embedder not consulted


def test_embed_false_skips_a_configured_embedder():
    embed, calls = _fixed_embedder()
    client = _client(_FakeResponse(200, {"results": {"count": 0, "hits": []}}), embedder=embed)
    client.remember("x is y", graph=6, embed=False)
    assert "parameters" not in client._session.last_post["json"]
    assert calls == []


def test_embed_true_without_embedder_raises():
    client = _client(_FakeResponse(200, {"results": {"count": 0, "hits": []}}))
    with pytest.raises(FraiseError, match="no embedder"):
        client.remember("x is y", embed=True)


def test_no_embedder_sends_no_vector():
    client = _client(_FakeResponse(200, {"results": {"count": 0, "hits": []}}))
    client.remember("x is y", graph=6)
    assert "parameters" not in client._session.last_post["json"]


def test_embedder_abc_subclass_is_accepted():
    from fraise_sdk.providers import Embedder

    class Const(Embedder):
        def embed(self, text):
            return [1.0, 2.0, 3.0]

    client = _client(_FakeResponse(200, {"results": {"count": 0, "hits": []}}), embedder=Const())
    client.remember("hello world", graph=6)
    assert client._session.last_post["json"]["parameters"] == {"v": [1.0, 2.0, 3.0]}
