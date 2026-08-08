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

"""OpenAIEmbedder tests against an injected fake client — no network."""

from fraise_sdk.providers.base import Embedder
from fraise_sdk.providers.openai import OpenAIEmbedder


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
    assert fake.record == {
        "model": "text-embedding-3-small",
        "input": "hello",
        "dimensions": 3,
    }


def test_openai_embedder_is_callable_and_omits_dimensions_when_unset():
    fake = _FakeOpenAI()
    embedder = OpenAIEmbedder(client=fake)
    embedder("world")  # __call__ inherited from the Embedder ABC
    assert "dimensions" not in fake.record
    assert fake.record["input"] == "world"


def test_openai_embedder_is_an_embedder():
    assert isinstance(OpenAIEmbedder(client=_FakeOpenAI()), Embedder)
