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

"""The two tools (pkg/mcp/tools.go) end to end.

The daemon, the bridge and stdio are all real. What passes here is what an
agent's tool call does.
"""

from datetime import datetime

import pytest


def test_remember_then_recall_round_trips_a_fact(mcp):
    """A fact stored through the bridge is found through the bridge.

    Both faces of the result are pinned. One is the rendered text an agent
    reads. The other is the structured content a programmatic client parses:
    count, value, score, and a timestamp that is a real RFC 3339 instant.

    The fact was given no times, so the hit carries neither said nor occurred,
    and the line is dated the day the fact was written.
    """
    stored = mcp.call(
        "remember",
        {"query": "remember@1 'the kittiwake nests on sheer cliffs' topic:birds"},
    )
    assert not stored.get("isError", False), stored
    assert stored["content"][0]["text"] == "Remembered."

    found = mcp.call("recall", {"query": "recall@1 kittiwake"})
    assert not found.get("isError", False), found

    results = found["structuredContent"]["results"]
    assert results["count"] == 1
    hit = results["hits"][0]
    assert hit["value"] == "the kittiwake nests on sheer cliffs"
    assert hit["score"] > 0
    datetime.fromisoformat(hit["timestamp"])  # raises if not a real instant
    assert "said" not in hit
    assert "occurred" not in hit

    written = hit["timestamp"][:10]
    assert found["content"][0]["text"].startswith(
        f"- [{written}] the kittiwake nests on sheer cliffs (relevance "
    )


def test_a_dated_fact_comes_back_with_its_dates_on_both_faces(mcp):
    """A fact given said: and occurred: carries both, to a client and to a model.

    The structured hit holds the two times as the daemon stores them: said as
    an instant, occurred as the span of the day it names. The Go SDK checks the
    result against the tool's output schema before sending it, so their arrival
    also proves the schema declares them. The rendered line puts both dates in
    front of the fact, which is where a model reads when something happened.
    """
    stored = mcp.call(
        "remember",
        {
            "query": "remember@4 'the gannet colony was counted from the cliff' "
            "said:'2023-05-08' occurred:'2023-05-07'"
        },
    )
    assert not stored.get("isError", False), stored

    found = mcp.call("recall", {"query": "recall@4 gannet"})
    assert not found.get("isError", False), found

    hit = found["structuredContent"]["results"]["hits"][0]
    assert hit["said"] == "2023-05-08T00:00:00Z"
    assert hit["occurred"] == [
        {"start": "2023-05-07T00:00:00Z", "end": "2023-05-08T00:00:00Z"}
    ]
    assert found["content"][0]["text"].startswith(
        "- [said 2023-05-08, happened 2023-05-07] "
        "the gannet colony was counted from the cliff (relevance "
    )


def test_a_when_through_the_recall_tool_answers_with_its_time(mcp):
    """A when sent as the query comes back with the time of the fact it found.

    The recall tool forwards any read, so a when travels through it. Its time
    is on both faces: start and end beside the hits for a client, and a first
    line for the model, written the way the span was given. The fact is on the
    default graph, the one a when on its own reads.
    """
    stored = mcp.call(
        "remember",
        {
            "query": "remember 'the puffin census took three days' "
            "occurred:'2023-06-10'..'2023-06-12'"
        },
    )
    assert not stored.get("isError", False), stored

    found = mcp.call("recall", {"query": "when puffin"})
    assert not found.get("isError", False), found

    results = found["structuredContent"]["results"]
    assert results["count"] == 1
    assert results["start"] == "2023-06-10T00:00:00Z"
    assert results["end"] == "2023-06-13T00:00:00Z"
    assert found["content"][0]["text"].splitlines()[0] == "when: 2023-06-10..2023-06-12"


def test_a_bound_reads_the_time_another_fact_holds(mcp):
    """A when in a bound travels through the bridge and bounds the recall.

    Three facts on graph 5 happened a month apart. Bounding the recall by when
    the chicks hatched keeps only what happened before that day, and the when
    finds its fact on the recall's own graph, since graph 0 holds none of them.
    """
    for fact, day in (
        ("the guillemot eggs were laid", "2023-05-01"),
        ("the guillemot chicks hatched", "2023-06-01"),
        ("the guillemot chicks fledged", "2023-07-01"),
    ):
        stored = mcp.call(
            "remember", {"query": f"remember@5 '{fact}' occurred:'{day}'"}
        )
        assert not stored.get("isError", False), stored

    found = mcp.call("recall", {"query": "recall@5 guillemot until:(when hatched)"})
    assert not found.get("isError", False), found
    values = [hit["value"] for hit in found["structuredContent"]["results"]["hits"]]
    assert values == ["the guillemot eggs were laid"]


