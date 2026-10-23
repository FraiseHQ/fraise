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
class GraphStats:
    """One graph's shape, as ``GET /api/v1/stats`` reports it.

    The fields are named for what they count. Two differ from the wire, which
    uses the graph-theory names. ``vertices`` is the server's ``order`` (facts,
    topics and entities alike). ``edges`` is its ``size`` (a fact's links to
    the topics and entities it is filed under).

    The other fields keep their wire names. ``nodes`` is every stored node, the
    vertices and the relationships between them. ``vectors`` counts the vectors
    indexed. ``forest_entries`` counts the vector forest's entries: live
    vectors plus garbage awaiting compaction. They stay within the server's
    flush factor times ``vectors`` unless the index leaks.

    ``relevance`` is the relevance model the graph's text index ranks with,
    ``bm25`` or ``matchcount``. It is read back from the index, not echoed from
    configuration. It is ``None`` from a server too old to report it.

    A graph nothing was written to is a row of zeros, not a missing row.
    """

    id: int
    vertices: int
    edges: int
    nodes: int
    vectors: int
    forest_entries: int
    relevance: str | None = None

    @classmethod
    def from_json(cls, data: dict) -> GraphStats:  # noqa: D102
        return cls(
            id=int(data["id"]),
            vertices=int(data["order"]),
            edges=int(data["size"]),
            nodes=int(data["nodes"]),
            vectors=int(data["vectors"]),
            forest_entries=int(data["forest_entries"]),
            relevance=data.get("relevance"),
        )


@dataclass(frozen=True)
class Contribution:
    """One retrieval source's sighting of a hit, as ``explain`` reports it.

    ``source`` names the stage that saw the fact: ``text``, ``vector``,
    ``graph`` or ``anchor``. ``score`` is that stage's raw mass, not a share of
    the hit's score. The hit's score is the final value, after transmission and
    recency decay. So its contributions are its ingredients and do not sum to
    it. ``rank`` is the hit's place in that source's own list, 0 first.
    ``count`` is how many seeds funded the sighting. A ``graph`` or ``anchor``
    sighting also names the anchor it came through (``via``) and that anchor's
    ``degree``. A ``text`` or ``vector`` sighting has neither. ``named`` marks
    a ``graph`` sighting through an anchor the recall named, which owes no
    fair share to the background.
    """

    source: str
    score: float
    rank: int
    count: int
    via: str | None = None
    degree: int | None = None
    named: bool = False

    @classmethod
    def from_json(cls, data: dict) -> Contribution:  # noqa: D102
        return cls(
            source=data["source"],
            score=float(data["score"]),
            rank=int(data["rank"]),
            count=int(data["count"]),
            via=data.get("via"),
            degree=data.get("degree"),
            named=bool(data.get("named", False)),
        )


@dataclass(frozen=True)
class Hit:
    """One recalled fact and how strongly it matched the query.

    ``contributions`` is the per-source breakdown of the score.
    :meth:`~fraise_sdk.client.FraiseClient.explain` fills it. It is empty on a
    plain recall, which does not ask the server for it.
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

    ``warnings`` holds any warnings the server attached. The query ran and the
    hits are valid, but something in it cannot help or may not be what was
    meant. An example is a stop-word term that can never match. It is empty for
    a clean query.

    ``empty`` is about the graph, not the result set. ``bool(result)`` reports
    whether anything came back. ``empty`` tells apart the two ways a recall
    comes back with nothing. False is the ordinary miss. The graph holds facts
    and none of them matched, so the query is what to change. True means the
    graph searched holds nothing at all, so no rephrasing would have helped.
    The server carries the difference in the status line, with 204 for the
    empty graph.

    ``background`` is the query's background rate: the seed mass per unit of
    anchor degree the search observed. With each hit's contributions, it is
    every input of the scoring fold. So a caller can see why a fact ranked
    where it did. ``explain`` sets it, and it is ``None`` on a plain recall.
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
            results: the ``results`` member of the response body. It is empty
                for a 204, which has no body to carry one.
            warnings: the response's ``warnings`` list. It sits beside
                ``results``, not inside it, so the caller holding the whole
                body passes it here. ``None`` (a clean response, or a
                pre-warnings server) parses to an empty list.
            empty: whether the server answered 204, meaning the graph searched
                holds nothing. The status line carries it, not the body, so the
                caller that saw the response passes it in.
            explain: whether the response came from the explain route. The
                server omits a background rate of zero. So a missing rate reads
                as 0.0 on an explained result, and as ``None`` on a plain one.

        Returns:
            The typed result, warnings included.
        """
        hits = [Hit.from_json(h) for h in results.get("hits") or []]
        # Prefer the server-reported count. Fall back to the hit count, so the
        # two agree even when the field is omitted.
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
