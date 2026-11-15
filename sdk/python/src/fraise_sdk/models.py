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
from datetime import datetime


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
    """One retrieval source's sighting of a hit, as an explained recall reports it.

    ``source`` names the stage that saw the fact: ``text``, ``vector``,
    ``graph`` or ``anchor``. ``score`` is that stage's raw mass, not a share of
    the hit's score. The hit's score is the final value, after transmission and
    recency decay. So its contributions are its ingredients and do not sum to
    it. A ``vector`` score is the similarity as measured. ``rank`` is the
    hit's place in that source's own list, 0 first.
    ``count`` is how many seeds funded the sighting. A ``graph`` or ``anchor``
    sighting also names the anchor it came through (``via``) and that anchor's
    ``degree``. A ``text`` or ``vector`` sighting has neither. ``named`` marks
    a ``graph`` sighting through an anchor the recall named, which owes no
    fair share to the background. ``held`` is, on a ``text`` sighting, the
    share of the query's information the fact's own words hold, 0 when it was
    matched only through its neighbours.
    """

    source: str
    score: float
    rank: int
    count: int
    via: str | None = None
    degree: int | None = None
    named: bool = False
    held: float = 0.0

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
            held=float(data.get("held", 0.0)),
        )


@dataclass(frozen=True)
class Interval:
    """A span of time, which holds its start and excludes its end.

    ``end`` equal to ``start`` is a single moment. ``end`` of ``None`` is a
    span that has not ended. Both edges are datetimes that carry their time
    zone, as the server returns them.

    It is the shape of each span in a hit's ``occurred``, and what
    :meth:`~fraise_sdk.client.FraiseClient.remember` takes in ``occurred`` for
    a span between two moments. A whole month is the span from its first
    midnight to the next month's, and a recall tool prints it back as that
    month.
    """

    start: datetime
    end: datetime | None

    @classmethod
    def from_json(cls, data: dict) -> Interval:  # noqa: D102
        end = data.get("end")
        return cls(
            start=datetime.fromisoformat(data["start"]),
            end=datetime.fromisoformat(end) if end else None,
        )


@dataclass(frozen=True)
class HitExplain:
    """What an explained recall reports about one hit.

    ``seed`` says whether the hit was one of the recall's seeds: among the
    first ``max(db.seed-size, top)`` facts of a text or vector reading, or a
    member of a named anchor when the anchors seed alone. Only a seed funds an
    anchor. So a hit that is no seed and carries graph contributions alone was
    admitted by transmission, and one that is no seed with a text or vector
    contribution matched below the seed cut. The server sends it false too, so
    a missing key never stands for either.

    ``decay`` is the recency factor the hit's relevance was scaled by to give
    its score. It tells a fact that ranks low for its age from one that ranks
    low for its match. A returned hit's factor is never zero, so the server
    always sends it.

    ``contributions`` is the hit's per-source breakdown, the sightings its
    relevance was folded from, in the order the server sent them. A recalled
    fact was seen by at least one source, so it is never empty.
    """

    seed: bool
    decay: float
    contributions: list[Contribution]

    @classmethod
    def from_json(cls, data: dict) -> HitExplain:  # noqa: D102
        return cls(
            seed=bool(data["seed"]),
            decay=float(data["decay"]),
            contributions=[Contribution.from_json(c) for c in data["contributions"]],
        )


@dataclass(frozen=True)
class Hit:
    """One recalled fact and how strongly it matched the query.

    A fact holds three times. ``timestamp`` is when the server wrote it, and
    every hit has it. ``said`` is when the fact was said, and ``occurred`` each
    span over which what it states happened. Those two are the times its
    remember gave it, and each is ``None`` on a fact given none, or from a
    server too old to hold them. A fact given no ``occurred`` happened when it
    was said, and one given no ``said`` was said when it was written.

    ``explain`` is the hit's half of an explained recall (see
    :meth:`~fraise_sdk.client.FraiseClient.explain`): whether it seeded the
    recall, the recency factor its relevance was decayed by, and the
    contributions that relevance was folded from. It is ``None`` on a plain
    recall, whose hits carry no ``explain`` key.
    """

    value: str
    score: float
    timestamp: str | None = None
    said: datetime | None = None
    occurred: list[Interval] | None = None
    explain: HitExplain | None = None

    @classmethod
    def from_json(cls, data: dict) -> Hit:  # noqa: D102
        said = data.get("said")
        occurred = data.get("occurred")
        explain = data.get("explain")
        return cls(
            value=data["value"],
            score=float(data["score"]),
            timestamp=data.get("timestamp"),
            said=datetime.fromisoformat(said) if said else None,
            occurred=[Interval.from_json(s) for s in occurred] if occurred else None,
            explain=HitExplain.from_json(explain) if explain is not None else None,
        )


