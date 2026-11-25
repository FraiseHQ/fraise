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

"""Parsing of the server's JSON envelope into the typed models.

The unit tests parse recorded and hand-written bodies with no I/O. The tests
marked ``integration`` parse what a live server sends.
"""

from datetime import date, datetime, timedelta, timezone

import pytest
from fraise_sdk.models import (
    Catalogue,
    CatalogueEntry,
    Contribution,
    GraphStats,
    Hit,
    HitExplain,
    Interval,
    RecallExplain,
    RecallResult,
)


def test_graph_stats_names_the_counts_for_what_they_count(stats_response):
    """The wire's graph-theory names land on the fields named for their meaning.

    ``order`` counts vertices and ``size`` counts edges. Reading either into
    the other, or into ``nodes``, would misreport a graph's shape without
    failing.
    """
    row = GraphStats.from_json(stats_response["graphs"][0])

    assert row == GraphStats(
        id=0,
        vertices=4,
        edges=3,
        nodes=7,
        vectors=0,
        forest_entries=0,
        relevance="bm25",
    )


def test_graph_stats_without_relevance_is_none(stats_response):
    """A row from a server that predates ``relevance`` still parses.

    The SDK supports servers that never send the field. Reading it as required
    would turn every ``stats()`` against them into a ``KeyError``.
    """
    row = {k: v for k, v in stats_response["graphs"][0].items() if k != "relevance"}

    assert GraphStats.from_json(row).relevance is None


def test_from_json_defaults_to_no_warnings():
    """Omitting warnings yields an empty list, never None.

    A clean response, or a server that predates warnings, produces this shape.
    A caller iterating ``result.warnings`` must not have to guard it. It also
    keeps direct ``from_json`` callers that pass only the results dict working.
    """
    result = RecallResult.from_json({"count": 0, "hits": []})

    assert result.warnings == []


def test_from_json_carries_warnings_beside_the_hits():
    """Warnings pass through as a plain list of strings.

    The hits they arrived beside parse exactly as they would without them.
    """
    result = RecallResult.from_json(
        {"count": 1, "hits": [{"value": "since the storm", "score": 1.0}]},
        warnings=["parse warning at column 15: ..."],
    )

    assert result.warnings == ["parse warning at column 15: ..."]
    assert result.count == 1
    assert [hit.value for hit in result] == ["since the storm"]


def test_from_json_defaults_to_a_populated_graph():
    """``empty`` is False unless the caller says otherwise.

    It rides the status line (204), not the body. So ``from_json`` cannot infer
    it from the payload and must not try. An empty result set is the ordinary
    miss until the response says the graph itself was empty.
    """
    result = RecallResult.from_json({"count": 0, "hits": []})

    assert result.empty is False


def test_from_json_carries_the_empty_graph_flag():
    """A 204 has no body, so the empty result is paired with the flag by hand.

    The two halves of an empty answer come from different places: the body, or
    its absence, and the status. This is where they are joined.
    """
    result = RecallResult.from_json({}, empty=True)

    assert result.empty is True
    assert result.count == 0
    assert result.hits == []
    assert bool(result) is False


def test_an_explained_hit_carries_its_seed_decay_and_contributions(explain_response):
    """Each hit parses its block into a seed flag, a decay and typed contributions.

    The hit matched the text, so it seeded the recall, and was also reached
    through the polly anchor the recall names. Its fact was written moments
    before the recall, so its decay is just under 1. ``via`` and ``degree``
    ride only on the graph sighting. A text sighting involves no anchor, so it
    leaves them unset rather than zero.
    """
    result = RecallResult.from_json(explain_response["results"])

    assert result.hits[1].explain == HitExplain(
        seed=True,
        decay=0.9999996547997493,
        contributions=[
            Contribution(
                source="text", score=0.48903447372503717, rank=0, count=1, held=1.0
            ),
            Contribution(
                source="graph",
                score=0.9225902466493349,
                rank=0,
                count=2,
                via="polly",
                degree=3,
                named=True,
            ),
        ],
    )


