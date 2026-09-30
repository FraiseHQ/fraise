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

"""Providers — backends that embed text into vectors or find its anchors.

The contracts live in :mod:`fraise_sdk.providers.base`; concrete providers live
in their own modules and depend on their own optional extras — currently
:class:`OpenAIEmbedder` and :class:`OpenAIExtractor` (``fraise-sdk[openai]``),
and :class:`HuggingFaceEmbedder` (``fraise-sdk[huggingface]``). Importing this
package pulls in no vendor SDK: each provider imports its client inside
``__init__``, not at module scope.
"""

from fraise_sdk.providers.base import (
    Anchor,
    Embedder,
    EmbedderLike,
    Extractor,
    ExtractorLike,
    resolve_embedder,
    resolve_extractor,
)
from fraise_sdk.providers.huggingface import HuggingFaceEmbedder
from fraise_sdk.providers.openai import OpenAIEmbedder, OpenAIExtractor

__all__ = [
    "Anchor",
    "Embedder",
    "EmbedderLike",
    "Extractor",
    "ExtractorLike",
    "HuggingFaceEmbedder",
    "OpenAIEmbedder",
    "OpenAIExtractor",
    "resolve_embedder",
    "resolve_extractor",
]
