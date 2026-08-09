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

"""HuggingFaceEmbedder tests against a mocked inference client — no network."""

import sys
from unittest.mock import MagicMock, patch

import pytest

from fraise_sdk.providers.base import Embedder
from fraise_sdk.providers.huggingface import HuggingFaceEmbedder

DEFAULT_MODEL = "sentence-transformers/all-MiniLM-L6-v2"


def _client(values=(0.1, 0.2, 0.3)) -> MagicMock:
    """A mock ``huggingface_hub.InferenceClient`` answering with one array.

    ``feature_extraction`` really returns a numpy array, of which the embedder
    uses only ``tolist()``.
    """
    client = MagicMock()
    client.feature_extraction.return_value = MagicMock(
        **{"tolist.return_value": list(values)}
    )
    return client


def test_huggingface_embedder_calls_client_and_returns_vector():
    client = _client()
    embedder = HuggingFaceEmbedder(
        model=DEFAULT_MODEL, client=client, dimensions=3, normalize=True
    )
    assert embedder.embed("hello") == [0.1, 0.2, 0.3]
    client.feature_extraction.assert_called_once_with(
        "hello", model=DEFAULT_MODEL, dimensions=3, normalize=True
    )


def test_huggingface_embedder_is_callable_and_omits_unset_options():
    client = _client()
    HuggingFaceEmbedder(client=client)("world")  # __call__ from the Embedder ABC
    client.feature_extraction.assert_called_once_with("world", model=DEFAULT_MODEL)


def test_huggingface_embedder_is_an_embedder():
    assert isinstance(HuggingFaceEmbedder(client=_client()), Embedder)


def test_huggingface_embedder_returns_plain_floats():
    """tolist() output must survive as floats, not numpy scalars."""
    vector = HuggingFaceEmbedder(client=_client(values=[0, 1])).embed("hello")
    assert vector == [0.0, 1.0]
    assert all(type(value) is float for value in vector)


def test_huggingface_embedder_rejects_token_level_embeddings():
    embedder = HuggingFaceEmbedder(client=_client(values=[[0.1, 0.2], [0.3, 0.4]]))
    with pytest.raises(ValueError, match="token-level embeddings"):
        embedder.embed("hello")


def test_huggingface_embedder_accepts_a_plain_list():
    """A client returning a bare list (no .tolist) still works."""
    client = _client()
    client.feature_extraction.return_value = [0.5, 0.6]
    assert HuggingFaceEmbedder(client=client).embed("hi") == [0.5, 0.6]


def test_huggingface_embedder_builds_its_own_client_from_the_api_key():
    """Without an injected client the embedder imports the hub and builds one.

    The import is lazy and lives inside ``__init__``, so it is patched in
    ``sys.modules`` — that keeps the test running whether or not the optional
    'huggingface' extra is installed.
    """
    hub = MagicMock()
    hub.InferenceClient.return_value = _client()
    with patch.dict(sys.modules, {"huggingface_hub": hub}):
        embedder = HuggingFaceEmbedder(api_key="hf-test")
    hub.InferenceClient.assert_called_once_with(api_key="hf-test")
    assert embedder.embed("hello") == [0.1, 0.2, 0.3]
