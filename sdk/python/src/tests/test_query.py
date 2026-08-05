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
    assert build_remember("the parrot is turquoise") == "remember@0 'the parrot is turquoise'"


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


def test_remember_rejects_apostrophe():
    with pytest.raises(FraiseQueryError, match="single quote"):
        build_remember("it's turquoise")


def test_remember_rejects_empty_value():
    with pytest.raises(FraiseQueryError):
        build_remember("   ")


def test_recall_with_keywords_and_clauses():
    got = build_recall(["anna", "bob"], graph=2, top=10, depth=5)
    assert got == "recall@2 anna bob top:10 depth:5"


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
def test_recall_rejects_non_positive_top_and_depth(bad):
    with pytest.raises(FraiseQueryError):
        build_recall(["x"], top=bad)
    with pytest.raises(FraiseQueryError):
        build_recall(["x"], depth=bad)


def test_negative_graph_is_rejected():
    with pytest.raises(FraiseQueryError, match="non-negative"):
        build_remember("x", graph=-1)
