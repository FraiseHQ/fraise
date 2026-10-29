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

"""Providers: backends that embed text into vectors or find its anchors.

The contracts live in :mod:`fraise_sdk.providers.base` and are re-exported
here. Concrete providers live in their own modules, each with its own optional
extra. Import them from those modules, not from this package.
:mod:`fraise_sdk.providers.openai` has ``OpenAIEmbedder`` and
``OpenAIExtractor``, and needs ``fraise-sdk[openai]``.
:mod:`fraise_sdk.providers.huggingface` has ``HuggingFaceEmbedder``, and needs
``fraise-sdk[huggingface]``. :mod:`fraise_sdk.providers.fastembed` has
``FastEmbedEmbedder``, which runs locally, and needs ``fraise-sdk[fastembed]``.
Keeping them out of this package keeps ``import fraise_sdk`` free of vendor
SDKs.
"""

from fraise_sdk.providers.base import (
    Anchor,
    Context,
    Embedder,
    EmbedderLike,
    Extractor,
    ExtractorLike,
    resolve_embedder,
    resolve_extractor,
)

__all__ = [
    "Anchor",
    "Context",
    "Embedder",
    "EmbedderLike",
    "Extractor",
    "ExtractorLike",
    "resolve_embedder",
    "resolve_extractor",
]
