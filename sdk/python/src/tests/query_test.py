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

"""Unit tests for the pure query-string builders."""

import pytest
from fraise_sdk.errors import FraiseQueryError
from fraise_sdk.query import VECTOR_PARAM, build_recall, build_remember


def test_remember_minimal():
    assert (
        build_remember("the parrot is turquoise")
        == "remember@0 'the parrot is turquoise'"
    )


def test_remember_with_graph_topics_and_entities():
    got = build_remember(
        "anne loves the color orange",
        graph=3,
        topics=["color"],
        entities=["anne"],
    )
    assert got == "remember@3 'anne loves the color orange' topic:color entity:anne"


def test_remember_with_vector_appends_placeholder():
    got = build_remember("the parrot is turquoise", graph=6, with_vector=True)
    assert got == f"remember@6 'the parrot is turquoise' vec:${VECTOR_PARAM}"


def test_remember_escapes_apostrophes():
    """An apostrophe is doubled — the grammar's phrase escape — not rejected."""
    assert build_remember("it's turquoise") == "remember@0 'it''s turquoise'"


def test_remember_rejects_empty_value():
    with pytest.raises(FraiseQueryError):
        build_remember("   ")


def test_remember_keeps_free_text_verbatim_inside_the_quotes():
    """Ingestion feeds phrases arbitrary prose: newlines, tabs, emoji and
    backslashes travel inside the quotes untouched — only apostrophes are
    rewritten, by doubling.
    """
    value = 'line one\nline two\t— déjà vu 😀 C:\\temp "quoted"'
    assert build_remember(value) == f"remember@0 '{value}'"


def test_recall_query_phrase_is_one_quoted_term():
    """A whole question travels as a single quoted phrase term, so natural
    language never collides with the grammar's reserved keywords.
    """
    got = build_recall(
        query="What topic has John been blogging about recently",
        top=10,
        with_vector=True,
    )
    assert got == (
        "recall@0 'What topic has John been blogging about recently' "
        f"top:10 vec:${VECTOR_PARAM}"
    )


def test_recall_query_phrase_escapes_apostrophes():
    """The phrase escape covers the query too: John's travels as John''s."""
    assert (
        build_recall(query="what is John's blog about")
        == "recall@0 'what is John''s blog about'"
    )


def test_recall_with_keywords_and_clauses():
    got = build_recall(["anna", "bob"], graph=2, top=10, depth=2)
    assert got == "recall@2 anna bob top:10 depth:2"


def test_recall_with_vector_only():
    assert build_recall(graph=6, with_vector=True) == f"recall@6 vec:${VECTOR_PARAM}"


def test_recall_topic_seed_is_enough():
    assert build_recall(topics=["birds"]) == "recall@0 topic:birds"


def test_recall_requires_a_seed():
    with pytest.raises(FraiseQueryError, match="at least one seed"):
        build_recall(graph=1)


def test_recall_rejects_whitespace_in_keyword():
    with pytest.raises(FraiseQueryError, match="whitespace"):
        build_recall(["two words"])


@pytest.mark.parametrize("bad", [0, -1])
def test_recall_rejects_non_positive_top(bad):
    """top must be positive — a recall that can return nothing is a caller bug."""
    with pytest.raises(FraiseQueryError):
        build_recall(["x"], top=bad)


@pytest.mark.parametrize("bad", [-1, -2])
def test_recall_rejects_negative_depth(bad):
    """depth must be non-negative; a negative walk length is meaningless."""
    with pytest.raises(FraiseQueryError):
        build_recall(["x"], depth=bad)


def test_recall_emits_depth_zero():
    """depth:0 is the explicit floor lane (text/vector only) and travels verbatim."""
    assert build_recall(["x"], depth=0) == "recall@0 x depth:0"


def test_negative_graph_is_rejected():
    with pytest.raises(FraiseQueryError, match="non-negative"):
        build_remember("x", graph=-1)


@pytest.mark.parametrize("bad", [256, 300, 99999])
def test_graph_above_the_uint8_range_is_rejected(bad):
    """A selector travels as a uint8, so 256 is not "too big" — it is graph 0.

    Refusing it here means a mistyped graph id fails in the caller's own stack
    trace rather than reading or writing somebody else's memory.
    """
    with pytest.raises(FraiseQueryError, match="at most 255"):
        build_remember("x", graph=bad)


def test_a_keyword_spelled_term_is_quoted_after_the_first():
    """A search word that spells a keyword is quoted where a bare one is syntax.

    ``recall@0 ferry top`` is a parse error — after the first term the grammar
    reads ``top`` as an unfinished ``top:`` clause — so a caller passing "top"
    as a search word gets the quoted form that means the word.
    """
    assert build_recall(["ferry", "top"]) == "recall@0 ferry 'top'"


def test_a_leading_keyword_spelled_term_stays_bare():
    """The first term is left bare so the server's ambiguity warning survives.

    ``recall since 7d`` parses as a two-term search and warns that it is one
    ``:`` from ``since:7d``. Quoting it here would silence the only signal the
    caller gets that their two search words read like a time bound.
    """
    assert build_recall(["since", "7d"]) == "recall@0 since 7d"


def test_a_query_phrase_takes_the_leading_slot_so_keywords_are_quoted():
    """With a phrase first, no keyword is in the position that reads as data."""
    assert build_recall(["top"], query="what is at the top") == (
        "recall@0 'what is at the top' 'top'"
    )
