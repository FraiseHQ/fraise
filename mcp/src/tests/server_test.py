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
"""The recall and remember tools over a mocked client, called through the MCP client."""

import pytest
from fraise_mcp.server import render
from fraise_sdk.errors import FraiseAPIError, FraiseError


@pytest.mark.parametrize(
    "body, want",
    [
        ({"status": "ok"}, "Remembered."),
        (
            {"status": "ok", "warnings": ["anchor 'x' repeated"]},
            "Remembered.\nwarning: anchor 'x' repeated",
        ),
        (
            {
                "results": {
                    "count": 2,
                    "hits": [
                        {
                            "value": "a",
                            "timestamp": "2026-10-01T09:00:00Z",
                            "score": 1.23456,
                        },
                        {
                            "value": "b",
                            "timestamp": "2026-09-30T09:00:00Z",
                            "score": 0.5,
                        },
                    ],
                }
            },
            "- a (relevance 1.235)\n- b (relevance 0.500)",
        ),
        ({"results": {"count": 0, "hits": []}}, "No stored facts matched."),
        (
            {},
            "No stored facts matched: this graph is empty, nothing has been "
            "remembered in it yet.",
        ),
        (
            {"results": {"count": 0, "hits": []}, "warnings": ["'the' is a stop word"]},
            "No stored facts matched.\nwarning: 'the' is a stop word",
        ),
    ],
)
def test_render_pins_the_text_the_model_reads(body, want):
    """Each response shape renders to its exact text, as the Go bridge renders it.

    A 204 decodes to an empty body, and reads as a graph that holds nothing.
    That calls for remembering something, where a miss calls for asking
    differently, so the two must never read alike.
    """
    assert render(body) == want


def test_a_recall_with_terms_carries_the_vector_of_its_terms(
    server_over, fraise_client, embedder, vector, call_tool
):
    """The terms are encoded, vec:$v is appended, and the vector is bound beside it."""
    got = call_tool(server_over(), "recall", "recall storm topic:weather")
    embedder.assert_called_once_with("storm")
    fraise_client.query.assert_called_once_with(
        "recall storm topic:weather vec:$v", parameters={"v": vector}
    )
    assert not got.is_error
    assert got.content[0].text == (
        "- the barometer falls before the storm (relevance 1.235)\n"
        "- the harbour closes in a gale (relevance 0.500)"
    )


def test_an_explained_recall_carries_the_vector_of_its_terms(
    server_over, fraise_client, embedder, vector, explained_hits, call_tool
):
    """The prefix is stepped over: its terms are encoded, the statement sent whole.

    The daemon parses the prefix, so the statement reaches it as written, with
    vec:$v appended and the vector of the recall's own terms bound beside it.
    The text reads the same ranked facts as the plain recall's: the explain
    blocks ride in the structured content, not in the lines.
    """
    fraise_client.query.return_value = explained_hits
    got = call_tool(server_over(), "recall", "explain recall storm topic:weather")
    embedder.assert_called_once_with("storm")
    fraise_client.query.assert_called_once_with(
        "explain recall storm topic:weather vec:$v", parameters={"v": vector}
    )
    assert not got.is_error
    assert got.content[0].text == (
        "- the barometer falls before the storm (relevance 1.235)\n"
        "- the harbour closes in a gale (relevance 0.500)"
    )


def test_a_recall_answers_the_daemons_response_as_structured_content(
    server_over, hits, call_tool
):
    """The structured content is the daemon's response, beside the rendered lines.

    A client reading the structure gets each hit's value, times and score as
    the query endpoint returned them, as the built-in bridge hands them over.
    """
    got = call_tool(server_over(), "recall", "recall storm topic:weather")
    assert not got.is_error
    assert got.structured_content == hits


def test_an_explained_recall_carries_its_explain_blocks_as_structured_content(
    server_over, fraise_client, explained_hits, call_tool
):
    """An explain recall's blocks reach the client whole, per hit and per query.

    The text leaves them out, so the structured content is the only way the
    explanation reaches a model: dropping them there would make explain recall
    answer exactly as the recall it prefixes.
    """
    fraise_client.query.return_value = explained_hits
    got = call_tool(server_over(), "recall", "explain recall storm topic:weather")
    assert not got.is_error
    assert got.structured_content == explained_hits


@pytest.mark.parametrize(
    "body, want",
    [
        ({}, {"results": {"count": 0, "hits": []}}),
        (
            {"results": {"count": 0, "hits": []}, "warnings": ["'the' is a stop word"]},
            {"results": {"count": 0, "hits": []}, "warnings": ["'the' is a stop word"]},
        ),
    ],
)
def test_a_recall_with_nothing_to_show_still_answers_its_schema(
    server_over, fraise_client, call_tool, body, want
):
    """An empty graph's 204 reads as an empty result, and warnings ride beside it.

    The 204 carries no body, but the output schema requires results, so a
    client validating the structure would reject the call without them.
    """
    fraise_client.query.return_value = body
    got = call_tool(server_over(), "recall", "recall storm")
    assert not got.is_error
    assert got.structured_content == want