def test_a_when_that_matched_nothing_says_so_in_band(mcp):
    """A when with no fact to read answers with no time and a warning.

    It is not a tool error: the query ran. The model reads why it has no time
    in the text, and a client finds the same sentence under warnings.
    """
    stored = mcp.call("remember", {"query": "remember 'the skua patrols the shore'"})
    assert not stored.get("isError", False), stored

    found = mcp.call("recall", {"query": "when zzznomatchzzz"})
    assert not found.get("isError", False), found

    warning = "the when matched no fact, so it has no time to answer with"
    assert found["structuredContent"] == {
        "results": {"count": 0, "hits": []},
        "warnings": [warning],
    }
    assert found["content"][0]["text"] == (
        f"No stored facts matched.\nwarning: {warning}"
    )


def test_vector_parameters_bind_through_the_bridge(mcp):
    """Out-of-band vector bindings survive the stdio hop.

    A fact stored with vec:$v is recallable by vector seed alone. That works
    only if the parameters travelled beside both queries intact.
    """
    vector = [0.5, 0.25, 0.125, 0.0625]
    stored = mcp.call(
        "remember",
        {
            "query": "remember@2 'the foghorn sounds at one-minute intervals' vec:$v",
            "parameters": {"v": vector},
        },
    )
    assert not stored.get("isError", False), stored

    found = mcp.call(
        "recall", {"query": "recall@2 vec:$v", "parameters": {"v": vector}}
    )
    assert not found.get("isError", False), found
    values = [hit["value"] for hit in found["structuredContent"]["results"]["hits"]]
    assert "the foghorn sounds at one-minute intervals" in values


def test_daemon_rejections_surface_in_band(mcp):
    """A query the daemon rejects comes back as a tool error.

    The error carries the daemon's own message, the text a model needs to
    correct itself. It is never a protocol failure, which would end the
    conversation.
    """
    result = mcp.call("recall", {"query": "recall@1 zebras top:0"})
    assert result.get("isError") is True
    assert "top:0 out of range" in result["content"][0]["text"]


def test_a_recall_of_an_empty_graph_tells_the_model_to_write(mcp):
    """Graph 7 is never written to, so the daemon answers its recall with 204.

    Only the status carries the distinction, because a 204 has no body. A
    bridge that read the body alone would relay "no stored facts matched". It
    uses the same words for a query that missed facts that do exist. The model
    needs to tell them apart. The first calls for remembering something, the
    second for asking differently.

    The structured half still satisfies the recall tool's output schema. The
    schema requires hits to be an array, so a 204 must not leave it null.
    """
    result = mcp.call("recall", {"query": "recall@7 zebras"})
    assert not result.get("isError", False), result

    assert result["structuredContent"]["results"] == {"count": 0, "hits": []}
    assert result["content"][0]["text"] == (
        "No stored facts matched: this graph is empty, "
        "nothing has been remembered in it yet."
    )


def test_a_write_is_acknowledged_with_its_own_shape(mcp):
    """The daemon answers a write with a status token, not a recall envelope.

    The bridge's structured output follows it, so a programmatic client can
    tell an accepted write from a recall that matched nothing.
    """
    stored = mcp.call("remember", {"query": "remember@1 'the ack is its own shape'"})
    assert not stored.get("isError", False), stored

    assert stored["structuredContent"]["status"] == "ok"
    assert "results" not in stored["structuredContent"]
    assert stored["content"][0]["text"] == "Remembered."


def test_parse_warnings_ride_beside_the_results(mcp):
    """A query that runs with a parse warning delivers it on both faces.

    The warning is in the structured payload, and in the text rendered for the
    model.
    """
    result = mcp.call("recall", {"query": "recall@1 opera Since:7d"})
    assert not result.get("isError", False), result
    assert result["structuredContent"].get("warnings"), (
        "the keyword-shaped term must warn"
    )
    assert "warning:" in result["content"][0]["text"]


