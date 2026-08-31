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
from fraise_sdk.errors import FraiseAPIError, FraiseQueryError
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
    assert got == "remember@3 'anne loves the color orange' topic:'color' entity:'anne'"


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
    assert build_recall(topics=["birds"]) == "recall@0 topic:'birds'"


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


def test_anchor_values_are_quoted_as_data():
    """Anchor values can contain grammar-significant characters and spaces."""
    got = build_remember(
        "anne has references",
        topics=["topic:secret", "since 7d"],
        entities=["o'brien", "a@b.com"],
    )
    assert got == (
        "remember@0 'anne has references' "
        "topic:'topic:secret' topic:'since 7d' entity:'o''brien' entity:'a@b.com'"
    )


def test_syntax_significant_recall_terms_are_quoted():
    """A non-leading search term with FQL punctuation stays a term, not syntax."""
    assert build_recall(["anne", "topic:secret", "since:7d", "a@b.com"]) == (
        "recall@0 anne 'topic:secret' 'since:7d' 'a@b.com'"
    )


def test_leading_colon_term_is_quoted():
    """A single colon-bearing keyword is still caller data, not a filter clause."""
    assert build_recall(["topic:secret"]) == "recall@0 'topic:secret'"


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
@pytest.mark.integration
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
@pytest.mark.integration
def test_every_recall_the_builder_emits_parses(kwargs, client, query_graph):
    """Every keyword-seeded shape build_recall can produce is accepted."""
    text = build_recall(graph=query_graph, **kwargs)
    body = client.query(text)
    assert "results" in body


@pytest.mark.integration
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
@pytest.mark.integration
def test_awkward_values_still_parse(value, client, query_graph):
    """Values that stress the quoting are still one phrase to the parser.

    Quoting is the builder's job and finding the phrase boundary is the
    server's; a value that makes the parser stumble means the two disagree
    about where the phrase ends — including over the doubled-quote escape
    the builder applies to apostrophes.
    """
    client.query(build_remember(value, graph=query_graph, topics=["quoting"]))


@pytest.mark.integration
def test_anchor_values_with_fql_punctuation_still_parse(client, query_graph):
    """Quoted anchors keep FQL punctuation and whitespace inside the value."""
    client.query(
        build_remember(
            "anne keeps a tricky citation",
            graph=query_graph,
            topics=["topic:secret", "since 7d"],
            entities=["o'brien", "a@b.com"],
        )
    )


@pytest.mark.integration
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


@pytest.mark.integration
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
        {"topics": ["weather"], "entities": ["barometer"]},
        {"topics": ["weather"], "top": 3, "depth": 2},
    ],
)
@pytest.mark.integration
def test_a_recall_seeded_without_keywords_parses(kwargs, client, query_graph, encode):
    """Every keyword-free seed the builder allows is accepted by the grammar.

    ``build_recall`` documents "a recall needs at least one seed: keywords, a
    vector, or a topic/entity filter" and builds ``recall@2 topic:'weather'`` (or
    ``vec:$v``, or ``entity:...``) accordingly. The grammar used to want a word
    or quoted phrase before any clause and answered 400, so
    ``client.recall(topics=[...])`` and ``client.recall(vector=[...])`` — pure
    anchor and pure semantic search — were unreachable, and callers seeded with
    a keyword matching nothing ("zzznomatch") to get past the parser. The two
    agree now, and this is what says so.
    """
    text = build_recall(graph=query_graph, **kwargs)
    parameters = (
        {VECTOR_PARAM: encode("anything at all")} if kwargs.get("with_vector") else None
    )
    body = client.query(text, parameters=parameters)
    assert "results" in body


@pytest.mark.parametrize(
    "keywords",
    [
        ["barometer", "top"],
        ["barometer", "since"],
        ["barometer", "depth", "entity"],
        ["storm", "recall"],
    ],
)
@pytest.mark.integration
def test_a_keyword_spelled_search_word_survives_the_grammar(
    keywords, client, query_graph
):
    """A search word that spells a keyword reaches the engine as a word.

    The builder quotes these because a bare one reads as a clause after the
    first term — ``recall@2 barometer top`` is a parse error. Only a live parser
    can prove the quoting is the right escape, which is what this file is for.
    """
    body = client.query(build_recall(keywords, graph=query_graph))
    assert "results" in body


@pytest.mark.integration
def test_syntax_significant_search_words_survive_the_grammar(client, query_graph):
    """Colon, at-sign, dollar and apostrophe terms reach the parser as data."""
    body = client.query(
        build_recall(
            ["anne", "topic:secret", "since:7d", "a@b.com", "o'brien"],
            graph=query_graph,
        )
    )
    assert "results" in body


@pytest.mark.parametrize(
    "kwargs,expected",
    [
        ({"keywords": ["barometer"], "depth": 3}, "out of range"),
        ({"keywords": ["barometer"], "depth": 99}, "out of range"),
        ({"keywords": ["barometer"], "top": 100000}, "out of range"),
    ],
)
@pytest.mark.integration
def test_a_bound_past_the_servers_ceiling_names_the_range(
    kwargs, expected, client, query_graph
):
    """The builder's floor and the server's ceiling are different jobs.

    ``build_recall`` refuses a negative depth or a non-positive top because
    those are meaningless at any configuration; how deep or how many a *server*
    will go is operator-set, so only the server can refuse these — and its
    message has to carry the range, or an agent retries with another large
    number.
    """
    with pytest.raises(FraiseAPIError) as excinfo:
        client.query(build_recall(graph=query_graph, **kwargs))
    assert excinfo.value.status_code == 400
    assert expected in excinfo.value.message


@pytest.mark.integration
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
