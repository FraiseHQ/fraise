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

The unit tests pin the strings they build. The tests marked ``integration``
send those strings to a live server, whose parser judges whether they parse.
"""

import warnings
from datetime import date, datetime, timedelta, timezone

import pytest
from fraise_sdk.constants import VECTOR_PARAM
from fraise_sdk.errors import FraiseAPIError, FraiseQueryError, FraiseWarning
from fraise_sdk.models import Interval
from fraise_sdk.query import When, build_recall, build_remember


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
    """An apostrophe is doubled (the grammar's phrase escape), not rejected."""
    assert build_remember("it's turquoise") == "remember@0 'it''s turquoise'"


def test_remember_rejects_empty_value():
    """A blank fact raises FraiseQueryError instead of being sent."""
    with pytest.raises(FraiseQueryError):
        build_remember("   ")


def test_remember_keeps_free_text_verbatim_inside_the_quotes():
    """Ingestion feeds phrases arbitrary prose, which travels inside the quotes.

    Newlines, tabs, emoji and backslashes are untouched. Only apostrophes are
    rewritten, by doubling.
    """
    value = 'line one\nline two\t— déjà vu 😀 C:\\temp "quoted"'
    assert build_remember(value) == f"remember@0 '{value}'"


def test_recall_query_phrase_is_one_quoted_term():
    """A whole question travels as one quoted phrase term.

    Natural language then never collides with the grammar's reserved keywords.
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
    """The phrase escape covers the query too, so John's travels as John''s."""
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
    Iterated, it would file the fact under ``topic:b topic:i topic:l ...``, one
    anchor per letter. A recall by ``billing`` would then find nothing. The
    five cases are every builder argument that takes a sequence of strings, and
    each reaches the check on its own. The message names the fix.
    """
    with pytest.raises(FraiseQueryError) as exc_info:
        build(*args, **{param: "billing"})
    assert str(exc_info.value) == (
        f"{kind} must be a sequence of strings, not a string: 'billing'; "
        "did you mean ['billing']?"
    )


@pytest.mark.parametrize("bad", [0, -1])
def test_recall_rejects_non_positive_top(bad):
    """top must be positive. A recall that can return nothing is a caller bug."""
    with pytest.raises(FraiseQueryError):
        build_recall(["x"], top=bad)


@pytest.mark.parametrize("bad", [-1, -2])
def test_recall_rejects_negative_depth(bad):
    """depth must be non-negative. There is no lane below the floor."""
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

    Narrowed to a uint8, 256 would be graph 0, which is somebody else's memory.
    The builder refuses it before any request, so a mistyped graph id fails in
    the caller's own stack trace.
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
        ({"keywords": ["when", "said"]}, "recall@0 'when' 'said'"),
        ({"keywords": ["occurred"]}, "recall@0 'occurred'"),
        ({"keywords": ["When"]}, "recall@0 'When'"),
    ],
)
def test_a_keyword_spelled_search_word_is_quoted_and_warns(kwargs, expected):
    """A search word that spells a reserved word is quoted and warns, anywhere.

    The server reads a bare reserved word as syntax, in any position and any
    casing, and rejects it as a term. The caller passed a search word, so the
    builder writes the quoted form that means the word. It warns because the
    caller may have meant the clause, which only ``client.query`` can send.
    The warning names the caller's line, not the SDK's.
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
    the word ends. The caller passed one search word, and the quoted form
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

    A space, a colon, an apostrophe or a hyphen cannot stand in a bare word, so
    those values are quoted and travel whole. A reserved word stays bare. After
    the ``:`` only a value can appear, so ``topic:top`` is data and does not
    warn. The rule is the same on a read and on a write.
    """
    with warnings.catch_warnings():
        warnings.simplefilter("error", FraiseWarning)
        assert build_recall(topics=[value]) == f"recall@0 topic:{rendered}"
        assert build_recall(entities=[value]) == f"recall@0 entity:{rendered}"
        assert build_remember("x", topics=[value]) == (
            f"remember@0 'x' topic:{rendered}"
        )


@pytest.mark.parametrize(
    "said, rendered",
    [
        (date(2023, 5, 8), "'2023-05-08'"),
        (
            datetime(2023, 5, 8, 9, 30, tzinfo=timezone.utc),
            "'2023-05-08T09:30:00+00:00'",
        ),
        (
            datetime(2023, 5, 8, 23, 30, tzinfo=timezone(timedelta(hours=-7))),
            "'2023-05-08T23:30:00-07:00'",
        ),
        (
            datetime(2023, 5, 8, 9, 30, 15, 250000, tzinfo=timezone.utc),
            "'2023-05-08T09:30:15.250000+00:00'",
        ),
        (datetime(2023, 5, 8, 9, 30), "'2023-05-08T09:30:00+00:00'"),
    ],
)
def test_a_time_is_written_as_the_day_or_the_moment_it_names(said, rendered):
    """A date is written as its day, and a datetime as its moment and offset.

    The two are different times to the server: a day is the whole of it, and a
    moment is one instant. So a datetime is never cut down to its day, though
    it is a ``date`` too. The offset travels because the server keeps it: 23:30
    at UTC-7 on the 8th is the 9th in UTC, and the fact was said on the 8th. A
    datetime with no time zone is written in UTC, where a day is read.
    """
    assert build_remember("anna ran the race", said=said) == (
        f"remember@0 'anna ran the race' said:{rendered}"
    )


@pytest.mark.parametrize(
    "occurred, rendered",
    [
        ([date(2023, 5, 7)], "occurred:'2023-05-07'"),
        (
            [datetime(2023, 5, 7, 18, 30, tzinfo=timezone.utc)],
            "occurred:'2023-05-07T18:30:00+00:00'",
        ),
        (
            [
                Interval(
                    start=datetime(2023, 5, 1, tzinfo=timezone.utc),
                    end=datetime(2023, 6, 1, tzinfo=timezone.utc),
                )
            ],
            "occurred:'2023-05-01T00:00:00+00:00'..'2023-06-01T00:00:00+00:00'",
        ),
        (
            [Interval(start=datetime(2023, 5, 20, tzinfo=timezone.utc), end=None)],
            "occurred:'2023-05-20T00:00:00+00:00'..",
        ),
        (
            [
                Interval(
                    start=datetime(2023, 5, 7, 18, 30, tzinfo=timezone.utc),
                    end=datetime(2023, 5, 7, 18, 30, tzinfo=timezone.utc),
                )
            ],
            "occurred:'2023-05-07T18:30:00+00:00'",
        ),
        (
            [Interval(start=datetime(2023, 5, 1), end=datetime(2023, 6, 1))],
            "occurred:'2023-05-01T00:00:00+00:00'..'2023-06-01T00:00:00+00:00'",
        ),
        (
            [date(2023, 5, 1), date(2023, 5, 28)],
            "occurred:'2023-05-01' occurred:'2023-05-28'",
        ),
    ],
)
def test_each_time_a_fact_happened_is_one_occurred_clause(occurred, rendered):
    """Each value in ``occurred`` is its own clause: a time, or an interval's range.

    An interval is written as the range between its two moments, which the
    server reads back as the same span, start held and end excluded. One that
    has not ended is left open after the ``..``, and one that ends where it
    starts is that single moment. A fact that happened twice carries two
    clauses in the order given, never one span stretched over both.
    """
    assert build_remember("anna trained for the race", occurred=occurred) == (
        f"remember@0 'anna trained for the race' {rendered}"
    )


def test_remember_writes_its_times_between_the_anchors_and_the_vector():
    """The times follow the anchors, ``said`` before ``occurred``, and the vector is last."""
    got = build_remember(
        "anna ran the race",
        graph=3,
        topics=["sport"],
        entities=["anna"],
        said=date(2023, 5, 8),
        occurred=[date(2023, 5, 7)],
        with_vector=True,
    )
    assert got == (
        "remember@3 'anna ran the race' topic:sport entity:anna "
        f"said:'2023-05-08' occurred:'2023-05-07' vec:${VECTOR_PARAM}"
    )


@pytest.mark.parametrize(
    "occurred",
    [
        date(2023, 5, 7),
        datetime(2023, 5, 7, 18, 30, tzinfo=timezone.utc),
        Interval(start=datetime(2023, 5, 20, tzinfo=timezone.utc), end=None),
        "2023-05-07",
    ],
)
def test_one_time_where_occurred_wants_a_sequence_is_rejected(occurred):
    """``occurred=date(...)`` is refused by name, with the fix in the message.

    A fact can have happened more than once, so ``occurred`` takes a sequence
    where ``said`` takes one time. One time on its own is the mistake that
    difference invites. Left to the loop, it would fail as "not iterable", and
    a string would be read one character at a time.
    """
    with pytest.raises(FraiseQueryError) as exc_info:
        build_remember("anna ran the race", occurred=occurred)
    assert str(exc_info.value) == (
        f"occurred must be a sequence of times, not one value: {occurred!r}; "
        f"did you mean [{occurred!r}]?"
    )


@pytest.mark.parametrize(
    ("build", "args", "kwargs", "kind", "text"),
    [
        (build_remember, ("a fact",), {"said": "2023-05-08"}, "said", "2023-05-08"),
        (
            build_remember,
            ("a fact",),
            {"occurred": ["2023-05-07"]},
            "occurred",
            "2023-05-07",
        ),
        (build_recall, (["anna"],), {"since": "7d"}, "since", "7d"),
        (build_recall, (["anna"],), {"until": "2023-05-09"}, "until", "2023-05-09"),
    ],
)
def test_a_time_given_as_text_is_rejected(build, args, kwargs, kind, text):
    """A string is never read as a time, and the message names both ways out.

    A string could be a day, a moment in some zone, or a duration such as
    ``7d``. Telling them apart is parsing FQL, which is the server's job. The
    builders write a time from a ``date`` or a ``datetime``. Anything else is
    written as FQL and sent with ``client.query``.
    """
    with pytest.raises(FraiseQueryError) as exc_info:
        build(*args, **kwargs)
    assert str(exc_info.value) == (
        f"{kind} cannot be written from the str {text!r}; give a date or a "
        "datetime, or write the query as FQL and send it with client.query"
    )


@pytest.mark.parametrize(
    "start, end",
    [
        (date(2023, 5, 1), date(2023, 5, 3)),
        (datetime(2023, 5, 1, tzinfo=timezone.utc), date(2023, 5, 3)),
        (date(2023, 5, 1), None),
    ],
)
def test_an_interval_with_a_day_at_an_edge_is_rejected(start, end):
    """An interval runs between two moments, so a ``date`` at an edge is refused.

    An interval excludes its end, and a range of days holds its last day.
    Written as ``'2023-05-01'..'2023-05-03'``, the fact would cover the 3rd, a
    day the interval never held. The message names the fix: a whole day is a
    ``date`` on its own, and a longer span runs between two datetimes.
    """
    span = Interval(start=start, end=end)
    with pytest.raises(FraiseQueryError) as exc_info:
        build_remember("anna trained for the race", occurred=[span])
    assert str(exc_info.value) == (
        f"occurred takes an Interval between two datetimes, got {span!r}; "
        "a whole day is given as a date on its own"
    )


@pytest.mark.parametrize(
    "kwargs, expected",
    [
        ({"since": date(2023, 5, 7)}, "recall@0 anna since:'2023-05-07'"),
        ({"until": date(2023, 5, 9)}, "recall@0 anna until:'2023-05-09'"),
        (
            {"since": date(2023, 5, 7), "until": date(2023, 5, 9)},
            "recall@0 anna since:'2023-05-07' until:'2023-05-09'",
        ),
        (
            {"since": datetime(2023, 5, 7, 18, 30, tzinfo=timezone.utc)},
            "recall@0 anna since:'2023-05-07T18:30:00+00:00'",
        ),
        (
            {"until": datetime(2023, 5, 7, 18, 30)},
            "recall@0 anna until:'2023-05-07T18:30:00+00:00'",
        ),
    ],
)
def test_a_recall_bound_is_written_as_the_time_it_names(kwargs, expected):
    """A bound is written as a remember's time is, ``since`` before ``until``.

    One rule on both sides means a fact stored with ``said=date(2023, 5, 7)``
    and a recall bounded by that date name the same day.
    """
    assert build_recall(["anna"], **kwargs) == expected


@pytest.mark.parametrize(
    "when, rendered",
    [
        (When("regatta"), "(when regatta)"),
        (When("regatta", "harbour"), "(when regatta harbour)"),
        (When("regatta", clock="happened"), "(when:happened regatta)"),
        (When("regatta", clock="said"), "(when:said regatta)"),
        (When("regatta", clock="written"), "(when:written regatta)"),
        (
            When(query="when did anna's race start"),
            "(when 'when did anna''s race start')",
        ),
        (
            When(topics=["sailing"], entities=["anna smith"]),
            "(when topic:sailing entity:'anna smith')",
        ),
        (When("regatta", top=3, depth=1), "(when regatta top:3 depth:1)"),
        (When("e-mail"), "(when 'e-mail')"),
    ],
)
def test_a_when_is_written_in_parentheses_as_the_search_it_runs(when, rendered):
    """A when is ``(when ...)`` around a recall's own search, on either bound.

    The parentheses are what make it a bound: the server rejects a bare
    ``when`` after ``since:``. Inside them the search is written as a recall's
    is, so a value is quoted by the same rule in both places. A clock is glued
    to the word with a colon. None is written unless one was given, which
    leaves the default to the server.
    """
    with warnings.catch_warnings():
        warnings.simplefilter("error", FraiseWarning)
        assert build_recall(["anna"], since=when) == f"recall@0 anna since:{rendered}"
        assert build_recall(["anna"], until=when) == f"recall@0 anna until:{rendered}"


def test_a_reserved_word_keyword_in_a_when_is_quoted_and_warns():
    """A when's keywords are search words too: a reserved one is quoted, and warns.

    ``When("said", "regatta")`` asks for the facts that hold the word "said".
    Bare, the server would read it as the ``said:`` clause and reject the
    query. The warning names the caller's line, as a recall's does.
    """
    with pytest.warns(FraiseWarning, match="is a reserved word") as record:
        got = build_recall(["anna"], until=When("said", "regatta"))
    assert got == "recall@0 anna until:(when 'said' regatta)"
    assert record[0].filename == __file__


def test_recall_writes_its_bounds_after_the_search_and_before_the_vector():
    """The bounds follow ``top`` and ``depth``, and the vector placeholder is last."""
    got = build_recall(
        ["anna"],
        graph=2,
        query="what did anna do",
        topics=["sport"],
        entities=["anna"],
        top=5,
        depth=1,
        since=date(2023, 5, 1),
        until=When("regatta"),
        with_vector=True,
    )
    assert got == (
        "recall@2 'what did anna do' anna topic:sport entity:anna top:5 depth:1 "
        f"since:'2023-05-01' until:(when regatta) vec:${VECTOR_PARAM}"
    )


def test_a_when_with_nothing_to_search_with_is_rejected():
    """A when needs a seed for the reason a recall does: it has nothing to search from."""
    with pytest.raises(FraiseQueryError) as exc_info:
        build_recall(["anna"], until=When())
    assert str(exc_info.value) == (
        "a when needs at least one seed: keywords, a query, or a topic/entity anchor"
    )


@pytest.mark.parametrize("clock", ["heard", "Said"])
def test_a_when_naming_an_unknown_clock_is_rejected(clock):
    """A clock the grammar does not read is refused, and the message lists the three.

    The clock is written into the query as it is given. A misspelt one would
    come back as the server's parse error, about a string the caller never
    wrote. It is lower case, as the grammar writes it.
    """
    with pytest.raises(FraiseQueryError) as exc_info:
        build_recall(["anna"], since=When("regatta", clock=clock))
    assert str(exc_info.value) == (
        f"clock must be one of happened, said, written, got {clock!r}"
    )


@pytest.mark.parametrize(
    "kwargs, message",
    [
        ({"top": 0}, "top must be positive, got 0"),
        ({"depth": -1}, "depth must be non-negative, got -1"),
        (
            {"topics": "sailing"},
            "topic must be a sequence of strings, not a string: 'sailing'; "
            "did you mean ['sailing']?",
        ),
    ],
)
def test_a_when_is_held_to_a_recalls_own_limits(kwargs, message):
    """What a recall refuses in its own search, a when refuses in its.

    A when's search is written through the same path as the recall around it.
    So a ``top`` of zero, a negative ``depth`` or a bare string of anchors
    fails the same way in either place.
    """
    with pytest.raises(FraiseQueryError) as exc_info:
        build_recall(["anna"], until=When("regatta", **kwargs))
    assert str(exc_info.value) == message


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
        {"said": date(2023, 5, 8)},
        {"said": datetime(2023, 5, 8, 9, 30, tzinfo=timezone.utc)},
        {
            "said": datetime(
                2023, 5, 8, 23, 30, 15, 250000, tzinfo=timezone(timedelta(hours=-7))
            )
        },
        {"said": datetime(2023, 5, 8, 9, 30)},
        {"occurred": [date(2023, 5, 7)]},
        {"occurred": [datetime(2023, 5, 7, 18, 30, tzinfo=timezone.utc)]},
        {
            "occurred": [
                Interval(
                    start=datetime(2023, 5, 1, tzinfo=timezone.utc),
                    end=datetime(2023, 6, 1, tzinfo=timezone.utc),
                )
            ]
        },
        {
            "occurred": [
                Interval(start=datetime(2023, 5, 20, tzinfo=timezone.utc), end=None)
            ]
        },
        {"occurred": [date(2023, 5, 1), date(2023, 5, 28)]},
        {
            "topics": ["weather"],
            "entities": ["barometer"],
            "said": date(2023, 5, 8),
            "occurred": [date(2023, 5, 7)],
        },
    ],
)
@pytest.mark.integration
def test_every_dated_remember_the_builder_emits_parses(kwargs, client, query_graph):
    """Every way the builder writes a time is a time the live parser reads.

    The builder writes a moment as Python prints it: ``+00:00`` where FQL
    examples write ``Z``, and microseconds where the moment has them. Only the
    server can say those are time literals, and that a range or an open ``..``
    is one where ``occurred:`` takes it.
    """
    text = build_remember(
        "the clockprobe ticks in the hall", graph=query_graph, **kwargs
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


@pytest.mark.parametrize(
    "kwargs",
    [
        {"since": date(2023, 5, 7)},
        {"until": date(2023, 5, 9)},
        {"since": date(2023, 5, 7), "until": date(2023, 5, 9)},
        {"since": datetime(2023, 5, 7, 18, 30, tzinfo=timezone.utc)},
        {
            "until": datetime(
                2023, 5, 8, 23, 30, 15, 250000, tzinfo=timezone(timedelta(hours=-7))
            )
        },
        {"since": datetime(2023, 5, 7, 18, 30)},
        {"until": When("clockprobe")},
        {"since": When("clockprobe", clock="happened")},
        {"since": When("clockprobe", clock="said")},
        {"until": When("clockprobe", clock="written", top=3)},
        {"until": When(query="when did the clockprobe tick")},
        {"since": When(topics=["weather"], entities=["barometer"], depth=1)},
        {"since": When("clockprobe"), "until": When("barometer", clock="said")},
        {
            "since": date(2023, 5, 1),
            "until": When("clockprobe"),
            "topics": ["weather"],
            "top": 3,
            "depth": 1,
        },
    ],
)
@pytest.mark.integration
def test_every_bounded_recall_the_builder_emits_parses(kwargs, client, query_graph):
    """Every bound the builder writes is one the live parser reads.

    A bound is a time written as a remember's is, or a when in parentheses.
    The when is the form only a server can vouch for: its clock glued on with a
    colon, a recall's own clauses inside the parentheses, and one on each bound
    of the same recall.
    """
    text = build_recall(["barometer"], graph=query_graph, **kwargs)
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

    Quoting is the builder's job, and finding the phrase boundary is the
    server's. A value that makes the parser stumble means the two disagree
    about where the phrase ends. That includes the doubled-quote escape the
    builder applies to apostrophes.
    """
    client.query(build_remember(value, graph=query_graph, topics=["quoting"]))


@pytest.mark.integration
def test_the_vector_placeholder_binds_to_the_parameter_the_builder_names(
    client, query_graph, encode
):
    """The name in ``vec:$v`` is the name the client sends the vector under.

    VECTOR_PARAM is a two-sided contract. The builder writes the placeholder
    into the string, and the client sends ``{"v": [...]}`` beside it. Nothing
    else in the suite would notice if those two names drifted apart.
    """
    text = build_remember("a vector bound by name", graph=query_graph, with_vector=True)
    client.query(text, parameters={VECTOR_PARAM: encode("a vector bound by name")})


@pytest.mark.integration
def test_the_vector_placeholder_is_really_bound_not_ignored(client, query_graph):
    """A placeholder sent without its parameter is rejected for the unbound name.

    This mirrors the test above. If the server ignored the placeholder rather
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
    ``entity:...``). If the grammar disagreed, ``client.recall(topics=[...])``
    and ``client.recall(vector=[...])`` would need a keyword the caller does
    not mean. Those are pure anchor search and pure semantic search.
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
        ["when", "said"],
        ["barometer", "occurred"],
        ["When"],
    ],
)
@pytest.mark.integration
def test_a_keyword_spelled_search_word_survives_the_grammar(
    keywords, client, query_graph
):
    """A search word that spells a keyword reaches the engine as a word.

    The builder quotes these because a bare reserved word is syntax, in any
    position and any casing. ``recall@2 since 7d`` is a parse error. Only a
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

    The builder writes an anchor bare or quoted, depending on what it holds.
    This pins three things:
    - Either form lands on the anchor a caller writing FQL would name, folded
      to lower case.
    - A reserved word is an ordinary anchor after the ``:``.
    - A space, a colon or an apostrophe survives the trip.
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

    ``build_recall`` refuses a negative depth or a non-positive top, because
    those are meaningless under any configuration. How deep or how many a
    *server* will go is set by its operator, so only the server can refuse
    these. Its message must carry the range, or an agent retries with another
    large number.
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

    The first vector a graph stores fixes its width. So the test first stores
    one at the suite's width, rather than rely on an earlier test to have done
    it. Run alone, the mis-sized vector would otherwise be the first. It would
    be accepted and fix the graph at the wrong width for every later test. The
    test also proves that the vector is bound and used, not parsed and dropped.
    """
    sized = build_remember("a well-sized vector", graph=query_graph, with_vector=True)
    client.query(sized, parameters={VECTOR_PARAM: encode("a well-sized vector")})
    text = build_remember("a mis-sized vector", graph=query_graph, with_vector=True)
    with pytest.raises(FraiseAPIError):
        client.query(text, parameters={VECTOR_PARAM: [0.5] * (vector_dim // 2)})
