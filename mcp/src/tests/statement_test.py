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
"""What the reader encodes from a statement, and when it leaves one alone."""

import pytest
from fraise_mcp.statement import embedding_text, with_vector


@pytest.mark.parametrize(
    "statement, want",
    [
        ("remember 'the barometer falls' topic:weather", "the barometer falls"),
        ("remember@3 'it''s raining' entity:harbour", "it's raining"),
        ("recall barometer storm topic:weather top:5 depth:1", "barometer storm"),
        ("recall@2 'favourite colour' color since:7d", "favourite colour color"),
        ("recall x topic:top", "x"),
        ("recall Kraków 2026", "Kraków 2026"),
        ("explain recall barometer", "barometer"),
        ("describe recall barometer topic:weather", "barometer"),
        ("recall  barometer\t", "barometer"),
    ],
)
def test_the_fact_or_the_terms_are_encoded(statement, want):
    """A remember encodes its fact, a recall the terms before its first clause.

    A clause value is never a term, even one that spells a keyword, and a
    phrase is encoded unquoted, so the vector is of the words the model meant.
    """
    assert embedding_text(statement) == want


@pytest.mark.parametrize(
    "statement",
    [
        "recall topic:weather",
        "recall@2 topic:weather entity:harbour top:20",
        "describe recall",
        "recall barometer vec:$q",
        "recall barometer VEC:$q",
        "remember 'the barometer falls' vec:$q",
        "recall barometer top",
        "recall Top barometer",
        "recall barometer topic:",
        "recall e-mail",
        "recall 'unterminated",
        "recall barometer\nrecall storm",
        "RECALL barometer",
        "explain remember 'the barometer falls'",
        "remember barometer",
        "forget 'the barometer falls'",
        "explain",
        "",
    ],
)
def test_a_statement_the_reader_cannot_follow_takes_no_vector(statement):
    """Anything but a readable remember or a recall with terms is sent as written.

    An anchor-only recall would become a scored search with a vector beside
    it. A statement binding a vector already would bind a second. The rest are
    errors, and the server's message must be about what the model wrote.
    """
    assert embedding_text(statement) is None


def test_the_vector_clause_goes_last():
    """The clause is appended after trailing blanks, so every token keeps its place."""
    assert with_vector("recall barometer topic:weather \t") == (
        "recall barometer topic:weather vec:$v"
    )
