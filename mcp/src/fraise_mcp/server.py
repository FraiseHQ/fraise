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
"""The MCP server: the recall and remember tools over a Fraise client.

Each tool takes one FQL statement and sends it to the daemon through
:meth:`fraise_sdk.client.FraiseClient.query`. The daemon parses, plans and
runs it. When the statement takes a vector (see
:func:`fraise_mcp.statement.embedding_text`), the server encodes its text,
appends ``vec:$v`` and binds the vector out of band. So the tools carry
hybrid retrieval without the model ever writing a vector. The daemon's error
messages come back to the model as tool errors, so it can read them and
correct its statement.

A recall answers twice, as the built-in ``fraise mcp`` bridge does: rendered
to text for the model, and as structured content, the daemon's response under
the output schema below. The structured content is where the ``explain``
blocks of an ``explain recall`` ride, which the text leaves out.
"""

from __future__ import annotations

from datetime import datetime
from typing import Annotated, NotRequired, TypedDict

from fraise_sdk import models
from fraise_sdk.client import FraiseClient
from fraise_sdk.constants import VECTOR_PARAM
from fraise_sdk.errors import FraiseAPIError, FraiseError
from fraise_sdk.integrations.render import render_span
from fraise_sdk.models import RecallResult
from fraise_sdk.providers.base import EmbedderLike
from mcp.server.mcpserver import MCPServer
from mcp.server.mcpserver.exceptions import ToolError
from mcp.types import CallToolResult, TextContent
from pydantic import Field

from fraise_mcp.statement import embedding_text, with_vector

RECALL_DESCRIPTION = (
    "Search the agent's long-term memory: run an FQL recall against the fraise "
    "daemon and return the matching facts ranked by relevance and recency. "
    "Anchors alone — topic:/entity: with no search terms — seed the search with "
    "every fact filed under them, scored one unit per named anchor and decayed by "
    "age, newest first under a single anchor: the way to see what is stored under "
    "a topic before you know what to search for. A recall may also be written "
    "explain recall …: it ranks exactly as the recall it prefixes, and its "
    "structured result also says how the ranking was reached. This tool also "
    "runs describe: put describe in front of a recall and it answers with the "
    "shape of what the recall reaches instead of the facts — how many, when "
    "they happened, and the topics and entities they are filed under, with how "
    "many facts each files — and describe recall alone describes the whole "
    "graph. When you do not know how memory is filed, describe recall first."
)

RECALL_QUERY = (
    "A full FQL recall: search terms, then optional topic:/entity: anchors (beside "
    "terms they lift the facts filed under them through the graph and filter "
    "nothing, so the recall runs at depth:1 at least), top:N result cap, depth:0-2 "
    "retrieval lane (the graph is searched only beside a topic:/entity:), "
    "since:/until: time bounds (a date is quoted: since:'2026-01-15'). A bare term "
    "or value is letters and digits only; quote anything else: 'e-mail', "
    "topic:'machine-learning'. A search term that spells a keyword (top, since, "
    "topic...) is quoted too: 'since'. Example: recall@2 barometer storm "
    "topic:weather top:5 depth:1 since:7d. Anchors alone, with no terms, seed the "
    "search with every fact filed under those topics/entities, scored one unit per "
    "named anchor and decayed by age, so newest first under one anchor (depth: has "
    "no effect). Example: recall@2 topic:weather top:20. describe in front of a "
    "recall, which this tool runs as it runs a recall, answers with the catalogue "
    "of what the recall reaches instead of its facts: how many facts, when they happened, and the topics and entities they "
    "are filed under with each one's count. describe recall@2 describes the whole "
    "graph, describe recall@2 topic:weather what is filed under an anchor and the "
    "anchors filed beside it, and describe recall@2 barometer the anchors the "
    "matching facts are filed under. Under describe, top:N caps the topics and the "
    "entities listed, and since:/until: bound the facts counted."
)

REMEMBER_DESCRIPTION = (
    "Store a self-contained fact in the agent's long-term memory: run an FQL "
    "remember against the fraise daemon."
)

REMEMBER_QUERY = (
    "A full FQL remember: the fact in single quotes, anchors as repeatable "
    "topic:/entity: pairs, @N selecting the graph. An anchor value that is not "
    "letters and digits only is quoted too: topic:'machine-learning'. Example: "
    "remember@2 'the barometer falls before the storm' topic:weather entity:harbour"
)


