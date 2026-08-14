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

"""Integration tests for fraise_sdk.query: does what it builds actually parse?

``build_remember`` and ``build_recall`` are pure string builders, so the unit
suite can only assert the string they produce — it compares one hand-written
expectation against another. The e2e suite sends its own hand-written strings.
Neither notices when the SDK's grammar drifts from the server's, which is the
one failure that breaks every SDK caller at once. This file closes that gap by
sending the builders' own output to a live parser.

The tests deliberately go through ``client.query`` rather than
``client.remember`` / ``client.recall``: the string under test has to be the one
the builder returned, unmediated.
"""

import pytest
from fraise_sdk.errors import FraiseAPIError
from fraise_sdk.query import VECTOR_PARAM, build_recall, build_remember


@pytest.mark.parametrize(
    "kwargs",
    [
        {},
        {"topics": ["weather"]},
        {"entities": ["barometer"]},
        {"topics": ["weather", "instruments"]},
        {"topics": ["weather"], "entities": ["barometer"]},
    ],
)
def test_every_remember_the_builder_emits_parses(kwargs, client, query_graph):
    """Every shape build_remember can produce is accepted by the live parser."""
    text = build_remember(
        "the barometer falls before a storm", graph=query_graph, **kwargs
    )
    client.query(text)


@pytest.mark.parametrize(
    "kwargs",
    [
        {"keywords": ["barometer"]},
        {"keywords": ["barometer", "storm"]},
        {"keywords": ["barometer"], "topics": ["weather"]},
        {"keywords": ["barometer"], "entities": ["barometer"]},
        {"keywords": ["barometer"], "top": 3},
        {"keywords": ["barometer"], "depth": 2},
        {"keywords": ["barometer"], "top": 3, "depth": 2},
        {
            "keywords": ["barometer", "storm"],
            "topics": ["weather"],
            "entities": ["barometer"],
            "top": 3,
            "depth": 2,
        },
    ],
)
def test_every_recall_the_builder_emits_parses(kwargs, client, query_graph):
    """Every keyword-seeded shape build_recall can produce is accepted."""
    text = build_recall(graph=query_graph, **kwargs)
    body = client.query(text)
    assert "results" in body


def test_a_query_phrase_recall_parses(client, query_graph):
    """A whole question as one quoted phrase term is accepted by the server.

    Reserved words inside the quotes ("topic") stay literal, and the trailing
    top: clause still parses as a clause.
    """
    body = client.query(
        build_recall(
            query="what topic has john been blogging about recently",
            graph=query_graph,
            top=10,
        )
    )
    assert "results" in body


@pytest.mark.parametrize(
    "value",
    [
        "a plain sentence with spaces",
        "punctuation, semicolons; and dashes - like this",
        "digits 1234 and symbols @ # % mixed in",
        "it's got an apostrophe, and rock 'n' roll has two",
        "le barometre chute avant la tempete",
        "line one\nline two\r\n\tindented",
        "déjà vu 😀 東京",
        'C:\\temp\\new says "hi"',
        "a nul\x00survives json transport",
        '{"looks": ["like", "json"]} - [markdown](too)',
        "a" * 300,
    ],
)
def test_awkward_values_still_parse(value, client, query_graph):
    """Values that stress the quoting are still one phrase to the parser.

    Quoting is the builder's job and finding the phrase boundary is the
    server's; a value that makes the parser stumble means the two disagree
    about where the phrase ends — including over the doubled-quote escape
    the builder applies to apostrophes.
    """
    client.query(build_remember(value, graph=query_graph, topics=["quoting"]))


def test_the_vector_placeholder_binds_to_the_parameter_the_builder_names(
    client, query_graph, encode
):
    """The name in ``vec:$v`` is the name the client sends the vector under.

    VECTOR_PARAM is a two-sided contract: the builder writes the placeholder
    into the string and the client sends ``{"v": [...]}`` beside it. Nothing
    else in the suite would notice if those two names drifted apart.
    """
    text = build_remember("a vector bound by name", graph=query_graph, with_vector=True)
    client.query(text, parameters={VECTOR_PARAM: encode("a vector bound by name")})


def test_the_vector_placeholder_is_really_bound_not_ignored(client, query_graph):
    """The same string without its parameter is rejected for the unbound name.

    The mirror of the test above: if the server ignored the placeholder rather
    than binding it, that test would prove nothing.
    """
    text = build_recall(["barometer"], graph=query_graph, with_vector=True)
    with pytest.raises(FraiseAPIError) as excinfo:
        client.query(text)
    assert f"${VECTOR_PARAM}" in excinfo.value.message


@pytest.mark.parametrize(
    "kwargs",
    [
        {"topics": ["weather"]},
        {"entities": ["barometer"]},
        {"with_vector": True},
    ],
)
def test_a_recall_without_keywords_does_not_parse(kwargs, client, query_graph, encode):
    """Every keyword-free seed the builder allows is rejected by the grammar.

    NOTE: this pins a defect, not an intended contract. ``build_recall``
    documents "a recall needs at least one seed: keywords, a vector, or a
    topic/entity filter" and builds ``recall@2 topic:weather`` (or ``vec:$v``,
    or ``entity:...``) accordingly — but the parser wants a word or quoted
    phrase before any clause and answers 400. So ``client.recall(topics=[...])``
    and ``client.recall(vector=[...])`` — pure semantic search — do not work end
    to end; only a keyword-seeded recall reaches the engine.

    Nothing caught this before because both suites sidestep it: the unit tests
    stop at the generated string, and the e2e vector tests seed with a keyword
    that matches no fact ("zzznomatch") without noting that the grammar leaves
    them no choice.

    Replace this with the working behaviour once the builder and the grammar
    agree — whichever side moves.
    """
    text = build_recall(graph=query_graph, **kwargs)
    parameters = (
        {VECTOR_PARAM: encode("anything at all")} if kwargs.get("with_vector") else None
    )
    with pytest.raises(FraiseAPIError) as excinfo:
        client.query(text, parameters=parameters)
    assert excinfo.value.status_code == 400
    assert "expected a word or quoted phrase" in excinfo.value.message


def test_the_builders_agree_with_the_graphs_vector_dimension(
    client, query_graph, vector_dim
):
    """A vector of the wrong width reaches the index and is refused there.

    Incidentally proof that the vector is bound and used, rather than parsed
    and dropped on the floor.
    """
    text = build_remember("a mis-sized vector", graph=query_graph, with_vector=True)
    with pytest.raises(FraiseAPIError):
        client.query(text, parameters={VECTOR_PARAM: [0.5] * (vector_dim // 2)})
