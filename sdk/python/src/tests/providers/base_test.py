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

"""Tests for the embedder resolver and the Embedder base class."""

import pytest
from fraise_sdk.providers.base import Embedder, resolve_embedder


def test_resolve_none():
    assert resolve_embedder(None) is None


def test_resolve_callable():
    fn = lambda text: [1.0]
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