def test_an_unreachable_daemon_is_an_in_band_error(mcp_without_daemon):
    """With nothing listening, the bridge still converses.

    The handshake ran, as the fixture proves. A tool call fails in band. Its
    error names the address it tried and the service commands that start the
    daemon.
    """
    result = mcp_without_daemon.call("recall", {"query": "recall@1 anything"})
    assert result.get("isError") is True
    text = result["content"][0]["text"]
    assert "is the daemon running" in text


def test_anchor_only_recall_returns_what_is_filed_under_the_anchor(mcp):
    """Anchors alone seed the search with what is filed under them.

    So an agent can see what it knows about a topic before it knows what to
    search for. Two facts filed under one topic, and nothing else on graph 3,
    both come back from `recall@3 topic:cormorants`. Each is scored like any
    recall hit: the unit of mass its anchor lends, decayed by age.
    """
    facts = (
        "the cormorant dries its wings on the post",
        "the cormorant dives from the surface",
    )
    for phrase in facts:
        stored = mcp.call(
            "remember", {"query": f"remember@3 '{phrase}' topic:cormorants"}
        )
        assert not stored.get("isError", False), stored

    found = mcp.call("recall", {"query": "recall@3 topic:cormorants"})
    assert not found.get("isError", False), found
    hits = found["structuredContent"]["results"]["hits"]
    assert {hit["value"] for hit in hits} == set(facts)
    assert all(hit["score"] > 0 for hit in hits), hits


@pytest.mark.parametrize(
    ("text", "scope"),
    [
        ("describe recall@6 topic:seabirds", "topic:seabirds"),
        ("describe recall@6 entity:shearwater", "entity:shearwater"),
    ],
)
def test_a_describe_through_the_recall_tool_answers_with_its_catalogue(
    mcp, text, scope
):
    """A describe sent as the query comes back as a catalogue on both faces.

    The structured results hold the daemon's catalogue beside no hits, for a
    client. The text leads with the describe as the daemon read it and what it
    counted, then lists each kind's anchors with when their facts happened, for
    a model choosing what to recall next. The two facts are the only ones on
    graph 6 filed under anything, so the topic and the entity describe the
    same set, whatever else the graph holds.
    """
    for fact, day in (
        ("the shearwater skims the swell", "2023-08-01"),
        ("the shearwater returns to its burrow at night", "2023-08-03"),
    ):
        stored = mcp.call(
            "remember",
            {
                "query": f"remember@6 '{fact}' topic:seabirds entity:shearwater "
                f"occurred:'{day}'"
            },
        )
        assert not stored.get("isError", False), stored

    found = mcp.call("recall", {"query": text})
    assert not found.get("isError", False), found

    results = found["structuredContent"]["results"]
    assert (results["count"], results["hits"]) == (0, [])
    catalogue = results["catalogue"]
    happened = {"start": "2023-08-01T00:00:00Z", "end": "2023-08-04T00:00:00Z"}
    assert {key: catalogue[key] for key in ("graph", "scope", "facts", "anchors")} == {
        "graph": 6,
        "scope": scope,
        "facts": 2,
        "anchors": 2,
    }
    assert catalogue["happened"] == happened
    for kind, value in (("topics", "seabirds"), ("entities", "shearwater")):
        [entry] = catalogue[kind]
        assert (entry["value"], entry["facts"], entry["happened"]) == (
            value,
            2,
            happened,
        ), kind

    lines = found["content"][0]["text"].splitlines()
    assert (
        lines[0] == f"{text}: 2 facts under 2 anchors, happened 2023-08-01..2023-08-03"
    )
    assert lines[1:] == [
        "topics:",
        "- [2023-08-01..2023-08-03] seabirds: 2 facts",
        "entities:",
        "- [2023-08-01..2023-08-03] shearwater: 2 facts",
    ]