@pytest.mark.parametrize(
    ("named", "want"),
    [({"named": True}, True), ({}, False)],
)
def test_a_graph_sighting_says_whether_the_recall_named_its_anchor(named, want):
    """``named`` parses from the wire, and its absence reads as not named.

    A named anchor owes no fair share, so a client recomputing the score needs
    the flag. The server omits it when false, so a missing key must not fail.
    """
    data = {
        "source": "graph",
        "score": 2.0,
        "rank": 0,
        "count": 1,
        "via": "archive",
        "degree": 8,
    }
    assert Contribution.from_json(data | named).named is want


@pytest.mark.parametrize(
    ("held", "want"),
    [({"held": 0.5}, 0.5), ({}, 0.0)],
)
def test_a_text_sighting_says_how_much_of_the_query_its_words_hold(held, want):
    """``held`` parses from the wire, and its absence reads as holding none.

    The server omits it when the fact's own words hold none of the query, so a
    missing key must not fail; a client recomputing the score needs it to know
    how far the vector was trusted.
    """
    data = {"source": "text", "score": 1.5, "rank": 0, "count": 1}
    assert Contribution.from_json(data | held).held == want


@pytest.mark.parametrize(
    "data, want",
    [
        (
            {"start": "2023-05-07T00:00:00Z", "end": "2023-05-08T00:00:00Z"},
            Interval(
                start=datetime(2023, 5, 7, tzinfo=timezone.utc),
                end=datetime(2023, 5, 8, tzinfo=timezone.utc),
            ),
        ),
        (
            {"start": "2023-05-20T00:00:00Z"},
            Interval(start=datetime(2023, 5, 20, tzinfo=timezone.utc), end=None),
        ),
        (
            {"start": "2023-05-07T18:30:00Z", "end": "2023-05-07T18:30:00Z"},
            Interval(
                start=datetime(2023, 5, 7, 18, 30, tzinfo=timezone.utc),
                end=datetime(2023, 5, 7, 18, 30, tzinfo=timezone.utc),
            ),
        ),
        (
            {
                "start": "2023-05-07T18:30:00.123456+02:00",
                "end": "2023-05-09T00:00:00+02:00",
            },
            Interval(
                start=datetime(
                    2023, 5, 7, 18, 30, 0, 123456, tzinfo=timezone(timedelta(hours=2))
                ),
                end=datetime(2023, 5, 9, tzinfo=timezone(timedelta(hours=2))),
            ),
        ),
    ],
)
def test_a_span_parses_into_the_moments_it_runs_between(data, want):
    """A span's edges parse into datetimes, and an end the server left out stays out.

    The server omits the end of a span that has not ended. ``None`` says so,
    where a made-up moment would close a span that is still open. A single
    moment arrives with its end equal to its start, and parses that way.
    """
    assert Interval.from_json(data) == want


def test_a_parsed_time_keeps_the_offset_it_was_written_with():
    """A time parses in the offset the wire gives it, never moved to UTC.

    Two datetimes at one instant are equal whatever their offsets, so the
    comparison above cannot see this. The offset is what a reader takes the
    date from: 23:30 at UTC-7 on the 8th is the 8th to whoever said it, and the
    9th in UTC.
    """
    hit = Hit.from_json(
        {
            "value": "anna called late that night",
            "said": "2023-05-08T23:30:00-07:00",
            "occurred": [
                {
                    "start": "2023-05-01T00:00:00+02:00",
                    "end": "2023-06-01T00:00:00+02:00",
                }
            ],
            "score": 1.0,
        }
    )

    assert hit.said is not None
    assert hit.said.utcoffset() == timedelta(hours=-7)
    assert hit.said.date() == date(2023, 5, 8)
    assert hit.occurred is not None
    assert hit.occurred[0].start.utcoffset() == timedelta(hours=2)
    assert hit.occurred[0].end is not None
    assert hit.occurred[0].end.utcoffset() == timedelta(hours=2)


