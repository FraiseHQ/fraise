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

"""Tests for the pure query-string builders.

The unit tests pin the strings they build; the tests marked ``integration``
send those strings to a live server, whose parser is the judge of whether
they parse.
"""

import warnings

import pytest
from fraise_sdk.constants import VECTOR_PARAM
from fraise_sdk.errors import FraiseAPIError, FraiseQueryError, FraiseWarning
from fraise_sdk.query import build_recall, build_remember


def test_remember_minimal():
    """A bare remember is the default graph and the quoted fact."""
    assert (
        build_remember("the parrot is turquoise")
        == "remember@0 'the parrot is turquoise'"
    )


def test_remember_with_graph_topics_and_entities():
    """The selector is glued to the verb, and the topics precede the entities."""
    got = build_remember(
        "anne loves the color orange",
        graph=3,
        topics=["color"],
        entities=["anne"],
    )
    assert got == "remember@3 'anne loves the color orange' topic:color entity:anne"


def test_remember_with_vector_appends_placeholder():
    """``with_vector`` appends the placeholder the client binds in the parameters."""
    got = build_remember("the parrot is turquoise", graph=6, with_vector=True)
    assert got == f"remember@6 'the parrot is turquoise' vec:${VECTOR_PARAM}"


def test_remember_escapes_apostrophes():
    """An apostrophe is doubled — the grammar's phrase escape — not rejected."""
    assert build_remember("it's turquoise") == "remember@0 'it''s turquoise'"


def test_remember_rejects_empty_value():
    """A blank fact raises FraiseQueryError instead of being sent."""
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
    """Keywords come first, bare, then the top and depth clauses."""
    got = build_recall(["anna", "bob"], graph=2, top=10, depth=2)
    assert got == "recall@2 anna bob top:10 depth:2"


def test_recall_with_vector_only():
    """A vector alone seeds the recall, which then carries only the placeholder."""
    assert build_recall(graph=6, with_vector=True) == f"recall@6 vec:${VECTOR_PARAM}"


def test_recall_topic_seed_is_enough():
    """A topic alone seeds the recall, so no keyword is needed."""
    assert build_recall(topics=["birds"]) == "recall@0 topic:birds"


def test_recall_requires_a_seed():
    """A recall with no keyword, query, anchor or vector has nothing to search from."""
    with pytest.raises(FraiseQueryError, match="at least one seed"):
        build_recall(graph=1)


@pytest.mark.parametrize(
    ("build", "args", "param", "kind"),
    [
        (build_remember, ("a fact",), "topics", "topic"),
        (build_remember, ("a fact",), "entities", "entity"),
        (build_recall, (), "topics", "topic"),
        (build_recall, (), "entities", "entity"),
        (build_recall, (), "keywords", "keyword"),
    ],
)
def test_a_bare_string_where_a_sequence_is_wanted_is_rejected(build, args, param, kind):
    """``topics="billing"`` is refused by name, never expanded letter by letter.

    ``str`` satisfies ``Sequence[str]``, so the bare string type-checks.
    Iterated, it would file the fact under ``topic:b topic:i topic:l ...``,
    one anchor per letter, and a recall by ``billing`` would find nothing. The
    five cases are every path that takes a sequence, each reaching the check
    independently, and the message names the fix.
    """
    with pytest.raises(FraiseQueryError) as exc_info:
        build(*args, **{param: "billing"})
    assert str(exc_info.value) == (
        f"{kind} must be a sequence of strings, not a string: 'billing'; "
        "did you mean ['billing']?"
    )


@pytest.mark.parametrize("bad", [0, -1])
def test_recall_rejects_non_positive_top(bad):
    """top must be positive — a recall that can return nothing is a caller bug."""
    with pytest.raises(FraiseQueryError):
        build_recall(["x"], top=bad)


@pytest.mark.parametrize("bad", [-1, -2])
def test_recall_rejects_negative_depth(bad):
    """depth must be non-negative; there is no lane below the floor."""
    with pytest.raises(FraiseQueryError):
        build_recall(["x"], depth=bad)


def test_recall_emits_depth_zero():
    """depth:0 is the explicit floor lane (text/vector only) and travels verbatim."""
    assert build_recall(["x"], depth=0) == "recall@0 x depth:0"


def test_negative_graph_is_rejected():
    """A negative graph raises before anything is sent."""
    with pytest.raises(FraiseQueryError, match="non-negative"):
        build_remember("x", graph=-1)


@pytest.mark.parametrize("bad", [256, 300, 99999])
def test_graph_above_the_uint8_range_is_rejected(bad):
    """A selector travels as a uint8, so no graph above 255 can be named.

    Narrowed to a uint8, 256 would be graph 0: somebody else's memory. The
    builder refuses it before any request, so a mistyped graph id fails in the
    caller's own stack trace.
    """
    with pytest.raises(FraiseQueryError, match="at most 255"):
        build_remember("x", graph=bad)


@pytest.mark.parametrize(
    "kwargs, expected",
    [
        ({"keywords": ["since", "7d"]}, "recall@0 'since' 7d"),
        ({"keywords": ["ferry", "top"]}, "recall@0 ferry 'top'"),
        ({"keywords": ["Top"]}, "recall@0 'Top'"),
        (
            {"keywords": ["top"], "query": "what is at the top"},
            "recall@0 'what is at the top' 'top'",
        ),
    ],
)
def test_a_keyword_spelled_search_word_is_quoted_and_warns(kwargs, expected):
    """A search word that spells a reserved word is quoted and warns, anywhere.

    The server reads a bare reserved word as syntax in every position and in
    any casing, and rejects it as a term; the caller passed a search word, so
    the builder writes the quoted form that means the word. It warns because
    the caller may have meant the clause instead, which only ``client.query``
    can send, and the warning names the caller's line, not the SDK's.
    """
    with pytest.warns(FraiseWarning, match="is a reserved word") as record:
        assert build_recall(**kwargs) == expected
    assert record[0].filename == __file__