def test_an_explained_recall_carries_its_blocks_in_both_halves(mcp):
    """``explain recall`` through the bridge keeps the daemon's explain blocks.

    Each hit carries its seed flag, the recency factor its relevance was
    decayed by and the contributions its score was folded from, and the
    results carry the query's background rate, admission bars, seeds and
    timings per source and the terms the text index read. The output schema
    the SDK validates against admits them. The text the model reads carries
    the same account: a line for the query above the facts, then one under
    each fact. Many clients hand the model the text alone, and an explanation
    that stays in the structured half never reaches it. The same recall
    written plainly carries no explain key and renders the facts alone. The daemon runs on its defaults, so the depth is written out: the
    anchor round runs at depth 2 and both bars come back.
    """
    for phrase in (
        "the osprey plunges for mackerel",
        "the osprey nests on the bass rock",
    ):
        stored = mcp.call("remember", {"query": f"remember@4 '{phrase}' entity:osprey"})
        assert not stored.get("isError", False), stored

    found = mcp.call(
        "recall", {"query": "explain recall@4 mackerel entity:osprey depth:2"}
    )
    assert not found.get("isError", False), found

    results = found["structuredContent"]["results"]
    hits = {hit["value"]: hit for hit in results["hits"]}
    assert hits["the osprey plunges for mackerel"]["explain"]["seed"] is True
    for hit in results["hits"]:
        assert set(hit["explain"]) == {"seed", "decay", "contributions"}, hit
        assert 0 < hit["explain"]["decay"] <= 1, hit
        assert hit["explain"]["contributions"], hit
    explain = results["explain"]
    assert set(explain) == {"background", "admission", "seeds", "terms", "timings"}
    assert set(explain["admission"]) == {"1", "2"}
    sources = {"text", "vector", "graph", "anchor"}
    assert set(explain["seeds"]) == sources
    assert set(explain["timings"]) == sources
    assert explain["seeds"]["text"] >= 1
    assert explain["terms"]

    facts = [
        f"- [{hit['timestamp'][:10]}] {hit['value']} (relevance {hit['score']:.3f})"
        for hit in results["hits"]
    ]
    header, *lines = found["content"][0]["text"].split("\n")
    assert header.startswith("explain: background "), header
    assert lines[0::2] == facts
    for hit, line in zip(results["hits"], lines[1::2], strict=True):
        seed = "seed" if hit["explain"]["seed"] else "not a seed"
        assert line.startswith(f"    {seed} · decay "), line

    plain = mcp.call("recall", {"query": "recall@4 mackerel entity:osprey depth:2"})
    assert not plain.get("isError", False), plain
    assert "explain" not in plain["structuredContent"]["results"]
    assert all(
        "explain" not in hit for hit in plain["structuredContent"]["results"]["hits"]
    )
    assert plain["content"][0]["text"] == "\n".join(facts)


def test_an_explained_recall_carries_the_answer_its_bound_read(mcp):
    """An explained recall bounded by a when carries that when's answer.

    Three facts on graph 6 happened a month apart. The recall's explain block
    holds, under when, the result of the when its until: read: the fact it
    matched and the time it gave. The when explains itself with the recall,
    so its result and its hit carry their own blocks. The output schema the
    SDK validates against admits the nested result, which refers back to the
    results schema. The text's query line names the fact the bound was read
    from and the time it gave, which is how a model sees a window drawn from
    the wrong fact.
    """
    for fact, day in (
        ("the puffin burrows were dug", "2023-05-01"),
        ("the puffin eggs hatched", "2023-06-01"),
        ("the puffin chicks fledged", "2023-07-01"),
    ):
        stored = mcp.call(
            "remember", {"query": f"remember@6 '{fact}' occurred:'{day}'"}
        )
        assert not stored.get("isError", False), stored

    found = mcp.call(
        "recall", {"query": "explain recall@6 puffin until:(when hatched)"}
    )
    assert not found.get("isError", False), found

    results = found["structuredContent"]["results"]
    assert [hit["value"] for hit in results["hits"]] == ["the puffin burrows were dug"]
    until = results["explain"]["when"]["until"]
    assert "since" not in results["explain"]["when"]
    assert until["start"] == "2023-06-01T00:00:00Z"
    assert [hit["value"] for hit in until["hits"]] == ["the puffin eggs hatched"]
    assert set(until["hits"][0]["explain"]) == {"seed", "decay", "contributions"}
    assert set(until["explain"]) >= {"background", "seeds", "terms", "timings"}
    header = found["content"][0]["text"].splitlines()[0]
    assert header.endswith(
        '; until read from "the puffin eggs hatched" → 2023-06-01'
    ), header


