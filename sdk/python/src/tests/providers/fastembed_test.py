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
"""FastEmbedEmbedder tests against a mocked model class, with no download."""

from unittest.mock import patch

import pytest
from fraise_sdk.providers.base import Embedder

# The provider imports fastembed at module scope, so skip the whole file when
# the optional 'fastembed' extra is not installed.
pytest.importorskip("fastembed", reason="requires the 'fastembed' extra")

from fraise_sdk.providers.fastembed import FastEmbedEmbedder  # noqa: E402

_TEXT_EMBEDDING = "fraise_sdk.providers.fastembed.fastembed.TextEmbedding"


def test_fastembed_embedder_loads_the_default_model_and_returns_vector(
    text_embedding, fastembed_default_model
):
    """embed builds the default model, encodes the text alone, and returns its vector."""
    cls = text_embedding()
    with patch(_TEXT_EMBEDDING, cls):
        got = FastEmbedEmbedder().embed("hello")
    assert got == [0.1, 0.2, 0.3]
    cls.assert_called_once_with(fastembed_default_model, cache_dir=None)
    cls.return_value.embed.assert_called_once_with(["hello"])


def test_fastembed_embedder_loads_the_model_on_first_embed_only(text_embedding):
    """Construction loads nothing, and the model loaded once serves every later embed.

    Loading reads the weights, and downloads them the first time. A tool server
    that builds the embedder at startup must not pay that before its first
    vector, nor again on every call after it.
    """
    cls = text_embedding()
    with patch(_TEXT_EMBEDDING, cls):
        embedder = FastEmbedEmbedder("BAAI/bge-base-en-v1.5", cache_dir="/models")
        cls.assert_not_called()
        embedder.embed("one")
        embedder("two")  # __call__ from the Embedder ABC
    cls.assert_called_once_with("BAAI/bge-base-en-v1.5", cache_dir="/models")
    assert cls.return_value.embed.call_count == 2


def test_fastembed_embedder_is_an_embedder():
    """FastEmbedEmbedder satisfies the Embedder contract the client resolves."""
    assert isinstance(FastEmbedEmbedder(), Embedder)


def test_fastembed_embedder_returns_plain_floats(text_embedding):
    """Whatever numbers tolist() yields, ints included, come back as plain floats."""
    with patch(_TEXT_EMBEDDING, text_embedding(values=[0, 1])):
        got = FastEmbedEmbedder().embed("hello")
    assert got == [0.0, 1.0]
    assert all(type(value) is float for value in got)
