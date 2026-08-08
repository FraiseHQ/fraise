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

:class:`Embedder` is the abstract base every provider subclasses: implement
``embed(text) -> Sequence[float]`` and instances are usable directly and as a
callable. :class:`~fraise_sdk.client.FraiseClient` accepts an ``Embedder`` (or,
for convenience, any bare ``callable(text) -> Sequence[float]``).

Concrete providers live in submodules and depend on their own optional extras —
currently :class:`OpenAIEmbedder` (``fraise-sdk[openai]``). They are imported
lazily via :func:`__getattr__`, so importing this package pulls in no vendor SDK.
"""

from __future__ import annotations

from abc import ABC, abstractmethod
from collections.abc import Callable, Sequence
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from fraise_sdk.providers.openai import OpenAIEmbedder

# For callers who would rather pass a plain function than subclass Embedder.
EmbedderLike = Callable[[str], Sequence[float]]


class Embedder(ABC):
    """Abstract base for anything that encodes text into a vector.

    Subclasses implement :meth:`embed`; :meth:`__call__` then comes for free, so
    an instance works anywhere a plain ``callable(text)`` is expected.
    """

    @abstractmethod
    def embed(self, text: str) -> Sequence[float]:
        """Encode ``text`` into a fixed-length sequence of floats."""
        raise NotImplementedError

    def __call__(self, text: str) -> Sequence[float]:  # noqa: D102
        return self.embed(text)


def resolve_embedder(embedder: Embedder | EmbedderLike | None) -> EmbedderLike | None:
    """Normalize an :class:`Embedder`, a bare callable, or None to one function.

    Prefers an ``.embed`` method (an :class:`Embedder`) over calling the object
    directly, so an embedder exposing both stays on its named method.

    Raises:
        TypeError: if method is not of type Embedder or EmbbederLike
    """
    if embedder is None:
        return None
    embed = getattr(embedder, "embed", None)
    if callable(embed):
        return embed
    if callable(embedder):
        return embedder
    raise TypeError(
        "embedder must be an Embedder (with an .embed method) or a callable, "
        f"got {type(embedder).__name__}"
    )


def __getattr__(name: str) -> object:
    # Lazy re-export so `from fraise_sdk.providers import OpenAIEmbedder` works
    # without importing the openai SDK until it is actually referenced.
    if name == "OpenAIEmbedder":
        from fraise_sdk.providers.openai import OpenAIEmbedder

        return OpenAIEmbedder
    raise AttributeError(f"module {__name__!r} has no attribute {name!r}")


__all__ = ["Embedder", "EmbedderLike", "OpenAIEmbedder", "resolve_embedder"]
