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

"""Builders that turn structured arguments into Fraise query strings.

These are pure functions with no I/O, so they are the natural unit-test seam:
the wire format lives here, and :mod:`fraise_sdk.client` only concerns itself
with transport. The grammar they target:

    remember@<graph> '<value>' [topic:<t>]... [entity:<e>]... [vec:$<name>]
    recall@<graph> <keyword>... [topic:<t>]... [entity:<e>]...
                   [top:<n>] [depth:<n>] [vec:$<name>]

The vector itself never appears in the string — the caller sends it out of band
in the request ``parameters`` map, and only the ``vec:$<name>`` placeholder is
emitted here.
"""

from __future__ import annotations

from collections.abc import Iterable, Sequence

from fraise_sdk.errors import FraiseQueryError

# Name bound to the out-of-band vector in the request parameters. A query carries
# at most one vector, so a single fixed placeholder keeps client and builder in
# lock-step without threading a name through every call.
VECTOR_PARAM = "v"


def _token(kind: str, value: str) -> str:
    """Validate and return a bare grammar token (a keyword, topic or entity).

    The grammar splits on whitespace, so a token may not contain any — a stray
    space would be parsed as two tokens (or fail), silently changing the query.

    Raises:
        FraiseQueryError: if query is not valid
    """
    value = value.strip()
    if not value:
        raise FraiseQueryError(f"{kind} must not be empty")
    if any(ch.isspace() for ch in value):
        raise FraiseQueryError(f"{kind} must not contain whitespace: {value!r}")
    return value


def _clauses(prefix: str, values: Iterable[str] | None) -> list[str]:
    if not values:
        return []
    return [f"{prefix}:{_token(prefix, v)}" for v in values]


def _quote_value(value: str) -> str:
    """Wrap a value (a fact, or a recall's query phrase) in the grammar's quotes.

    Inside a quoted phrase every character is literal and an apostrophe is
    escaped by doubling it (``''``), so any text travels verbatim —
    ``it's blue`` goes over the wire as ``'it''s blue'`` and comes back with
    its apostrophe intact.

    Raises:
        FraiseQueryError: if query is not valid
    """
    if not value.strip():
        raise FraiseQueryError("a quoted value must not be empty")
    escaped = value.replace("'", "''")
    return f"'{escaped}'"


def _selector(graph: int) -> str:
    if not isinstance(graph, int) or isinstance(graph, bool):
        raise FraiseQueryError(f"graph must be an int, got {type(graph).__name__}")
    if graph < 0:
        raise FraiseQueryError(f"graph must be non-negative, got {graph}")
    return f"@{graph}"


def build_remember(
    value: str,
    *,
    graph: int = 0,
    topics: Sequence[str] | None = None,
    entities: Sequence[str] | None = None,
    with_vector: bool = False,
) -> str:
    """Build a ``remember`` query string that stores ``value`` in ``graph``.

    Set ``with_vector`` when a vector is being sent in the request parameters, so
    the ``vec:$v`` placeholder is appended for the server to bind.
    """
    parts = [f"remember{_selector(graph)}", _quote_value(value)]
    parts += _clauses("topic", topics)
    parts += _clauses("entity", entities)
    if with_vector:
        parts.append(f"vec:${VECTOR_PARAM}")
    return " ".join(parts)


def build_recall(
    keywords: Sequence[str] | None = None,
    *,
    graph: int = 0,
    query: str | None = None,
    topics: Sequence[str] | None = None,
    entities: Sequence[str] | None = None,
    top: int | None = None,
    depth: int | None = None,
    with_vector: bool = False,
) -> str:
    """Build a ``recall`` query string over ``graph``.

    ``query`` is a whole question sent as one quoted phrase term — the grammar
    keeps every character inside the quotes literal, so natural language
    ("What topic has John been blogging about recently?") travels verbatim
    instead of as bare words that would collide with the grammar's reserved
    keywords. ``keywords`` remain individual bare terms and may accompany it.

    A recall needs at least one seed: a query phrase, keywords, a vector, or a
    topic/entity filter. Building one with no seed at all is a programming
    error and is rejected here.

    Raises:
        FraiseQueryError: if query is not valid
    """
    parts = [f"recall{_selector(graph)}"]
    if query is not None:
        parts.append(_quote_value(query))
    parts += [_token("keyword", k) for k in (keywords or [])]
    parts += _clauses("topic", topics)
    parts += _clauses("entity", entities)
    if top is not None:
        if top <= 0:
            raise FraiseQueryError(f"top must be positive, got {top}")
        parts.append(f"top:{top}")
    if depth is not None:
        if depth < 0:
            raise FraiseQueryError(f"depth must be non-negative, got {depth}")
        parts.append(f"depth:{depth}")
    if with_vector:
        parts.append(f"vec:${VECTOR_PARAM}")

    if len(parts) == 1:
        raise FraiseQueryError(
            "recall needs at least one seed: keywords, a vector, or a "
            "topic/entity filter"
        )
    return " ".join(parts)
