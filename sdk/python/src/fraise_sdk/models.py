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
    """

    count: int
    hits: list[Hit]
    warnings: list[str] = field(default_factory=list)

    @classmethod
    def from_json(
        cls, results: dict, warnings: Sequence[str] | None = None
    ) -> RecallResult:
        """Parse the server's ``results`` object, with any response warnings.

        Args:
            results: the ``results`` member of the response body.
            warnings: the response's ``warnings`` list, which sits beside
                ``results`` rather than inside it — the caller holding the
                whole body passes it through here. ``None`` (a clean response,
                or a pre-warnings server) parses to an empty list.

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
        )

    def __bool__(self) -> bool:
        return bool(self.hits)

    def __iter__(self) -> Iterable:
        return iter(self.hits)

    def __len__(self) -> int:
        return len(self.hits)
