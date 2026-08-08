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

"""Embedding providers — backends that turn text into vectors.

The contract lives in :mod:`fraise_sdk.providers.base`; concrete providers live
in their own modules and depend on their own optional extras — currently
:class:`OpenAIEmbedder` (``fraise-sdk[openai]``). Importing this package pulls
in no vendor SDK: ``openai`` is imported inside
:meth:`~fraise_sdk.providers.openai.OpenAIEmbedder.embed`, not at module scope.
"""

from fraise_sdk.providers.base import Embedder, EmbedderLike, resolve_embedder
from fraise_sdk.providers.openai import OpenAIEmbedder

__all__ = ["Embedder", "EmbedderLike", "OpenAIEmbedder", "resolve_embedder"]
