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

These functions do no I/O: the wire format lives here, and
:mod:`fraise_sdk.client` only concerns itself with transport. The grammar they
target:

    remember@<graph> '<value>' [topic:<t>]... [entity:<e>]... [vec:$<name>]
    recall@<graph> ['<query>'] [<keyword>]... [topic:<t>]... [entity:<e>]...
                   [top:<n>] [depth:<n>] [vec:$<name>]

A topic, entity or keyword is written bare when it is one plain word — letters
and digits only — and quoted otherwise; a keyword that spells a reserved word
is quoted too, since a bare one is syntax, and warns. So the builders take any
value. A query they do not cover — a ``since:``/``until:`` bound, say — is
written as FQL and sent with :meth:`fraise_sdk.client.FraiseClient.query`.

The vector itself never appears in the string — the caller sends it out of band
in the request ``parameters`` map, and only the ``vec:$<name>`` placeholder is
emitted here.
"""

from __future__ import annotations

from collections.abc import Iterable, Sequence

from fraise_sdk.compat import warn
from fraise_sdk.constants import KEYWORDS, MAX_GRAPH, SDK_FILES, VECTOR_PARAM
from fraise_sdk.errors import FraiseQueryError, FraiseWarning


def _token(kind: str, value: str) -> str:
    """Validate and return a keyword, topic or entity value, trimmed.

    Whitespace inside the value is data, not a reason to refuse it: a value
    that is not one plain word is quoted on the way out (see
    :func:`_bare_or_quoted`), so ``my project`` travels whole as
    ``'my project'``. Only an empty value has no encoding the server accepts.

    Raises:
        FraiseQueryError: if the value is empty or only whitespace.
    """
    value = value.strip()
    if not value:
        raise FraiseQueryError(f"{kind} must not be empty")
    return value


def _sequence(kind: str, values: Iterable[str] | None) -> Iterable[str]:
    """Return ``values``, or an empty list for ``None``, refusing a bare string.

    ``str`` satisfies ``Sequence[str]``, so ``topics="billing"`` passes the
    type hint and would iterate into one token per character: the write
    succeeds, the fact is filed under ``b``, ``i``, ``l``... and a recall by the
    anchor the caller named finds nothing. The caller meant one value, so the
    string is refused by name.

    Raises:
        FraiseQueryError: if ``values`` is a string rather than a sequence.
    """
    if isinstance(values, str):
        raise FraiseQueryError(
            f"{kind} must be a sequence of strings, not a string: {values!r}; "
            f"did you mean [{values!r}]?"
        )
    return values or []


def _clauses(prefix: str, values: Iterable[str] | None) -> list[str]:
    """Render ``topic:``/``entity:`` clauses, quoting a value only when it must.

    A reserved word needs no quoting here: after the ``:`` only a value can
    appear, so ``topic:top`` is the anchor ``top``. A space, a colon or an
    apostrophe does, and the quoted form carries it whole —
    ``topic:'us elections'``, ``entity:'o''brien'`` — while the server folds
    it to the same anchor a bare spelling would name.
    """
    return [
        f"{prefix}:{_bare_or_quoted(_token(prefix, v))}"
        for v in _sequence(prefix, values)
    ]


def _bare_or_quoted(token: str) -> str:
    """Return ``token`` bare when the grammar reads it as one word, else quoted.

    Outside quotes a word is letters and digits only: the server rejects any
    other character, such as the ``-`` of ``machine-learning``, and a space
    ends the word, so bare ``new york`` would be two. The caller passed one
    value, so the builder writes the form that carries it whole.
    ``str.isalpha`` and ``str.isdecimal`` are the server's letter and digit
    classes, so letters in any script stay bare.
    """
    if all(ch.isalpha() or ch.isdecimal() for ch in token):
        return token
    return _quote_value(token)


def _quote_value(value: str) -> str:
    """Wrap a value in the grammar's quotes.

    Inside a quoted phrase every character is literal and an apostrophe is
    escaped by doubling it (``''``), so any text travels verbatim:
    ``it's blue`` goes over the wire as ``'it''s blue'`` and comes back with
    its apostrophe intact.

    Raises:
        FraiseQueryError: if the value is empty or only whitespace.
    """
    if not value.strip():
        raise FraiseQueryError("a quoted value must not be empty")
    escaped = value.replace("'", "''")
    return f"'{escaped}'"


def _term(value: str) -> str:
    """Render a recall search word so the server reads it as that word.

    A term follows the anchor rule (see :func:`_bare_or_quoted`) plus one more:
    a reserved word is syntax wherever a term stands, in any casing, and the
    server rejects ``top`` or ``Since`` as a bare term rather than guess
    whether a clause was meant. The builder quotes it, so the call runs as the
    search the caller wrote, and warns: ``recall("since", "7d")`` may as well
    have meant a ``since:7d`` bound, and only the caller can say which.

    Warns:
        FraiseWarning: when the keyword spells a reserved word.
    """
    token = _token("keyword", value)
    if token.lower() in KEYWORDS:
        warn(
            f'keyword "{token}" is a reserved word, so it was searched as the '
            f"word '{token}'; to use it as FQL syntax, write the query with "
            "client.query",
            FraiseWarning,
            skip_file_prefixes=SDK_FILES,
        )
        return _quote_value(token)
    return _bare_or_quoted(token)


def _selector(graph: int) -> str:
    if not isinstance(graph, int) or isinstance(graph, bool):
        raise FraiseQueryError(f"graph must be an int, got {type(graph).__name__}")
    if graph < 0:
        raise FraiseQueryError(f"graph must be non-negative, got {graph}")
    if graph > MAX_GRAPH:
        raise FraiseQueryError(f"graph must be at most {MAX_GRAPH}, got {graph}")
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

    ``query`` is a whole question sent as one quoted phrase term. The grammar
    keeps every character inside the quotes literal, so natural language
    ("What topic has John been blogging about recently?") travels verbatim
    instead of as bare words that would collide with the grammar's reserved
    keywords. ``keywords`` accompany it as individual terms, each quoted when a
    bare one would not read back as that word (see :func:`_term`).

    A recall needs at least one seed: a query phrase, keywords, a vector, or a
    topic or entity anchor. A call given nothing but ``graph`` is rejected
    here; one given only ``top`` or ``depth`` is built as written, and the
    server rejects it for the missing seed.

    Raises:
        FraiseQueryError: if nothing but ``graph`` is given, ``top`` is not
            positive or ``depth`` is negative, or a value cannot be written as
            FQL (an empty value, a bare string where a sequence is wanted, a
            graph outside 0–255).
    """
    parts = [f"recall{_selector(graph)}"]
    if query is not None:
        parts.append(_quote_value(query))
    for keyword in _sequence("keyword", keywords):
        parts.append(_term(keyword))
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