def test_a_hit_carries_the_times_its_fact_was_given():
    """``said`` parses into a datetime and ``occurred`` into its spans, in order.

    They are typed because a caller computes with them: sorting hits by when
    they happened, or testing a span against a day. ``timestamp`` stays the
    server's string, as it was before the fact had other times.
    """
    hit = Hit.from_json(
        {
            "value": "anna ran the charity race",
            "timestamp": "2026-01-02T03:04:05.062614Z",
            "said": "2023-05-08T09:30:00Z",
            "occurred": [
                {"start": "2023-05-07T00:00:00Z", "end": "2023-05-08T00:00:00Z"},
                {"start": "2023-05-20T00:00:00Z"},
            ],
            "score": 1.25,
        }
    )

    assert hit == Hit(
        value="anna ran the charity race",
        score=1.25,
        timestamp="2026-01-02T03:04:05.062614Z",
        said=datetime(2023, 5, 8, 9, 30, tzinfo=timezone.utc),
        occurred=[
            Interval(
                start=datetime(2023, 5, 7, tzinfo=timezone.utc),
                end=datetime(2023, 5, 8, tzinfo=timezone.utc),
            ),
            Interval(start=datetime(2023, 5, 20, tzinfo=timezone.utc), end=None),
        ],
    )


def test_a_hit_given_no_times_leaves_them_unset():
    """A hit that arrives without ``said`` or ``occurred`` holds ``None`` for both.

    The server omits a time the fact was not given, and a server that predates
    the two fields never sends them. Either way the hit parses. ``None`` reads
    as "not given", so a caller can tell a time a remember set from one it
    would have to assume.
    """
    hit = Hit.from_json(
        {
            "value": "polly whistles at dawn",
            "timestamp": "2026-01-02T03:04:05Z",
            "score": 1.0,
        }
    )

    assert hit.said is None
    assert hit.occurred is None
    assert hit.timestamp == "2026-01-02T03:04:05Z"


def test_a_hit_admitted_by_transmission_says_so(explain_response):
    """A hit that is no seed and has only a graph sighting was admitted by the graph.

    Explain exists to tell this case apart. Here the graph reached a fact that
    matched nothing itself, through the anchor the recall named. In the other
    case, the search never reached it at all. ``seed`` is what separates it
    from a fact that matched the text below the seed cut, which would carry a
    text sighting too.
    """
    result = RecallResult.from_json(explain_response["results"])

    admitted = result.hits[0]
    assert admitted.value == "the parrot sleeps in the kitchen"
    assert admitted.explain is not None
    assert admitted.explain.seed is False
    assert [c.source for c in admitted.explain.contributions] == ["graph"]
    assert admitted.explain.contributions[0].via == "polly"


def test_an_explained_result_carries_what_the_search_measured(explain_response):
    """The query's block parses whole: the rate, the bars, the seeds, terms, timings.

    The admission bars arrive keyed by depth as JSON strings and are read back
    as ints, so ``admission[1]`` is the depth-1 bar. The depth-1 bar is twice
    the background rate and the depth-2 bar is the rate itself. Timings stay
    integer nanoseconds. The term is the stem the text index read, not the word
    the query wrote. The recall has no when in a bound, so the server sends no
    ``when`` key, and its absence reads as empty.
    """
    result = RecallResult.from_json(explain_response["results"])

    assert result.explain == RecallExplain(
        background=0.30753008221644496,
        admission={1: 0.6150601644328899, 2: 0.30753008221644496},
        seeds={"anchor": 0, "graph": 0, "text": 2, "vector": 0},
        terms=["polli"],
        timings={"anchor": 0, "graph": 37958, "text": 24875, "vector": 0},
        when={},
    )


