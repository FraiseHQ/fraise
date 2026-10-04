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

"""Typed views over the server's JSON responses."""

from __future__ import annotations

from collections.abc import Iterable, Sequence
from dataclasses import dataclass, field


@dataclass(frozen=True)
class Contribution:
    """One retrieval source's sighting of a hit, as ``explain`` reports it.

    ``source`` names the stage that saw the fact (``text``, ``vector``,
    ``graph`` or ``anchor``) and ``score`` is that stage's raw mass, not a share
    of the hit's score: the hit's score is the final value, after transmission
    and recency decay, so its contributions are its ingredients and do not sum
    to it. ``rank`` is the hit's place in that source's own list, 0 first, and
    ``count`` how many seeds funded the sighting. A ``graph`` or ``anchor``
    sighting also names the anchor it came through (``via``) and that anchor's
    ``degree``; a ``text`` or ``vector`` one has neither.
    """

    source: str
    score: float
    rank: int
    count: int
    via: str | None = None
    degree: int | None = None

    @classmethod
    def from_json(cls, data: dict) -> Contribution:  # noqa: D102
        return cls(
            source=data["source"],
            score=float(data["score"]),
            rank=int(data["rank"]),
            count=int(data["count"]),
            via=data.get("via"),
            degree=data.get("degree"),
        )


@dataclass(frozen=True)
class Hit:
    """One recalled fact and how strongly it matched the query.

    ``contributions`` is the per-source breakdown of the score, filled by
    :meth:`~fraise_sdk.client.FraiseClient.explain` and empty on a plain recall,
    which does not ask the server for it.
    """

    value: str
    score: float
    timestamp: str | None = None
    contributions: list[Contribution] = field(default_factory=list)

    @classmethod
    def from_json(cls, data: dict) -> Hit:  # noqa: D102
        return cls(
            value=data["value"],
            score=float(data["score"]),
            timestamp=data.get("timestamp"),
            contributions=[
                Contribution.from_json(c) for c in data.get("contributions") or []
            ],
        )


@dataclass(frozen=True)
class RecallResult:
    """The result of a ``recall``: the hits, in ranked order, and how many came back.

    ``warnings`` carries any warnings the server attached: the query ran and
    the hits are valid, but something in it cannot help or may not be what was
    meant (e.g. a stop-word term that can never match). Empty for a clean query.

    ``empty`` is about the graph, not the result set; ``bool(result)`` is what
    reports whether anything came back. It separates the two ways a recall comes
    back with nothing: False is the ordinary miss, where the graph holds facts
    and none of them matched, so the query is what to change; True means the
    graph searched holds nothing at all, so no rephrasing would have helped. The
    server carries the difference in the status line (204 for the empty graph).

    ``background`` is the query's background rate, the seed mass per unit of
    anchor degree the search observed. With each hit's contributions it is
    every input of the scoring fold, so a caller can see why a fact ranked
    where it did. It is set by ``explain`` and ``None`` on a plain recall.
    """

    count: int
    hits: list[Hit]
    warnings: list[str] = field(default_factory=list)
    empty: bool = False
    background: float | None = None

    @classmethod
    def from_json(
        cls,
        results: dict,
        warnings: Sequence[str] | None = None,
        *,
        empty: bool = False,
        explain: bool = False,
    ) -> RecallResult:
        """Parse the server's ``results`` object, with any response warnings.

        Args:
            results: the ``results`` member of the response body. Empty for a
                204, which has no body to carry one.
            warnings: the response's ``warnings`` list, which sits beside
                ``results`` rather than inside it — the caller holding the
                whole body passes it through here. ``None`` (a clean response,
                or a pre-warnings server) parses to an empty list.
            empty: whether the server answered 204, i.e. the graph searched
                holds nothing. It rides the status line rather than the body,
                so the caller that saw the response passes it in.
            explain: whether the response came from the explain route. The
                server omits a background rate of zero, so its absence reads
                as 0.0 on an explained result and as ``None`` on a plain one.

        Returns:
            The typed result, warnings included.
        """
        hits = [Hit.from_json(h) for h in results.get("hits") or []]
        # Prefer the server-reported count, falling back to the hit count so the
        # two never disagree if the field is ever omitted.
        return cls(
            count=results.get("count", len(hits)),
            hits=hits,
            warnings=list(warnings or []),
            empty=empty,
            background=float(results.get("background", 0.0)) if explain else None,
        )

    def __bool__(self) -> bool:
        return bool(self.hits)

    def __iter__(self) -> Iterable:
        return iter(self.hits)

    def __len__(self) -> int:
        return len(self.hits)