@dataclass(frozen=True)
class RecallExplain:
    """What an explained recall reports about its query, beside the ranking.

    Each value is the search's own measurement, never a figure rebuilt from
    the hits. A caller reads them to see why a fact ranked where it did, or
    why a word matched nothing.

    ``background`` is the query's background rate: the seed mass the anchor
    round observed per unit of degree, over every anchor it touched. An anchor
    the recall did not name owes its degree times the rate as its fair share
    before any of its mass transmits. A named anchor owes none. With each
    hit's contributions it is what the graph evidence is folded from, so a
    caller sees which anchor lifted a fact and by how much. It is 0.0 when no
    round ran.

    ``admission`` maps a depth to the bar, per unit of anchor degree, that the
    mass of an anchor the recall did not name had to exceed to transmit at
    that depth: depth 1 bars at twice the background rate, for precision, and
    depth 2 at the rate itself, for recall. One round measures both, so it
    shows what the same recall admits one lane over. An anchor transmits at
    depth ``d`` when a graph contribution's ``score`` exceeds its ``degree``
    times ``admission[d]``. A named anchor transmits at either depth. It is
    empty when no round ran: when the recall runs at depth 0, names no anchor,
    seeds nothing or is seeded by its named anchors alone, or when the
    server's ``db.search-algorithm`` is ``none``. A recall naming an anchor
    beside a term or vector runs at depth 1 at least, so a ``depth:0`` written
    there still reports both bars. The server omits the key when no round
    ran, and its absence reads as empty.

    ``seeds`` counts, per source, the distinct facts that source seeded. A
    ``text`` or ``vector`` seed is among the first ``max(db.seed-size, top)``
    facts of one of that source's readings, and an ``anchor`` seed is a member
    of a named anchor when the anchors seed alone. So it is not the number of
    matches. ``graph`` is always 0, since the anchor round admits facts rather
    than seeding them.

    ``terms`` lists the query's distinct terms as the text index read them,
    after stop-word cleaning and stemming, in first-occurrence order. A term
    no stored fact holds is listed too, which is how a caller sees why a word
    matched nothing. It is empty when the recall has no terms, or every
    keyword is a stop word.

    ``timings`` holds, per source, the wall time it took, in integer
    nanoseconds: ``text`` the text reading, ``vector`` the vector reading,
    ``anchor`` the named anchors' seeding and ``graph`` the anchor round. A
    source that did not run took 0.

    ``seeds`` and ``timings`` are keyed by the names a contribution's
    ``source`` carries, ``text``, ``vector``, ``anchor`` and ``graph``, and
    list every source whatever the recall asked for.

    ``when`` holds the result of each query nested in the recall's time
    bounds, a :class:`~fraise_sdk.query.When` given as ``since`` or
    ``until``, keyed by the name of the bound that holds it. It shows which
    fact a bound was read from and the time that fact gave, each result's
    ``start`` and ``end``, so a caller tells a window drawn from the wrong
    fact from a ranking that missed inside the right one. A nested query is
    explained with the recall, so each result carries its own ``explain``
    blocks. What a when warned about is listed on the recall's ``warnings``,
    under the bound's name, so a nested result's own ``warnings`` is empty. A
    bound written as a time has no entry. The server omits the key when
    neither bound holds a when, and its absence reads as empty.
    """

    background: float
    admission: dict[int, float]
    seeds: dict[str, int]
    terms: list[str]
    timings: dict[str, int]
    when: dict[str, RecallResult]

    @classmethod
    def from_json(cls, data: dict) -> RecallExplain:  # noqa: D102
        return cls(
            background=float(data["background"]),
            # JSON object keys are strings, so the depths are read back as ints.
            admission={
                int(depth): float(bar)
                for depth, bar in (data.get("admission") or {}).items()
            },
            seeds={source: int(n) for source, n in data["seeds"].items()},
            terms=list(data["terms"]),
            timings={source: int(ns) for source, ns in data["timings"].items()},
            when={
                bound: RecallResult.from_json(result)
                for bound, result in (data.get("when") or {}).items()
            },
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

    ``explain`` is the query's half of an explained recall: what the search
    measured and decided about the query as a whole (see
    :class:`RecallExplain`). Each hit carries its own half. It is ``None`` on a
    plain recall, and on a recall of an empty graph, whose 204 carries no body.

    ``start`` and ``end`` are the time a ``when`` answers with, a span that
    holds ``start`` and excludes ``end``: the hull of what its hits' facts hold
    on the clock it read. A bound holding the when is cut at that time, so the
    caller reads it here rather than rebuilding it from the hits, whose facts
    may fall back to another clock. Both are ``None`` on a recall, which
    answers with no time, and on a when that matched no fact. ``end`` alone is
    ``None`` for an answer that has not ended.
    """

    count: int
    hits: list[Hit]
    warnings: list[str] = field(default_factory=list)
    empty: bool = False
    explain: RecallExplain | None = None
    start: datetime | None = None
    end: datetime | None = None

    @classmethod
    def from_json(
        cls,
        results: dict,
        warnings: Sequence[str] | None = None,
        *,
        empty: bool = False,
    ) -> RecallResult:
        """Parse the server's ``results`` object, with any response warnings.

        The ``explain`` blocks are read wherever the body carries them. The
        server sends them on an explained recall only, so their presence is
        what marks one. A body ``FraiseClient.query`` returned for an
        ``explain recall`` therefore parses as ``FraiseClient.explain``'s does.

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

        Returns:
            The typed result, warnings included.
        """
        hits = [Hit.from_json(h) for h in results.get("hits") or []]
        explain = results.get("explain")
        start = results.get("start")
        end = results.get("end")
        # Prefer the server-reported count. Fall back to the hit count, so the
        # two agree even when the field is omitted.
        return cls(
            count=results.get("count", len(hits)),
            hits=hits,
            warnings=list(warnings or []),
            empty=empty,
            explain=RecallExplain.from_json(explain) if explain is not None else None,
            start=datetime.fromisoformat(start) if start else None,
            end=datetime.fromisoformat(end) if end else None,
        )

    def __bool__(self) -> bool:
        return bool(self.hits)

    def __iter__(self) -> Iterable:
        return iter(self.hits)

    def __len__(self) -> int:
        return len(self.hits)
