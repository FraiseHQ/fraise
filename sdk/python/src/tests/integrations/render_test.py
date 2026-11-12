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

"""Tests for the line a recall tool prints for a fact.

Each hit and span is given as the server sends it and parsed the way the client
parses it, so a case reads as the wire form beside the line it prints. The
cases are the ones the MCP bridge pins for its own line
(``pkg/mcp/render_test.go``). A model reads the same memory through a tool or
through the bridge, so a line that changes here changes there.
"""

import pytest
from fraise_sdk.integrations.render import render_hit, render_span
from fraise_sdk.models import Hit, Interval


@pytest.mark.parametrize(
    "hit, line",
    [
        (
            {
                "value": "the barometer falls before the storm",
                "timestamp": "2026-01-02T03:04:05Z",
                "score": 1.25,
            },
            "- [2026-01-02] the barometer falls before the storm (relevance 1.250)",
        ),
        (
            {
                "value": "anna ran the charity race",
                "timestamp": "2026-01-02T03:04:05Z",
                "said": "2023-05-08T00:00:00Z",
                "occurred": [
                    {"start": "2023-05-07T00:00:00Z", "end": "2023-05-08T00:00:00Z"}
                ],
                "score": 1,
            },
            "- [said 2023-05-08, happened 2023-05-07] anna ran the charity race (relevance 1.000)",
        ),
        (
            {
                "value": "anna ran the charity race",
                "timestamp": "2026-01-02T03:04:05Z",
                "said": "2023-05-08T00:00:00Z",
                "score": 1,
            },
            "- [2023-05-08] anna ran the charity race (relevance 1.000)",
        ),
        (
            {
                "value": "anna ran the charity race",
                "timestamp": "2026-01-02T03:04:05Z",
                "said": "2023-05-08T00:00:00Z",
                "occurred": [
                    {"start": "2023-05-08T00:00:00Z", "end": "2023-05-09T00:00:00Z"}
                ],
                "score": 1,
            },
            "- [2023-05-08] anna ran the charity race (relevance 1.000)",
        ),
        (
            {
                "value": "anna ran the charity race",
                "timestamp": "2026-01-02T03:04:05Z",
                "occurred": [
                    {"start": "2023-05-07T00:00:00Z", "end": "2023-05-08T00:00:00Z"}
                ],
                "score": 1,
            },
            "- [said 2026-01-02, happened 2023-05-07] anna ran the charity race (relevance 1.000)",
        ),
        (
            {
                "value": "anna ran the harbour drills",
                "timestamp": "2026-01-02T03:04:05Z",
                "said": "2023-06-02T00:00:00Z",
                "occurred": [
                    {"start": "2023-05-01T00:00:00Z", "end": "2023-05-02T00:00:00Z"},
                    {"start": "2023-05-28T00:00:00Z", "end": "2023-05-29T00:00:00Z"},
                ],
                "score": 1,
            },
            "- [said 2023-06-02, happened 2023-05-01 and 2023-05-28] anna ran the harbour drills (relevance 1.000)",
        ),
        (
            {
                "value": "anna called late that night",
                "timestamp": "2026-01-02T03:04:05Z",
                "said": "2023-05-08T23:30:00-07:00",
                "score": 1,
            },
            "- [2023-05-08] anna called late that night (relevance 1.000)",
        ),
        (
            {
                "value": "anna trained for the race",
                "timestamp": "2026-01-02T03:04:05.062614+01:00",
                "occurred": [
                    {"start": "2023-05-01T00:00:00Z", "end": "2023-06-01T00:00:00Z"}
                ],
                "score": 0.4124,
            },
            "- [said 2026-01-02, happened 2023-05] anna trained for the race (relevance 0.412)",
        ),
    ],
)
def test_a_hit_prints_the_dates_its_fact_holds(hit, line):
    """The line leads with when the fact was said and when it happened.

    Both are read as a bound reads them. A fact given no ``occurred`` happened
    when it was said, and one given no ``said`` was said when it was written.
    So the line names the two only where they differ, and a fact given no time
    at all shows the day it was written. A date is read in the offset it was
    written with: 23:30 at UTC-7 on the 8th stays the 8th, though it is the 9th
    in UTC. Without the dates a model cannot answer "what did she do the day
    before she told me?", because neither is in the value.
    """
    assert render_hit(Hit.from_json(hit)) == line