def test_the_recall_tool_publishes_the_bridges_output_schema(server_over, list_tools):
    """The recall tool's output schema holds the query endpoint's read response.

    It is the built-in bridge's schema: results required, each hit's explain
    block and the query's, and the when results nested in the query's block
    shaped as the results themselves. A remember answers in text alone.
    """
    tools = {tool.name: tool for tool in list_tools(server_over())}
    schema = tools["recall"].output_schema
    defs = schema["$defs"]
    got = {
        "output": schema["required"],
        "results": defs["Results"]["required"],
        "hit": defs["Hit"]["required"],
        "hit explain": defs["Hit"]["properties"]["explain"],
        "hit block": defs["HitExplain"]["required"],
        "recall explain": defs["Results"]["properties"]["explain"],
        "recall block": defs["RecallExplain"]["required"],
        "when": defs["NestedResults"]["properties"],
    }
    assert got == {
        "output": ["results"],
        "results": ["count", "hits"],
        "hit": ["value", "timestamp", "score"],
        "hit explain": {"$ref": "#/$defs/HitExplain"},
        "hit block": ["seed", "decay", "contributions"],
        "recall explain": {"$ref": "#/$defs/RecallExplain"},
        "recall block": ["background", "seeds", "terms", "timings"],
        "when": {
            "since": {"$ref": "#/$defs/Results"},
            "until": {"$ref": "#/$defs/Results"},
        },
    }
    assert tools["remember"].output_schema is None


def test_a_remember_carries_the_vector_of_its_fact(
    server_over, fraise_client, embedder, vector, call_tool
):
    """The fact is encoded, so a recall's vector can later find it."""
    fraise_client.query.return_value = {"status": "ok"}
    got = call_tool(server_over(), "remember", "remember 'the tide turns' topic:sea")
    embedder.assert_called_once_with("the tide turns")
    fraise_client.query.assert_called_once_with(
        "remember 'the tide turns' topic:sea vec:$v", parameters={"v": vector}
    )
    assert got.content[0].text == "Remembered."


def test_an_anchor_only_recall_is_sent_as_written(
    server_over, fraise_client, embedder, call_tool
):
    """Anchors alone seed the search, so no vector is encoded or bound."""
    call_tool(server_over(), "recall", "recall topic:weather top:20")
    embedder.assert_not_called()
    fraise_client.query.assert_called_once_with(
        "recall topic:weather top:20", parameters=None
    )


def test_without_vectors_every_statement_is_sent_as_written(
    server_over, fraise_client, call_tool
):
    """With vectors off the server is a plain FQL bridge."""
    call_tool(server_over(vectors=False), "recall", "recall storm topic:weather")
    fraise_client.query.assert_called_once_with(
        "recall storm topic:weather", parameters=None
    )


@pytest.mark.parametrize(
    "error, want",
    [
        (
            FraiseAPIError(400, "top:0 out of range (1-1000)"),
            "top:0 out of range (1-1000)",
        ),
        (
            FraiseError("could not reach fraise at http://localhost:9876: refused"),
            "could not reach fraise at http://localhost:9876: refused",
        ),
    ],
)
def test_a_failure_is_a_tool_error_the_model_can_read(
    server_over, fraise_client, call_tool, error, want
):
    """The daemon's own message reaches the model, so it can correct its statement.

    An unreachable daemon is a tool error too, never a protocol error: the
    model should relay it, not lose the call.
    """
    fraise_client.query.side_effect = error
    got = call_tool(server_over(), "recall", "recall storm top:0")
    assert got.is_error
    assert want in got.content[0].text


def test_the_tools_take_one_fql_string_and_never_mention_vectors(
    server_over, list_tools
):
    """Each tool takes a single query string, and nothing tells the model about vec:.

    The server binds the vector itself. A model that learnt the clause would
    write one, which the server leaves alone, and lose the local embedding.
    """
    tools = {tool.name: tool for tool in list_tools(server_over())}
    assert sorted(tools) == ["recall", "remember"]
    for tool in tools.values():
        assert tool.input_schema["required"] == ["query"]
        assert list(tool.input_schema["properties"]) == ["query"]
        assert tool.input_schema["properties"]["query"]["type"] == "string"
        assert "vec" not in str(tool.description) + str(tool.input_schema)


def test_the_recall_description_names_the_explain_prefix(server_over, list_tools):
    """The recall tool says a recall may be written with the explain prefix.

    A model learns the statements the tool accepts only from its description,
    so the prefix is named there.
    """
    tools = {tool.name: tool for tool in list_tools(server_over())}
    assert "explain recall" in str(tools["recall"].description)


def test_an_embedding_failure_is_a_tool_error_naming_it(
    server_over, fraise_client, embedder, call_tool
):
    """A model that cannot load says why, and nothing is sent without its vector.

    The first vector loads the model, and downloads it the first time. Offline
    or out of disk, that fails, and the operator needs the reason, not an
    anonymous crash. Sending the statement without the vector instead would
    store a fact a later vector recall cannot find.
    """
    embedder.side_effect = OSError("no space left on device")
    got = call_tool(server_over(), "remember", "remember 'the tide turns'")
    assert got.is_error
    assert "could not embed the statement: no space left on device" in (
        got.content[0].text
    )
    fraise_client.query.assert_not_called()


@pytest.mark.integration
def test_an_explained_recall_brings_its_blocks_back_through_the_command(
    call_command, spelling_pair, control_graph
):
    """An explain recall through the running command returns the daemon's blocks.

    The control graph takes no vector, so the run goes without one, and the
    fact is recalled by its own words. The client validates the structured
    content against the published schema, so a block missing a key the schema
    requires, or carrying one of the wrong type, fails the call.
    """
    fact, _ = spelling_pair
    call_command("remember", f"remember@{control_graph} '{fact}'", "--no-vectors")
    got = call_command(
        "recall", f"explain recall@{control_graph} '{fact}'", "--no-vectors"
    )
    assert not got.is_error
    assert got.content[0].text.startswith(f"- {fact} (relevance")
    results = got.structured_content["results"]
    assert {"background", "seeds", "terms", "timings"} <= set(results["explain"])
    assert all(
        {"seed", "decay", "contributions"} <= set(hit["explain"])
        for hit in results["hits"]
    )