class Interval(TypedDict):
    """A span that holds start and excludes end; a single moment has the two equal."""

    start: Annotated[datetime, Field(description="Where the span starts.")]
    end: NotRequired[
        Annotated[
            datetime,
            Field(
                description="Where the span ends, itself excluded: equal to start "
                "for a single moment, absent for a span that has not ended."
            ),
        ]
    ]


class Contribution(TypedDict):
    """One source's sighting of a fact, with that source's raw mass."""

    source: Annotated[
        str,
        Field(
            description="The source that saw the fact: text, vector, graph or anchor."
        ),
    ]
    score: Annotated[
        float,
        Field(
            description="That source's raw mass: the match mass for text, the "
            "similarity for vector, the funding anchor's whole observed mass for "
            "graph, one unit for anchor."
        ),
    ]
    rank: Annotated[
        int, Field(description="The fact's place in the source's ranking, 0 first.")
    ]
    via: NotRequired[
        Annotated[
            str,
            Field(
                description="The topic or entity a graph or anchor sighting came "
                "through."
            ),
        ]
    ]
    degree: NotRequired[
        Annotated[int, Field(description="How many facts are filed under that anchor.")]
    ]
    count: Annotated[int, Field(description="How many seeds funded the sighting.")]
    named: NotRequired[
        Annotated[
            bool,
            Field(
                description="True when the statement named the anchor, which then "
                "passes its mass on without a fair share."
            ),
        ]
    ]
    held: NotRequired[
        Annotated[
            float,
            Field(
                description="On a text sighting, the share of the query's "
                "information the fact's own words hold."
            ),
        ]
    ]


class HitExplain(TypedDict):
    """Present only on an explained recall: how this fact reached the ranking."""

    seed: Annotated[
        bool,
        Field(
            description="Whether the fact seeded the recall: a top text or vector "
            "match, or a member of a named anchor when anchors alone seed. A fact "
            "that is no seed and has graph contributions alone came in through an "
            "anchor the seeds funded."
        ),
    ]
    decay: Annotated[
        float,
        Field(
            description="The recency factor the fact's relevance was scaled by to "
            "give its score, in (0, 1]: it tells a fact ranked low for its age from "
            "one ranked low for its match."
        ),
    ]
    contributions: Annotated[
        list[Contribution],
        Field(
            description="One entry per sighting of the fact, the ingredients its "
            "score was folded from."
        ),
    ]


class Hit(TypedDict):
    """A recalled fact: its value, its times, its score and, explained, its block."""

    value: Annotated[str, Field(description="The remembered fact, exactly as stored.")]
    timestamp: Annotated[datetime, Field(description="When the fact was written.")]
    said: NotRequired[
        Annotated[
            datetime,
            Field(
                description="When the fact was said, as its remember gave it with "
                "said:. Absent when it gave none: the fact was then said when it "
                "was written."
            ),
        ]
    ]
    occurred: NotRequired[
        Annotated[
            list[Interval],
            Field(
                description="Each span over which what the fact states happened, as "
                "its remember gave them with occurred:. Absent when it gave none: "
                "the fact then happened when it was said."
            ),
        ]
    ]
    score: Annotated[
        float,
        Field(
            description="Raw relevance in the scorer's own units: an ordering, not "
            "a probability, with no upper bound. When anchors alone seed the search "
            "it is one unit per named anchor the fact is filed under, decayed by age."
        ),
    ]
    explain: NotRequired[HitExplain]


class NestedResults(TypedDict):
    """The results each when in a since:/until: bound answered, under that bound."""

    since: NotRequired[Results]
    until: NotRequired[Results]


class RecallExplain(TypedDict):
    """Present only on an explained recall: what the search measured about the query as a whole."""

    background: Annotated[
        float,
        Field(
            description="The background rate: seed mass per unit of anchor degree "
            "over every anchor the round touched, 0 when no round ran."
        ),
    ]
    admission: NotRequired[
        Annotated[
            dict[str, float],
            Field(
                description="Per depth, 1 and 2, the bar per unit of degree an "
                "anchor the statement did not name had to exceed to pass its mass "
                "on. Absent when no anchor round ran."
            ),
        ]
    ]
    seeds: Annotated[
        dict[str, int],
        Field(
            description="How many distinct facts each source (text, vector, anchor, "
            "graph) seeded."
        ),
    ]
    terms: Annotated[
        list[str],
        Field(
            description="The query's terms as the text index read them, after "
            "stop-word removal and stemming, including terms no fact holds."
        ),
    ]
    timings: Annotated[
        dict[str, int],
        Field(description="Wall time each source took, in nanoseconds."),
    ]
    when: NotRequired[
        Annotated[
            NestedResults,
            Field(
                description="Present only when a since:/until: bound holds a when: "
                "the results that when answered, under the bound that holds it, "
                "showing which fact the bound was read from and the time it gave."
            ),
        ]
    ]