def test_a_hit_that_holds_no_time_prints_without_the_bracket():
    """A hit built by hand, with no time at all, prints its value and relevance.

    The server stamps every hit it returns. A hit a caller builds for a tool of
    their own may hold nothing, and an empty bracket would read as a missing
    date rather than as no date.
    """
    hit = Hit(value="the sky is blue", score=0.9)
    assert render_hit(hit) == "- the sky is blue (relevance 0.900)"


def test_a_hit_that_holds_only_when_it_happened_names_that_alone():
    """With no day it was said or written, the line carries the happened time alone."""
    hit = Hit.from_json(
        {
            "value": "anna ran the charity race",
            "occurred": [
                {"start": "2023-05-07T00:00:00Z", "end": "2023-05-08T00:00:00Z"}
            ],
            "score": 1,
        }
    )
    assert render_hit(hit) == (
        "- [happened 2023-05-07] anna ran the charity race (relevance 1.000)"
    )


@pytest.mark.parametrize(
    "span, rendered",
    [
        (
            {"start": "2023-05-07T00:00:00Z", "end": "2023-05-08T00:00:00Z"},
            "2023-05-07",
        ),
        ({"start": "2023-05-01T00:00:00Z", "end": "2023-06-01T00:00:00Z"}, "2023-05"),
        ({"start": "2023-01-01T00:00:00Z", "end": "2024-01-01T00:00:00Z"}, "2023"),
        (
            {"start": "2023-05-10T00:00:00Z", "end": "2023-05-17T00:00:00Z"},
            "2023-05-10..2023-05-16",
        ),
        (
            {"start": "2023-05-01T00:00:00Z", "end": "2023-07-01T00:00:00Z"},
            "2023-05..2023-06",
        ),
        (
            {"start": "2022-01-01T00:00:00Z", "end": "2024-01-01T00:00:00Z"},
            "2022..2023",
        ),
        (
            {"start": "2022-11-01T00:00:00Z", "end": "2023-02-01T00:00:00Z"},
            "2022-11..2023-01",
        ),
        (
            {"start": "2023-05-01T00:00:00Z", "end": "2023-05-17T00:00:00Z"},
            "2023-05-01..2023-05-16",
        ),
        ({"start": "2023-05-20T00:00:00Z"}, "2023-05-20.."),
        (
            {"start": "2023-05-07T18:30:00Z", "end": "2023-05-07T18:30:00Z"},
            "2023-05-07",
        ),
        (
            {"start": "2023-05-07T09:00:00Z", "end": "2023-05-07T18:30:00Z"},
            "2023-05-07",
        ),
        (
            {"start": "2023-05-07T18:30:00Z", "end": "2023-06-01T00:00:00Z"},
            "2023-05-07..2023-05-31",
        ),
        (
            {"start": "2023-05-01T00:00:00+02:00", "end": "2023-06-01T00:00:00+02:00"},
            "2023-05",
        ),
        (
            {"start": "2023-04-30T22:00:00Z", "end": "2023-05-31T22:00:00Z"},
            "2023-04-30..2023-05-31",
        ),
    ],
)
def test_a_span_prints_in_the_widest_unit_that_loses_none_of_it(span, rendered):
    """A span reads back in the unit it was written in: a year, a month or a day.

    If a month printed as its first day, a model would answer "on the first of
    May" to a fact that only says "in May". The unit is the widest one both
    edges sit on the boundary of, so a span that starts or ends inside a month
    reads in days. The last unit named is inside the span: a week that ends
    before the 17th is ``..2023-05-16``. A span that has not ended trails off
    after its first day, and a single moment is the day it falls on. The edges
    are judged in the offset they carry, so May at UTC+2 is ``2023-05`` and the
    same moments written in UTC are a range of days.
    """
    assert render_span(Interval.from_json(span)) == rendered
