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

"""The line a recall tool prints for each fact it found.

Every integration prints one line per hit, best first: the dates the fact
holds, its value, then its relevance to three decimals. It is the line the MCP
bridge prints too (``pkg/mcp/render.go``). A model reads the same memory
through a tool here or through the bridge, so the line is written once for the
tools and changes together with the bridge's.
"""

from __future__ import annotations

from datetime import datetime, timedelta

from fraise_sdk.models import Hit, Interval


def render_hit(hit: Hit) -> str:
    """Render one hit as a line: the dates the fact holds, its value, its relevance.

    The dates are when the fact was said and when what it states happened, each
    read as a bound reads it. A fact given no ``occurred`` happened when it was
    said, and one given no ``said`` was said when it was written. A model
    answering "what did she do the day before she told me?" needs both, and
    neither is in the value. When the two read the same, the line carries that
    one date, so a fact given no times shows the day it was written::

        - [said 2023-05-08, happened 2023-05-07] anna ran the race (relevance 1.000)
        - [2023-05-08] anna is rowing again (relevance 0.412)

    Several spans are joined with ``and``. A hit that holds no time at all, as
    one built by hand may, prints without the bracket.

    Args:
        hit: the recalled fact.

    Returns:
        The line, with no newline at its end.
    """
    said = hit.said
    if said is None and hit.timestamp:
        said = datetime.fromisoformat(hit.timestamp)
    dates = said.date().isoformat() if said else ""
    happened = " and ".join(render_span(span) for span in hit.occurred or ())
    if happened and happened != dates:
        dates = (
            f"said {dates}, happened {happened}" if dates else f"happened {happened}"
        )
    lead = f"[{dates}] " if dates else ""
    return f"- {lead}{hit.value} (relevance {hit.score:.3f})"


def render_span(span: Interval) -> str:
    """Render a span in the widest calendar unit that loses none of it.

    The unit is a year, a month or a day, written once or as ``first..last``
    with both inside the span. So a time reads back as it was given: May 2023
    as ``2023-05``, a week as ``2023-05-10..2023-05-16``, and neither as the
    day it starts on. A span that has not ended is its first day and a trailing
    ``..``. A single moment is the day it falls on. The day is the finest unit
    a line carries, and the hit has the moment.

    Args:
        span: the span, as a hit holds it in ``occurred``.

    Returns:
        The span as text, in the time zone its edges carry.
    """
    start, end = span.start, span.end
    first = start.date().isoformat()
    if end is None:
        return f"{first}.."
    if end <= start:
        return first
    # End is outside the span, so its last unit is the one the moment before
    # End falls in.
    last = (end - timedelta(microseconds=1)).date().isoformat()
    edges = [start, end]
    months = [
        edge.replace(day=1, hour=0, minute=0, second=0, microsecond=0) for edge in edges
    ]
    years = [edge.replace(month=1) for edge in months]
    # A month is the first 7 characters of a day and a year the first 4. An
    # edge sits on a unit's boundary when it is that unit's first midnight,
    # and a span whose edges both do is whole units of it. A month comes
    # before a year, so the loop stops at the first unit an edge falls inside.
    width = len(first)
    for unit, boundaries in ((7, months), (4, years)):
        if boundaries != edges:
            break
        width = unit
    first, last = first[:width], last[:width]
    return first if first == last else f"{first}..{last}"
