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
"""The text a statement's vector is encoded from, read off the statement itself.

The model writes plain FQL and never a ``vec:`` clause. The server takes a
vector only through one, bound out of band. So the server reads the statement
as written, and the bridge reads just enough of it to know what to encode: a
``remember``'s fact phrase, or a ``recall``'s terms. The grammar puts both
first, straight after the command. A recall's terms run until the first clause,
the first word glued to a ``:``. So the reader never needs the clause
vocabulary, and it never checks the statement. That stays the server's job.

Only statements the reader can follow take a vector. For anything else it
returns ``None``, and the statement is sent exactly as written. So whatever
the server says about it is about what the model wrote.
"""

from __future__ import annotations

from collections.abc import Iterator
from typing import Literal

from fraise_sdk.constants import KEYWORDS, VECTOR_PARAM

# The clause that binds the vector, appended to a statement that takes one.
VECTOR_CLAUSE = f"vec:${VECTOR_PARAM}"

# The prefixes the grammar allows in front of a recall. The recall behind one
# reads the same, so it takes a vector the same way.
_PREFIXES = frozenset({"explain", "describe"})

Kind = Literal["word", "phrase", "clause", "symbol"]


def _tokens(statement: str) -> Iterator[tuple[Kind, str]]:
    """Split ``statement`` into words, phrases, clause keys and symbols.

    A word glued to a ``:`` is a clause key, and the ``:`` goes with it. A
    phrase is unquoted, its doubled quotes read as one. Blanks separate
    tokens and are dropped.

    Yields:
        Each token's kind and text, in order.

    Raises:
        ValueError: at a newline, which starts a second instruction, or at a
            phrase with no closing quote.
    """
    i, n = 0, len(statement)
    while i < n:
        ch = statement[i]
        if ch in " \t\r":
            i += 1
        elif ch == "\n":
            raise ValueError("more than one instruction")
        elif ch == "'":
            parts: list[str] = []
            i += 1
            while True:
                end = statement.find("'", i)
                if end < 0:
                    raise ValueError("unterminated phrase")
                parts.append(statement[i:end])
                if statement.startswith("''", end):
                    parts.append("'")
                    i = end + 2
                    continue
                i = end + 1
                break
            yield "phrase", "".join(parts)
        elif ch.isalpha() or ch.isdecimal():
            start = i
            while i < n and (statement[i].isalpha() or statement[i].isdecimal()):
                i += 1
            if i < n and statement[i] == ":":
                i += 1
                yield "clause", statement[start : i - 1].lower()
            else:
                yield "word", statement[start:i]
        else:
            i += 1
            yield "symbol", ch


def embedding_text(statement: str) -> str | None:
    """Return the text ``statement``'s vector is encoded from, or ``None``.

    A ``remember`` encodes its fact phrase. A ``recall``, behind an
    ``explain`` or ``describe`` prefix or not, encodes its terms, joined by a
    space, with each phrase unquoted.

    It returns ``None``, and so the statement takes no vector, when:

    - the statement is any other command, or none;
    - a recall has no terms. Anchors alone seed the search with every fact
      filed under them, and a vector beside them would turn that into a
      scored search;
    - the statement binds a vector already;
    - a term is a bare reserved word or a symbol, or the statement ends on a
      bare reserved word or on a clause key with no value. Each is an error.
      A vector would be wasted on it, and a clause appended where the server
      reports it would change what the server says;
    - the statement holds more than one instruction, or a phrase with no
      closing quote.

    Args:
        statement: one FQL statement, as the model wrote it.

    Returns:
        The text to encode, or ``None`` when the statement takes no vector.
    """
    try:
        tokens = list(_tokens(statement))
    except ValueError:
        return None
    if not tokens or ("clause", "vec") in tokens:
        return None
    kind, last = tokens[-1]
    after_clause = len(tokens) > 1 and tokens[-2][0] == "clause"
    if kind == "clause" or (
        kind == "word" and last.lower() in KEYWORDS and not after_clause
    ):
        return None

    prefixed = tokens[0][0] == "word" and tokens[0][1] in _PREFIXES
    position = int(prefixed)
    command = tokens[position] if position < len(tokens) else None
    position += 1
    # A graph selector, @N, is glued to the command. The server checks the
    # glue, and the reader only steps over it.
    if tokens[position : position + 1] == [("symbol", "@")]:
        position += 2
    rest = tokens[position:]

    if command == ("word", "remember") and not prefixed:
        return rest[0][1] if rest and rest[0][0] == "phrase" else None
    if command != ("word", "recall"):
        return None
    terms: list[str] = []
    for kind, value in rest:
        if kind == "clause":
            break
        if kind == "symbol" or (kind == "word" and value.lower() in KEYWORDS):
            return None
        terms.append(value)
    return " ".join(terms) or None


def with_vector(statement: str) -> str:
    """Return ``statement`` with the vector clause appended.

    A clause goes last, so every token the model wrote keeps its position, and
    a recall's terms still come first.
    """
    return f"{statement.rstrip()} {VECTOR_CLAUSE}"
