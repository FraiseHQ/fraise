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

"""Tests for the provider resolvers and the Embedder and Extractor bases."""

from unittest.mock import MagicMock

import pytest
from fraise_sdk.providers.base import (
    Embedder,
    Extractor,
    resolve_embedder,
    resolve_extractor,
)


def test_resolve_none():
    assert resolve_embedder(None) is None


def test_resolve_callable():
    embedder = MagicMock(return_value=[1.0])
    # A plain callable has no .embed; deleting it is what makes this mock the
    # bare-callable shape rather than the Embedder one.
    del embedder.embed
    assert resolve_embedder(embedder) is embedder


def test_resolve_embedder_prefers_embed_method():
    embedder = MagicMock()
    resolved = resolve_embedder(embedder)
    # The bound .embed, not __call__ — which would recurse back into embed.
    assert resolved is embedder.embed
    resolved("abc")
    embedder.embed.assert_called_once_with("abc")
    embedder.assert_not_called()


def test_resolve_rejects_non_embedder():
    with pytest.raises(TypeError):
        resolve_embedder(object())


def test_embedder_abc_cannot_be_instantiated():
    with pytest.raises(TypeError):
        Embedder()  # abstract


def test_resolve_extractor_none():
    """No extractor resolves to None, so the client never extracts."""
    assert resolve_extractor(None) is None


def test_resolve_extractor_callable():
    """A bare callable is used as it is."""
    extractor = MagicMock(return_value=[])
    # A plain callable has no .extract; deleting it is what makes this mock the
    # bare-callable shape rather than the Extractor one.
    del extractor.extract
    assert resolve_extractor(extractor) is extractor


def test_resolve_extractor_prefers_extract_method():
    """An Extractor resolves to its bound ``extract``, never its ``__call__``.

    ``__call__`` delegates to ``extract``, so resolving to it would only add a
    hop — and a subclass overriding one of the two would split them.
    """
    extractor = MagicMock()
    resolved = resolve_extractor(extractor)
    assert resolved is extractor.extract
    resolved("abc")
    extractor.extract.assert_called_once_with("abc")
    extractor.assert_not_called()


def test_resolve_extractor_rejects_non_extractor():
    """Anything neither an Extractor nor callable is refused at construction."""
    with pytest.raises(TypeError, match="extractor must be"):
        resolve_extractor(object())


def test_extractor_abc_cannot_be_instantiated():
    """Extractor is a contract: it has no ``extract`` of its own."""
    with pytest.raises(TypeError):
        Extractor()  # abstract


def test_calling_an_extractor_extracts():
    """``__call__`` is ``extract``, so an Extractor works wherever a callable does."""
    extractor = MagicMock()
    Extractor.__call__(extractor, "the heron fishes at dawn")
    extractor.extract.assert_called_once_with("the heron fishes at dawn")
