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
        (
            {
                "results": {
                    "count": 0,
                    "hits": [],
                    "catalogue": {
                        "graph": 2,
                        "scope": "topic:weather top:2",
                        "facts": 3,
                        "anchors": 4,
                        "happened": {"start": "2023-05-01T00:00:00Z"},
                        "topics": [
                            {
                                "value": "weather",
                                "facts": 3,
                                "happened": {"start": "2023-05-01T00:00:00Z"},
                            }
                        ],
                        "entities": [
                            {
                                "value": "harbour",
                                "facts": 2,
                                "happened": {
                                    "start": "2023-05-02T00:00:00Z",
                                    "end": "2023-05-04T00:00:00Z",
                                },
                            },
                            {
                                "value": "anna",
                                "facts": 1,
                                "happened": {
                                    "start": "2023-05-02T00:00:00Z",
                                    "end": "2023-05-02T00:00:00Z",
                                },
                            },
                        ],
                    },
                }
            },
            "describe recall@2 topic:weather top:2: 3 facts under 4 anchors, "
            "happened 2023-05-01..\n"
            "topics:\n"
            "- [2023-05-01..] weather: 3 facts\n"
            "entities:\n"
            "- [2023-05-02..2023-05-03] harbour: 2 facts\n"
            "- [2023-05-02] anna: 1 fact",
        ),
        (
            {
                "results": {
                    "count": 0,
                    "hits": [],
                    "catalogue": {
                        "graph": 5,
                        "scope": "",
                        "facts": 1,
                        "anchors": 1,
                        "happened": {
                            "start": "2023-05-02T00:00:00Z",
                            "end": "2023-05-02T00:00:00Z",
                        },
                        "topics": [],
                        "entities": [
                            {
                                "value": "bob",
                                "facts": 1,
                                "happened": {
                                    "start": "2023-05-02T00:00:00Z",
                                    "end": "2023-05-02T00:00:00Z",
                                },
                            }
                        ],
                    },
                }
            },
            "describe recall@5: 1 fact under 1 anchor, happened 2023-05-02\n"
            "topics: none\n"
            "entities:\n"
            "- [2023-05-02] bob: 1 fact",
        ),
        (
            {
                "results": {
                    "count": 0,
                    "hits": [],
                    "catalogue": {
                        "graph": 0,
                        "scope": "until:(when zebras)",
                        "facts": 0,
                        "anchors": 0,
                        "topics": [],
                        "entities": [],
                    },
                },
                "warnings": ["until: the when matched no fact"],
            },
            "describe recall@0 until:(when zebras): no fact reached.\n"
            "warning: until: the when matched no fact",
        ),
    ],
)
def test_render_pins_the_text_the_model_reads(body, want):
    """Each response shape renders to its exact text, as the Go bridge renders it.

    A 204 decodes to an empty body, and reads as a graph that holds nothing.
    That calls for remembering something, where a miss calls for asking
    differently, so the two must never read alike. A describe's catalogue
    renders in place of hits, the describe spelt as the server read it first,
    then each kind of anchor, a count of one in the singular.
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


def test_a_describe_is_sent_as_written(server_over, fraise_client, embedder, call_tool):
    """A describe takes no vector, so its terms reach only the facts they match.

    A vector ranks every fact that holds one, so a describe sent with one
    would picture every such fact instead of what the terms reach.
    """
    call_tool(server_over(), "recall", "describe recall storm topic:weather")
    embedder.assert_not_called()
    fraise_client.query.assert_called_once_with(
        "describe recall storm topic:weather", parameters=None
    )


def test_the_recall_tool_teaches_describe(server_over, list_tools):
    """The recall tool's description and its query description name describe recall.

    Each says that this tool runs a describe, so a model looking for one
    reaches for the tool it has. A model that does not know how memory is filed
    learns from them to describe before its first recall.
    """
    recall = {tool.name: tool for tool in list_tools(server_over())}["recall"]
    query = recall.input_schema["properties"]["query"]["description"]
    assert "This tool also runs describe" in str(recall.description)
    assert "describe recall first" in str(recall.description)
    assert "describe recall@2" in query
    assert "which this tool runs as it runs a recall" in query


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
def test_a_describe_through_the_recall_tool_answers_with_what_its_terms_reach(
    call_command, describe_graph
):
    """The recall tool carries a describe, and reads back its catalogue.

    Both facts are written with vectors, so the recall of heron, sent with one,
    ranks the tide too: a vector reaches every fact that holds one. The
    describe of the same words is sent without one, so it counts the heron
    fact alone, under its one topic, rather than the whole graph.
    """
    for fact in (
        "remember@{} 'the heron fishes at dusk' topic:birds",
        "remember@{} 'the tide turns at dawn' topic:sea",
    ):
        remembered = call_command("remember", fact.format(describe_graph))
        assert remembered.content[0].text == "Remembered."

    recalled = call_command("recall", f"recall@{describe_graph} heron")
    described = call_command("recall", f"describe recall@{describe_graph} heron")

    assert len(recalled.content[0].text.splitlines()) == 2
    assert not described.is_error
    header, topics, birds, entities = described.content[0].text.splitlines()
    assert header.startswith(
        f"describe recall@{describe_graph} heron: 1 fact under 1 anchor, happened "
    )
    assert (topics, entities) == ("topics:", "entities: none")
    assert birds.startswith("- [")
    assert birds.endswith("] birds: 1 fact")