class CatalogueEntry(TypedDict):
    """One topic or entity the described facts are filed under."""

    value: Annotated[str, Field(description="The topic or entity, as stored.")]
    facts: Annotated[
        int,
        Field(
            description="How many of the described facts are filed under it. Under "
            "a describe of one anchor, the facts it shares with that anchor."
        ),
    ]
    happened: NotRequired[
        Annotated[
            Interval,
            Field(
                description="From the earliest start to the latest end of what the "
                "facts state took place; a fact given no occurred: happened when it "
                "was said. No end when one of them has not ended."
            ),
        ]
    ]
    said: NotRequired[
        Annotated[
            Interval,
            Field(
                description="From the first fact said to the last; a fact given no "
                "said: was said when it was written."
            ),
        ]
    ]
    written: NotRequired[
        Annotated[
            Interval, Field(description="From the first fact written to the last.")
        ]
    ]


class Catalogue(TypedDict):
    """What a describe answers with in place of hits: the shape of every fact its recall reaches."""

    graph: Annotated[int, Field(description="The graph described.")]
    scope: Annotated[
        str,
        Field(
            description="The recall's body as the server read it, empty for the "
            "whole graph."
        ),
    ]
    facts: Annotated[
        int,
        Field(
            description="How many facts the recall reaches: every candidate it would "
            "rank, not the first top."
        ),
    ]
    anchors: Annotated[
        int,
        Field(
            description="How many topics and entities those facts are filed under, "
            "before the lists are capped."
        ),
    ]
    happened: NotRequired[
        Annotated[
            Interval,
            Field(
                description="From the earliest start to the latest end of what the "
                "facts state took place; a fact given no occurred: happened when it "
                "was said. No end when one of them has not ended."
            ),
        ]
    ]
    said: NotRequired[
        Annotated[
            Interval,
            Field(
                description="From the first fact said to the last; a fact given no "
                "said: was said when it was written."
            ),
        ]
    ]
    written: NotRequired[
        Annotated[
            Interval, Field(description="From the first fact written to the last.")
        ]
    ]
    topics: Annotated[
        list[CatalogueEntry],
        Field(
            description="The topics the facts are filed under, the most filed first, "
            "at most top: of them."
        ),
    ]
    entities: Annotated[
        list[CatalogueEntry],
        Field(
            description="The entities the facts are filed under, the most filed "
            "first, at most top: of them."
        ),
    ]


class Results(TypedDict):
    """Ranked recall results, best first.

    With anchors alone, each hit scores one unit per named anchor it is filed
    under, decayed by age: newest first under a single anchor.
    """

    count: Annotated[
        int, Field(description="Number of hits returned, after the top: cap.")
    ]
    hits: list[Hit]
    start: NotRequired[
        Annotated[
            datetime,
            Field(
                description="Where the time a when answered with starts. Only a when "
                "that matched a fact carries it."
            ),
        ]
    ]
    end: NotRequired[
        Annotated[
            datetime,
            Field(
                description="Where that time ends, itself excluded: equal to start "
                "for a single moment, absent for a time that has not ended."
            ),
        ]
    ]
    explain: NotRequired[RecallExplain]
    catalogue: NotRequired[Catalogue]


# The output schema mirrors the query endpoint's read response, as the
# built-in bridge's does, so a client reads the same structure from either
# server. Each class's docstring is the description the schema publishes.
class RecallOutput(TypedDict):
    """The daemon's answer to the recall, with any warnings it attached."""

    results: Results
    warnings: NotRequired[
        Annotated[
            list[str],
            Field(
                description="Warnings the server attached; the query still ran and "
                "the results stand."
            ),
        ]
    ]


