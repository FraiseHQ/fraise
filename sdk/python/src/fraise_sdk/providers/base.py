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

"""The provider contracts — embedder and extractor — and their resolvers.

:class:`Embedder` is the abstract base an embedding provider subclasses:
implement ``embed(text) -> Sequence[float]`` and instances are usable directly
and as a callable. :class:`Extractor` is its counterpart for anchors:
implement ``extract(text, context) -> list[Anchor]``, where the
:class:`Context` is what the message's graph already holds.
:class:`~fraise_sdk.client.FraiseClient` accepts either, or for convenience any
bare callable of the same shape.

Concrete providers live in sibling modules and import from here, never from the
package root — that keeps ``providers/__init__.py`` a pure re-export and avoids
a cycle between the package and its own submodules.
"""

from __future__ import annotations

from abc import ABC, abstractmethod
from collections.abc import Callable, Sequence
from dataclasses import dataclass
from typing import Literal

# For callers who would rather pass a plain function than subclass Embedder.
EmbedderLike = Callable[[str], Sequence[float]]


@dataclass(frozen=True)
class Anchor:
    """One anchor a message is filed under: a topic or an entity.

    ``type`` says which clause it becomes on ``remember`` — ``topic:`` or
    ``entity:`` — and ``value`` is what follows the ``:``. A recall names
    anchors to raise the facts filed under them, and the graph transmits
    through them; the message text itself is never rewritten to carry them.
    """

    value: str
    type: Literal["topic", "entity"]


@dataclass(frozen=True)
class Context:
    """What a graph already holds when its next message is tagged.

    Facts reach one another through the graph only by sharing an anchor. A
    message tagged in isolation loses that twice: "she signed the papers"
    names nobody, and a subject that recurs sessions apart comes back under a
    new spelling each time, so the walk has no anchor to cross. ``recent``
    lets the extractor resolve a reference to what a nearby message named;
    ``vocabulary`` lets it file a recurring subject under the anchor the graph
    already has. The client keeps one per graph; an empty one is a graph that
    holds nothing yet.

    Attributes:
        recent: the messages stored in the graph just before this one, oldest
            first.
        vocabulary: the anchors extraction filed the graph's facts under,
            spelled as the server stores them (trimmed, lower case), most used
            first.
    """

    recent: tuple[str, ...] = ()
    vocabulary: tuple[Anchor, ...] = ()


# For callers who would rather pass a plain function than subclass Extractor.
ExtractorLike = Callable[[str, Context], Sequence[Anchor]]


class Embedder(ABC):
    """Abstract base for anything that encodes text into a vector.

    Subclasses implement :meth:`embed`; :meth:`__call__` then comes for free, so
    an instance works anywhere a plain ``callable(text)`` is expected.
    """

    @abstractmethod
    def embed(self, text: str) -> Sequence[float]:
        """Encode ``text`` into a fixed-length sequence of floats."""
        raise NotImplementedError

    def __call__(self, text: str) -> Sequence[float]:
        """Encode ``text``, so an instance is usable as a bare callable."""
        return self.embed(text)


class Extractor(ABC):
    """Abstract base for anything that finds the anchors a message belongs under.

    Subclasses implement :meth:`extract`; :meth:`__call__` then comes for free,
    so an instance works anywhere a plain ``callable(text, context)`` is
    expected. An extractor may raise when it cannot read a message: the client
    stores the message anyway, without the extracted anchors, and warns.
    """

    @abstractmethod
    def extract(self, text: str, context: Context) -> list[Anchor]:
        """Return the topics and entities ``text`` is about, as anchors.

        Args:
            text: the message to tag, as it will be stored.
            context: what the message's graph already holds, for resolving
                its references and reusing the graph's anchors.
        """
        raise NotImplementedError

    def __call__(self, text: str, context: Context) -> list[Anchor]:
        """Extract ``text``'s anchors, so an instance is usable as a callable."""
        return self.extract(text, context)


def resolve_embedder(embedder: Embedder | EmbedderLike | None) -> EmbedderLike | None:
    """Normalize an :class:`Embedder`, a bare callable, or None to one function.

    Prefers an ``.embed`` method (an :class:`Embedder`) over calling the object
    directly, so an embedder exposing both stays on its named method.

    Raises:
        TypeError: if embedder is neither an Embedder nor a callable
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


def resolve_extractor(
    extractor: Extractor | ExtractorLike | None,
) -> ExtractorLike | None:
    """Normalize an :class:`Extractor`, a bare callable, or None to one function.

    Prefers an ``.extract`` method (an :class:`Extractor`) over calling the
    object directly, so an extractor exposing both stays on its named method.

    Raises:
        TypeError: if extractor is neither an Extractor nor a callable
    """
    if extractor is None:
        return None
    extract = getattr(extractor, "extract", None)
    if callable(extract):
        return extract
    if callable(extractor):
        return extractor
    raise TypeError(
        "extractor must be an Extractor (with an .extract method) or a callable, "
        f"got {type(extractor).__name__}"
    )