def test_an_anchor_seeded_recall_runs_no_round(anchor_explain_response):
    """A recall seeded by its anchor alone reports no bars and a zero rate.

    No anchor round ran, so the server omits the admission bars, and their
    absence reads as an empty mapping, not a failure. The background rate is
    sent at zero, so 0.0 is the server's own figure. Every hit is a seed, the
    anchor seeded every fact, and the recall had no terms to read.
    """
    result = RecallResult.from_json(anchor_explain_response["results"])

    assert result.explain == RecallExplain(
        background=0.0,
        admission={},
        seeds={"anchor": 3, "graph": 0, "text": 0, "vector": 0},
        terms=[],
        timings={"anchor": 2791, "graph": 0, "text": 0, "vector": 0},
        when={},
    )
    explained = [hit.explain for hit in result.hits]
    assert all(e is not None and e.seed for e in explained)
    assert {c.source for e in explained if e for c in e.contributions} == {"anchor"}


def test_a_hits_score_is_its_relevance_decayed(anchor_explain_response):
    """The decay a hit's block carries is the factor its score was scaled by.

    An anchor-seeded hit's relevance is the unit mass of its one sighting, so
    its score is its decay exactly. A decay read from the wrong key, or a score
    the server decayed twice, would break the equality, and a caller could not
    tell a fact that ranks low for its age from one that ranks low for its
    match.
    """
    result = RecallResult.from_json(anchor_explain_response["results"])

    assert result.hits
    for hit in result.hits:
        assert hit.explain is not None
        assert hit.score == hit.explain.decay


def test_a_bounding_when_rides_inside_the_querys_block(when_explain_response):
    """The when a bound was read from parses into a result of its own, explained too.

    The recall keeps what happened before the regatta, so its one hit is the
    river training, which carries the day it happened beside its block. Its
    ``until`` was read from the regatta, and the when's result, keyed by the
    bound that holds it, is a full result: its hit holds the times the
    regatta was given, its ``start`` and ``end`` are the day the regatta
    happened, the time the bound was cut at, and both its hit and the result
    carry blocks of their own. The when names no anchor, so no round ran for
    it. Its warnings ride on the recall's, so its own list is empty.
    """
    result = RecallResult.from_json(when_explain_response["results"])

    [hit] = result.hits
    assert hit.value == "anna trained on the river"
    assert hit.occurred == [
        Interval(
            start=datetime(2023, 5, 1, tzinfo=timezone.utc),
            end=datetime(2023, 5, 2, tzinfo=timezone.utc),
        )
    ]
    assert hit.explain is not None
    assert hit.explain.seed is True
    assert result.explain is not None
    assert result.explain.terms == ["train"]
    assert result.explain.when == {
        "until": RecallResult(
            count=1,
            hits=[
                Hit(
                    value="anna rowed in the regatta",
                    score=0.9798525093475564,
                    timestamp="2026-11-15T15:18:21.628696Z",
                    said=datetime(2023, 5, 8, tzinfo=timezone.utc),
                    occurred=[
                        Interval(
                            start=datetime(2023, 5, 7, tzinfo=timezone.utc),
                            end=datetime(2023, 5, 8, tzinfo=timezone.utc),
                        )
                    ],
                    explain=HitExplain(
                        seed=True,
                        decay=0.999999189512672,
                        contributions=[
                            Contribution(
                                source="text",
                                score=0.9798533035062422,
                                rank=0,
                                count=1,
                                held=1.0,
                            )
                        ],
                    ),
                )
            ],
            explain=RecallExplain(
                background=0.0,
                admission={},
                seeds={"anchor": 0, "graph": 0, "text": 1, "vector": 0},
                terms=["regatta"],
                timings={"anchor": 0, "graph": 0, "text": 17208, "vector": 0},
                when={},
            ),
            start=datetime(2023, 5, 7, tzinfo=timezone.utc),
            end=datetime(2023, 5, 8, tzinfo=timezone.utc),
        )
    }