def render(body: dict) -> str:
    """Render a query response for the model.

    A write is acknowledged with a status token, so its confirmation is fixed
    text. A recall is one line per hit, best first, relevance to three
    decimals, in the voice of the SDK's framework tools. A recall can come back
    with nothing in two ways. Either the graph holds nothing at all, which the
    daemon answers 204 with no body, or a populated graph has no matching
    fact. They read differently on purpose: the first calls for remembering
    something, the second for asking differently. A describe answers with a
    catalogue in place of hits, rendered as :func:`render_catalogue` renders
    it. Warnings follow, one per line. The statement ran, and the model needs
    them to fix its next one.

    Args:
        body: the decoded response body, ``{}`` for a 204.

    Returns:
        The text the model reads.
    """
    catalogue = (body.get("results") or {}).get("catalogue")
    if "status" in body:
        lines = ["Remembered."]
    elif catalogue is not None:
        lines = render_catalogue(models.Catalogue.from_json(catalogue))
    else:
        result = RecallResult.from_json(
            body.get("results") or {}, empty="results" not in body
        )
        lines = [f"- {hit.value} (relevance {hit.score:.3f})" for hit in result]
        if not result and result.empty:
            lines = [
                "No stored facts matched: this graph is empty, nothing has been "
                "remembered in it yet."
            ]
        elif not result:
            lines = ["No stored facts matched."]
    lines += [f"warning: {warning}" for warning in body.get("warnings") or []]
    return "\n".join(lines)


def render_catalogue(catalogue: models.Catalogue) -> list[str]:
    """Render a describe's catalogue as the lines the model reads.

    The first line spells the describe as the server read it, so the model can
    widen or narrow it by editing what it reads, then gives how many facts the
    recall reaches, the anchors they are filed under and when they happened.
    The topics and then the entities follow, one per line in the order the
    catalogue lists them, each with when its facts happened in front, as the
    Go bridge renders them. A kind with no anchor says so, since a missing
    section reads as a list cut off.

    Args:
        catalogue: the describe's answer.

    Returns:
        The lines, with no newline at their ends.
    """
    statement = f"describe recall@{catalogue.graph}"
    if catalogue.scope:
        statement += f" {catalogue.scope}"
    if not catalogue or catalogue.happened is None:
        return [f"{statement}: no fact reached."]
    facts = "1 fact" if catalogue.facts == 1 else f"{catalogue.facts} facts"
    anchors = "1 anchor" if catalogue.anchors == 1 else f"{catalogue.anchors} anchors"
    lines = [
        f"{statement}: {facts} under {anchors}, happened {render_span(catalogue.happened)}"
    ]
    for kind, entries in (
        ("topics", catalogue.topics),
        ("entities", catalogue.entities),
    ):
        if not entries:
            lines.append(f"{kind}: none")
            continue
        lines.append(f"{kind}:")
        for entry in entries:
            span = f"[{render_span(entry.happened)}] " if entry.happened else ""
            count = "1 fact" if entry.facts == 1 else f"{entry.facts} facts"
            lines.append(f"- {span}{entry.value}: {count}")
    return lines


def build_server(
    client: FraiseClient, embedder: EmbedderLike | None, *, version: str = ""
) -> MCPServer:
    """Build the MCP server and register its recall and remember tools.

    Args:
        client: the client the statements are sent through.
        embedder: what encodes a statement's text into its vector, or ``None``
            to send every statement on text alone.
        version: the version the server reports to its MCP client.

    Returns:
        The server, ready to run over stdio.
    """
    server = MCPServer(name="fraise", version=version)

    def run(statement: str) -> dict:
        text = embedding_text(statement) if embedder is not None else None
        parameters = None
        if embedder is not None and text is not None:
            try:
                vector = [float(x) for x in embedder(text)]
            except Exception as exc:
                # The model loads, and downloads the first time, on this call.
                # Whatever stops it is the operator's to fix, so it is said in
                # words rather than as an anonymous crash.
                raise ToolError(f"could not embed the statement: {exc}") from exc
            parameters = {VECTOR_PARAM: vector}
            statement = with_vector(statement)
        try:
            return client.query(statement, parameters=parameters)
        except FraiseAPIError as exc:
            raise ToolError(exc.message) from exc
        except FraiseError as exc:
            raise ToolError(str(exc)) from exc

    def recall(
        query: Annotated[str, Field(description=RECALL_QUERY)],
    ) -> Annotated[CallToolResult, RecallOutput]:
        body = run(query)
        # The schema requires results. A 204, the answer for a graph that holds
        # nothing, has no body, and a write sent here answers with a status
        # alone, so both read as the empty result: the text says which it was.
        output: dict = {"results": body.get("results") or {"count": 0, "hits": []}}
        if body.get("warnings"):
            output["warnings"] = body["warnings"]
        return CallToolResult(
            content=[TextContent(type="text", text=render(body))],
            structured_content=output,
        )

    def remember(query: Annotated[str, Field(description=REMEMBER_QUERY)]) -> str:
        return render(run(query))

    server.add_tool(recall, description=RECALL_DESCRIPTION, structured_output=True)
    server.add_tool(remember, description=REMEMBER_DESCRIPTION, structured_output=False)
    return server
