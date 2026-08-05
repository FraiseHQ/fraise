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

"""Tests for the embedding providers and the embedder resolver."""

import pytest

from fraise_sdk.providers import Embedder, OpenAIEmbedder, resolve_embedder


def test_resolve_none():
    assert resolve_embedder(None) is None


def test_resolve_callable():
    fn = lambda text: [1.0]  # noqa: E731
    assert resolve_embedder(fn) is fn


def test_resolve_embedder_prefers_embed_method():
    class E(Embedder):
        def embed(self, text):
            return [len(text)]

    e = E()
    resolved = resolve_embedder(e)
    assert resolved("abc") == [3]  # bound .embed, not __call__ recursion


def test_resolve_rejects_non_embedder():
    with pytest.raises(TypeError):
        resolve_embedder(object())


def test_embedder_abc_cannot_be_instantiated():
    with pytest.raises(TypeError):
        Embedder()  # abstract


# -- OpenAIEmbedder with an injected fake client (no network) ----------------


class _FakeEmbeddings:
    def __init__(self, record):
        self._record = record

    def create(self, **kwargs):
        self._record.update(kwargs)
        return type(
            "Resp", (), {"data": [type("D", (), {"embedding": [0.1, 0.2, 0.3]})()]}
        )()


class _FakeOpenAI:
    def __init__(self):
        self.record: dict = {}
        self.embeddings = _FakeEmbeddings(self.record)


def test_openai_embedder_calls_client_and_returns_vector():
    fake = _FakeOpenAI()
    embedder = OpenAIEmbedder(model="text-embedding-3-small", client=fake, dimensions=3)
    vec = embedder.embed("hello")
    assert vec == [0.1, 0.2, 0.3]
    assert fake.record == {"model": "text-embedding-3-small", "input": "hello", "dimensions": 3}


def test_openai_embedder_is_callable_and_omits_dimensions_when_unset():
    fake = _FakeOpenAI()
    embedder = OpenAIEmbedder(client=fake)
    embedder("world")  # __call__ inherited from the Embedder ABC
    assert "dimensions" not in fake.record
    assert fake.record["input"] == "world"


def test_openai_embedder_is_an_embedder():
    assert isinstance(OpenAIEmbedder(client=_FakeOpenAI()), Embedder)