def test_a_plain_recall_carries_no_breakdown():
    """Without the explain prefix there is no block, on the hits or the result.

    ``explain`` is ``None`` rather than an empty block, so a caller can tell a
    recall that was never explained from one whose search measured zeros. The
    server sends no ``explain`` key on a plain recall, and that absence is the
    only signal ``from_json`` reads.
    """
    result = RecallResult.from_json(
        {"count": 1, "hits": [{"value": "polly whistles at dawn", "score": 1.0}]}
    )

    assert result.hits[0].explain is None
    assert result.explain is None


def test_a_catalogue_entry_parses_its_count_and_times(describe_response):
    """An anchor's entry parses its count and a typed span on each clock.

    The diet fact has happened since 1 May with no end, and the server omits
    the end of a span that has not ended. ``None`` says so, as on a hit's
    ``occurred``. Its said and written spans are single moments, each ending
    where it starts.
    """
    entry = CatalogueEntry.from_json(
        describe_response["results"]["catalogue"]["topics"][0]
    )

    assert entry == CatalogueEntry(
        value="diet",
        facts=1,
        happened=Interval(start=datetime(2023, 5, 1, tzinfo=timezone.utc), end=None),
        said=Interval(
            start=datetime(2023, 5, 10, tzinfo=timezone.utc),
            end=datetime(2023, 5, 10, tzinfo=timezone.utc),
        ),
        written=Interval(
            start=datetime(2026, 11, 15, 13, 37, 6, 378198, tzinfo=timezone.utc),
            end=datetime(2026, 11, 15, 13, 37, 6, 378198, tzinfo=timezone.utc),
        ),
    )


def test_a_catalogue_of_no_fact_holds_no_times(warned_describe_response):
    """A describe that reached no fact parses with every time ``None``.

    The server leaves the spans out when the set holds no fact, since there is
    no time to report. ``None`` says so, and the anchor lists are empty lists,
    never ``None``, so a caller can iterate them unguarded.
    """
    catalogue = Catalogue.from_json(warned_describe_response["results"]["catalogue"])

    assert catalogue == Catalogue(
        graph=0, scope="ferry the", facts=0, anchors=0, topics=[], entities=[]
    )
    assert catalogue.happened is None
    assert catalogue.said is None
    assert catalogue.written is None


@pytest.mark.parametrize(
    "catalogue, want",
    [
        ({"facts": 0, "anchors": 0, "topics": [], "entities": []}, False),
        ({"facts": 1, "anchors": 0, "topics": [], "entities": []}, True),
        (
            {
                "facts": 2,
                "anchors": 1,
                "topics": [{"value": "birds", "facts": 2}],
                "entities": [],
            },
            True,
        ),
    ],
)
def test_a_catalogue_is_true_when_it_reached_a_fact(catalogue, want):
    """``bool(catalogue)`` reports whether any fact was reached, anchors or not.

    A graph of facts filed under nothing is still memory worth recalling from,
    so a describe that lists no anchor but counts a fact is true.
    """
    assert bool(Catalogue.from_json({"graph": 0, "scope": ""} | catalogue)) is want


def test_a_catalogue_of_a_204_names_the_graph_it_was_asked_about():
    """A 204 has no catalogue, so the graph comes from the caller and ``empty`` is set.

    The status line says the graph holds nothing, and the body that would name
    the graph is absent. The caller sent the query, so it passes the graph in,
    and the catalogue still says which graph is empty. Every count is zero and
    the warnings an empty list.
    """
    catalogue = Catalogue.from_json({}, graph=8, empty=True)

    assert catalogue == Catalogue(
        graph=8, scope="", facts=0, anchors=0, topics=[], entities=[], empty=True
    )
    assert catalogue.warnings == []
    assert bool(catalogue) is False


