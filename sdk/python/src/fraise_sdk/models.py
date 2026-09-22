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
class Hit:
    """One recalled fact and how strongly it matched the query."""

    value: str
    score: float
    timestamp: str | None = None

    @classmethod
    def from_json(cls, data: dict) -> Hit:  # noqa: D102
        return cls(
            value=data["value"],
            score=float(data["score"]),
            timestamp=data.get("timestamp"),
        )


@dataclass(frozen=True)
class RecallResult:
    """The result set of a ``recall``: how many facts matched and, in ranked order, what they were.

    ``warnings`` carries any parse warnings the server attached: the query ran
    and the hits are valid, but it was one typo away from meaning something
    else (e.g. a leading term that spells a grammar keyword). Empty on the
    common, unambiguous path.

    ``empty`` is about the graph, not the result set — ``bool(result)`` is what
    reports whether anything came back. It separates the two ways a recall comes
    back with nothing: False is the ordinary miss, where the graph holds facts
    and none of them matched, so the query is what to change; True means the
    graph searched holds nothing at all — no fact has ever been written to it —
    and no rephrasing would have helped. The server carries the difference in
    the status line (204 for the empty graph), because the two were otherwise
    the same empty result set, and a caller could not tell a graph it had never
    written to from a question it had asked badly.
    """

    count: int
    hits: list[Hit]
    warnings: list[str] = field(default_factory=list)
    empty: bool = False

    @classmethod
    def from_json(
        cls,
        results: dict,
        warnings: Sequence[str] | None = None,
        *,
        empty: bool = False,
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
        )

    def __bool__(self) -> bool:
        return bool(self.hits)

    def __iter__(self) -> Iterable:
        return iter(self.hits)

    def __len__(self) -> int:
        return len(self.hits)