def test_an_explained_recall_renders_every_contribution_it_carries(mcp):
    """Each fact's line under an explained recall spells its structured block.

    The line holds the seed flag, the decay and one item per contribution, in
    the order the structured half lists them, each led by its source and
    naming the anchor it came through. So the text says nothing the daemon did
    not report and drops nothing it did. The recall names its anchor beside a
    term, so the graph round runs and the line carries text and graph sightings
    both, and the query line carries the admission bars that round drew.
    """
    for phrase in (
        "the razorbill swims under the ledge",
        "the razorbill sleeps on the ledge at dusk",
    ):
        stored = mcp.call(
            "remember", {"query": f"remember@5 '{phrase}' topic:razorbills"}
        )
        assert not stored.get("isError", False), stored

    found = mcp.call(
        "recall", {"query": "explain recall@5 swims topic:razorbills depth:2"}
    )
    assert not found.get("isError", False), found

    hits = found["structuredContent"]["results"]["hits"]
    header, *lines = found["content"][0]["text"].split("\n")
    assert " admission " in header, header
    assert header.endswith("; terms swim"), header
    assert "timings" not in header, header
    assert len(lines) == 2 * len(hits), lines
    sources = set()
    for hit, line in zip(hits, lines[1::2], strict=True):
        seed, decay, *contributions = line.removeprefix("    ").split(" · ")
        assert seed == ("seed" if hit["explain"]["seed"] else "not a seed"), line
        assert decay.startswith("decay "), line
        assert len(contributions) == len(hit["explain"]["contributions"]), line
        for item, block in zip(
            contributions, hit["explain"]["contributions"], strict=True
        ):
            assert item.startswith(f"{block['source']} "), item
            if "via" in block:
                assert f" via {block['via']} (" in item, item
            sources.add(block["source"])
    assert sources == {"text", "graph"}


def test_an_explained_anchor_recall_names_each_anchor_a_fact_is_filed_under(mcp):
    """Anchors seeding alone read as one unit sighting per named anchor.

    A fact filed under both named anchors carries two sightings, and each
    names its anchor and how many facts that anchor files, so a model sees why
    it outranks a fact filed under one. No anchor round runs when the anchors
    seed alone, so the query line carries no admission bar.
    """
    stored = mcp.call(
        "remember",
        {
            "query": "remember@5 'the gannet colony circles the stack' "
            "topic:gannets entity:'bass rock'"
        },
    )
    assert not stored.get("isError", False), stored
    stored = mcp.call(
        "remember",
        {"query": "remember@5 'the gannet folds its wings to dive' topic:gannets"},
    )
    assert not stored.get("isError", False), stored

    found = mcp.call(
        "recall", {"query": "explain recall@5 topic:gannets entity:'bass rock'"}
    )
    assert not found.get("isError", False), found

    header, *lines = found["content"][0]["text"].split("\n")
    assert header.startswith("explain: background 0; seeds anchor 2, "), header
    assert "admission" not in header, header
    assert lines[0].startswith("- ["), lines
    assert lines[0].endswith("] the gannet colony circles the stack (relevance 2.000)")
    assert sorted(lines[1].split(" · ")[2:]) == [
        "anchor 1.000 via bass rock (1 fact)",
        "anchor 1.000 via gannets (2 facts)",
    ]
    assert lines[2].endswith("] the gannet folds its wings to dive (relevance 1.000)")
    assert lines[3].split(" · ")[2:] == ["anchor 1.000 via gannets (2 facts)"]


def test_an_explained_bound_that_matched_nothing_says_so(mcp):
    """A when in a bound that matched no fact is named on the query line.

    The recall then has no window to read and returns no hits, and the model
    must see that the bound is why, not the terms: the query line says the
    bound was read from no fact, the hits say none matched, and the daemon's
    warning follows.
    """
    stored = mcp.call(
        "remember", {"query": "remember@5 'the kittiwake nests on the cliff'"}
    )
    assert not stored.get("isError", False), stored

    found = mcp.call(
        "recall", {"query": "explain recall@5 kittiwake until:(when zzznomatchzzz)"}
    )
    assert not found.get("isError", False), found

    header, empty, *warnings = found["content"][0]["text"].split("\n")
    assert header.endswith("; until read from no fact"), header
    assert empty == "No stored facts matched."
    assert warnings, found["content"][0]["text"]
    assert all(line.startswith("warning: ") for line in warnings), warnings