@pytest.mark.parametrize(
    "keywords, expected",
    [
        (["e-mail"], "recall@0 'e-mail'"),
        (["ferry", "v1.2"], "recall@0 ferry 'v1.2'"),
        (["new york"], "recall@0 'new york'"),
        (["café", "東京"], "recall@0 café 東京"),
    ],
)
def test_a_search_word_that_is_not_one_plain_word_is_quoted(keywords, expected):
    """Outside quotes a word is letters and digits only, so anything else is quoted.

    The server rejects ``recall e-mail`` at the ``-`` rather than guess where
    the word ends; the caller passed one search word, and the quoted form
    carries it whole. Letters in any script are plain and stay bare. None of
    these can be read as syntax, so none of them warns.
    """
    with warnings.catch_warnings():
        warnings.simplefilter("error", FraiseWarning)
        assert build_recall(keywords) == expected


@pytest.mark.parametrize(
    "value, rendered",
    [
        ("billing", "billing"),
        ("Harbour", "Harbour"),
        ("top", "top"),
        ("US elections", "'US elections'"),
        ("ratio:odds", "'ratio:odds'"),
        ("o'brien", "'o''brien'"),
        ("machine-learning", "'machine-learning'"),
    ],
)
def test_an_anchor_value_is_quoted_only_when_a_bare_word_cannot_hold_it(
    value, rendered
):
    """A topic or entity is bare when it is one plain word, quoted otherwise.

    A space, a colon, an apostrophe or a hyphen cannot stand in a bare word,
    so those values are quoted and travel whole. A reserved word stays bare:
    after the ``:`` only a value can appear, so ``topic:top`` is data and does
    not warn. The rule is the same on a read and on a write.
    """
    with warnings.catch_warnings():
        warnings.simplefilter("error", FraiseWarning)
        assert build_recall(topics=[value]) == f"recall@0 topic:{rendered}"
        assert build_recall(entities=[value]) == f"recall@0 entity:{rendered}"
        assert build_remember("x", topics=[value]) == (
            f"remember@0 'x' topic:{rendered}"
        )


@pytest.mark.parametrize(
    "kwargs",
    [
        {},
        {"topics": ["weather"]},
        {"entities": ["barometer"]},
        {"topics": ["weather", "instruments"]},
        {"topics": ["weather"], "entities": ["barometer"]},
        {"topics": ["machine-learning"], "entities": ["o'brien"]},
        {"topics": ["my project"], "entities": ["anna smith"]},
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
        {"keywords": ["e-mail", "v1.2"], "topics": ["machine-learning"]},
        {
            "keywords": ["new york"],
            "topics": ["my project"],
            "entities": ["anna smith"],
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
    """A placeholder sent without its parameter is rejected for the unbound name.

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

    ``build_recall`` takes a topic, an entity or a vector as a recall's only
    seed, and builds ``recall@2 topic:weather`` (or ``vec:$v``, or
    ``entity:...``) accordingly. If the grammar disagreed,
    ``client.recall(topics=[...])`` and ``client.recall(vector=[...])``, pure
    anchor and pure semantic search, could not be asked without a keyword the
    caller does not mean.
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
        ["since", "7d"],
        ["Top", "shelf"],
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

    The builder quotes these because a bare reserved word is syntax in every
    position and in any casing — ``recall@2 since 7d`` is a parse error. Only a
    live parser can prove the quoting is the right escape.
    """
    with pytest.warns(FraiseWarning, match="is a reserved word"):
        text = build_recall(keywords, graph=query_graph)
    body = client.query(text)
    assert "results" in body


@pytest.mark.parametrize(
    "value, anchor",
    [
        ("billing", "topic:billing"),
        ("top", "topic:top"),
        ("US elections", "topic:'us elections'"),
        ("ratio:odds", "topic:'ratio:odds'"),
        ("o'brien", "topic:'o''brien'"),
        ("Harbour", "topic:harbour"),
    ],
)
@pytest.mark.integration
def test_a_quoted_anchor_is_the_anchor_fql_names(value, anchor, client, query_graph):
    """A topic written through the builder is the anchor hand-written FQL names.

    The builder writes an anchor bare or quoted depending on what it holds, so
    this pins that either form lands on the same anchor a caller writing FQL
    would name — folded to lower case — that a reserved word is an ordinary
    anchor after the ``:``, and that a space, a colon or an apostrophe survives
    the trip.
    """
    fact = f"the anchor probe for {value}"
    client.query(build_remember(fact, graph=query_graph, topics=[value]))
    body = client.query(f"recall@{query_graph} {anchor}")
    assert fact in [hit["value"] for hit in body["results"]["hits"]]


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
    client, query_graph, vector_dim, encode
):
    """A vector of the wrong width reaches the index and is refused there.

    The first vector a graph stores fixes its width, so the test stores one at
    the suite's width itself rather than relying on an earlier test to have
    done it: run alone, the mis-sized vector would otherwise be the first, be
    accepted, and fix the graph at the wrong width for every test after it.
    Incidentally proof that the vector is bound and used, rather than parsed
    and dropped on the floor.
    """
    sized = build_remember("a well-sized vector", graph=query_graph, with_vector=True)
    client.query(sized, parameters={VECTOR_PARAM: encode("a well-sized vector")})
    text = build_remember("a mis-sized vector", graph=query_graph, with_vector=True)
    with pytest.raises(FraiseAPIError):
        client.query(text, parameters={VECTOR_PARAM: [0.5] * (vector_dim // 2)})