@pytest.mark.integration
def test_the_response_parses_into_the_declared_types(tide_result):
    """Every hit the server sent becomes a Hit with the declared field types."""
    assert isinstance(tide_result, RecallResult)
    assert tide_result.hits
    for hit in tide_result.hits:
        assert isinstance(hit, Hit)
        # from_json applies float() to `score`, so the score check below would
        # pass on a numeric string too. It pins the parsed type, not the wire
        # type.
        assert isinstance(hit.value, str)
        assert isinstance(hit.score, float)


@pytest.mark.integration
def test_the_server_count_agrees_with_the_hits_it_sent(tide_result):
    """The reported count matches the hits beside it.

    from_json uses the server's count when it sends one, and len(hits) only
    when it does not. So this compares the server's own count with its hits.
    """
    assert tide_result.count == len(tide_result.hits)


@pytest.mark.integration
def test_hits_arrive_ranked_by_descending_score(tide_result):
    """Hits come back in ranked order.

    RecallResult documents "in ranked order", and nothing in the SDK sorts. So
    this checks the server's order, which the SDK passes through unchanged.
    """
    scores = [hit.score for hit in tide_result]
    assert scores == sorted(scores, reverse=True)


@pytest.mark.integration
def test_timestamps_are_populated_and_rfc3339(tide_result):
    """Every hit carries a parseable RFC 3339 instant.

    ``timestamp`` is Optional on the dataclass, so every unit test would still
    pass if the server stopped sending it. This test notices. fromisoformat
    truncates the server's nanoseconds to microseconds rather than rejecting
    them, and it accepts the trailing Z. So parsing checks that the field is a
    real instant.
    """
    for hit in tide_result.hits:
        assert hit.timestamp is not None
        assert isinstance(datetime.fromisoformat(hit.timestamp), datetime)


@pytest.mark.integration
def test_len_and_iteration_agree_with_the_count(tide_result):
    """The container protocol RecallResult implements matches its own count."""
    assert len(tide_result) == tide_result.count
    assert len(list(tide_result)) == tide_result.count
    assert bool(tide_result) is True


@pytest.mark.integration
def test_an_empty_result_set_parses(client, models_graph, no_match):
    """A recall that matches nothing parses into an empty, falsey result."""
    empty = client.recall(no_match, graph=models_graph)
    assert empty.count == 0
    assert empty.hits == []
    assert len(empty) == 0
    assert list(empty) == []
    assert bool(empty) is False


@pytest.mark.integration
def test_a_warned_response_carries_the_warning_verbatim(tide_result):
    """A recall the server ran with a warning surfaces it as a list of strings.

    The fixture asks for depth:2 with no topic or entity. The server answers
    from the text and vector indices alone, and flags it. The exact text is
    pinned so a client can show it unchanged. The empty list of a clean
    response is a unit concern, covered by the ``from_json`` tests above.
    """
    assert tide_result.warnings == [
        "parse warning at column 19: depth:2 has no effect: the graph is searched "
        "only through a topic:/entity: anchor and this recall names none, so it "
        "runs on the text and vector indices alone"
    ]


@pytest.mark.integration
def test_a_vector_recall_parses_the_same_shape(vector_tide_result):
    """A vector-seeded result arrives in the envelope models.py knows to read.

    Vector-seeded results take a different path through the engine. So their
    shape is worth parsing apart from the text one.
    """
    assert isinstance(vector_tide_result, RecallResult)
    assert all(isinstance(hit.score, float) for hit in vector_tide_result)
    assert all(hit.timestamp for hit in vector_tide_result)


