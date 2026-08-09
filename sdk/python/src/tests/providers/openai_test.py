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

"""OpenAIEmbedder tests against a mocked openai client — no network."""

import sys
from unittest.mock import MagicMock, patch

from fraise_sdk.providers.base import Embedder
from fraise_sdk.providers.openai import OpenAIEmbedder

DEFAULT_MODEL = "text-embedding-3-small"


def _client(embedding=(0.1, 0.2, 0.3)) -> MagicMock:
    """A mock ``openai.OpenAI`` answering with one embedding."""
    client = MagicMock()
    client.embeddings.create.return_value = MagicMock(
        data=[MagicMock(embedding=list(embedding))]
    )
    return client


def test_openai_embedder_calls_client_and_returns_vector():
    client = _client()
    embedder = OpenAIEmbedder(model=DEFAULT_MODEL, client=client, dimensions=3)
    assert embedder.embed("hello") == [0.1, 0.2, 0.3]
    client.embeddings.create.assert_called_once_with(
        model=DEFAULT_MODEL, input="hello", dimensions=3
    )


def test_openai_embedder_is_callable_and_omits_dimensions_when_unset():
    client = _client()
    OpenAIEmbedder(client=client)("world")  # __call__ inherited from the Embedder ABC
    client.embeddings.create.assert_called_once_with(model=DEFAULT_MODEL, input="world")


def test_openai_embedder_is_an_embedder():
    assert isinstance(OpenAIEmbedder(client=_client()), Embedder)


def test_openai_embedder_builds_its_own_client_from_the_api_key():
    """Without an injected client the embedder imports openai and builds one.

    The import is lazy and lives inside ``__init__``, so it is patched in
    ``sys.modules`` — that keeps the test running whether or not the optional
    'openai' extra is installed.
    """
    openai = MagicMock()
    openai.OpenAI.return_value = _client()
    with patch.dict(sys.modules, {"openai": openai}):
        embedder = OpenAIEmbedder(api_key="sk-test")
    openai.OpenAI.assert_called_once_with(api_key="sk-test")
    assert embedder.embed("hello") == [0.1, 0.2, 0.3]