@pytest.mark.integration
def test_an_explained_response_parses_into_the_declared_types(explained_tide_result):
    """Both explain blocks a live server sends become the declared typed values.

    The unit tests parse recorded bodies, which stop proving anything the day
    the server's shape moves. Here the admission depths must come back as
    ints, the seed counts and timings as ints, keyed by every source, and each
    hit's decay as a factor in (0, 1] beside its contributions as typed
    sightings. The recall's bounds hold no when, so nothing rides under
    ``when``.
    """
    explain = explained_tide_result.explain
    assert isinstance(explain, RecallExplain)
    assert isinstance(explain.background, float)
    assert set(explain.admission) == {1, 2}
    assert all(isinstance(bar, float) for bar in explain.admission.values())
    sources = {"text", "vector", "anchor", "graph"}
    assert set(explain.seeds) == set(explain.timings) == sources
    assert all(isinstance(n, int) for n in explain.seeds.values())
    assert all(isinstance(ns, int) for ns in explain.timings.values())
    assert explain.terms == ["tide"]
    assert explain.when == {}
    for hit in explained_tide_result.hits:
        assert isinstance(hit.explain, HitExplain)
        assert isinstance(hit.explain.seed, bool)
        assert isinstance(hit.explain.decay, float)
        assert 0 < hit.explain.decay <= 1
        assert hit.explain.contributions
        assert all(isinstance(c, Contribution) for c in hit.explain.contributions)


@pytest.mark.integration
def test_hit_values_come_back_exactly_as_written(client, models_graph):
    """A stored value is returned byte for byte, not re-tokenised or trimmed."""
    stored = "the mudflats are exposed at low water"
    client.remember(stored, graph=models_graph)
    result = client.recall("mudflats", graph=models_graph, depth=0)
    assert stored in [hit.value for hit in result]


@pytest.mark.integration
def test_a_facts_times_come_back_as_they_were_given(client, models_graph):
    """The times a remember gives a fact are the times its hit holds.

    The said moment is 23:30 at UTC-7, and the span runs between two moments at
    UTC+2, with microseconds on its start. Each comes back as the same instant
    in the same offset, because the server keeps the offset a time was written
    with. A day comes back as the span from its midnight to the next, in UTC,
    where a date is read. The spans keep the order they were given in.
    """
    stored = "the keeper logged the eclipseprobe from the lighthouse"
    said = datetime(2023, 5, 8, 23, 30, tzinfo=timezone(timedelta(hours=-7)))
    span = Interval(
        start=datetime(
            2023, 5, 7, 18, 30, 0, 123456, tzinfo=timezone(timedelta(hours=2))
        ),
        end=datetime(2023, 5, 9, tzinfo=timezone(timedelta(hours=2))),
    )
    client.remember(
        stored, graph=models_graph, said=said, occurred=[date(2023, 5, 1), span]
    )

    result = client.recall("eclipseprobe", graph=models_graph, depth=0)

    [hit] = [hit for hit in result.hits if hit.value == stored]
    assert hit.said == said
    assert hit.occurred == [
        Interval(
            start=datetime(2023, 5, 1, tzinfo=timezone.utc),
            end=datetime(2023, 5, 2, tzinfo=timezone.utc),
        ),
        span,
    ]
    assert hit.said is not None
    assert hit.said.utcoffset() == timedelta(hours=-7)
    assert hit.occurred is not None
    assert hit.occurred[1].start.utcoffset() == timedelta(hours=2)


@pytest.mark.integration
def test_a_fact_given_no_times_holds_none(tide_result):
    """A fact its remember gave no times comes back with neither field set.

    The tide facts were written with a value and anchors only. The server sends
    no ``said`` and no ``occurred`` for them, where it could have sent a copy
    of the written time. So a caller can tell a time that was given from one
    that is assumed. ``timestamp`` is the one time every hit holds.
    """
    assert tide_result.hits
    for hit in tide_result.hits:
        assert hit.said is None
        assert hit.occurred is None
        assert hit.timestamp is not None


@pytest.mark.integration
def test_scores_are_raw_fused_quantities(tide_result):
    """Scores are positive and arrive raw, on the scorer's own scale.

    The default excess scorer adds a hit's seed mass and any surplus its
    anchors transmitted, in raw seed units. Nothing normalises the sum, so a
    score at or above 1.0 is ordinary. A score is not a similarity in [0, 1].
    Clients must treat scores as an ordering, not a probability.
    """
    assert all(hit.score > 0 for hit in tide_result)
