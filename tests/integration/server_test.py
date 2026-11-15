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

"""The bridge's identity and registration (pkg/mcp/server.go) over real stdio.

This is what an MCP client learns about fraise before it ever calls a tool.
"""


def test_initialize_names_the_server(mcp):
    """The handshake identifies the bridge as fraise with a real version.

    ``claude mcp add`` shows exactly this. A wrong name here is what a user
    sees in their client's server list.
    """
    result = mcp.handshake_result
    assert result["serverInfo"]["name"] == "fraise"
    assert result["serverInfo"]["version"], "the version must not be empty"


def test_tools_list_carries_both_tools_with_their_schemas(mcp):
    """tools/list names recall and remember, each carrying the wire schemas.

    The schemas mirror the HTTP query API. On the way in, query is required.
    On the way out, each tool requires what the API answers with: results for
    a recall, and the status token for a write. The Go SDK validates every
    tool result against its output schema before sending it. A client may
    validate against the schemas too, so they must be present and match the
    daemon.
    """
    tools = {t["name"]: t for t in mcp.request("tools/list")["tools"]}

    assert set(tools) == {"recall", "remember"}
    for name, tool in tools.items():
        assert tool["description"], f"{name} needs a model-facing description"
        assert tool["inputSchema"]["required"] == ["query"]
    assert tools["recall"]["outputSchema"]["required"] == ["results"]
    assert tools["remember"]["outputSchema"]["required"] == ["status"]


def test_recall_description_tells_the_model_anchors_alone_seed(mcp):
    """The recall tool says anchors alone are enough to search from.

    An agent learns the anchor-only shape only from the description it is
    given. So the phrase must be there for the agent to ask "what do I know
    about this topic" before it knows what to search for.
    """
    tools = {t["name"]: t for t in mcp.request("tools/list")["tools"]}
    assert "anchors alone" in tools["recall"]["description"].lower()


def test_remember_schema_tells_the_model_how_to_date_a_fact(mcp):
    """The remember tool's query description names said: and occurred:.

    An agent writes only the clauses its tool description shows it. A fact
    stored without them is dated the day it was written, and no later recall
    can be bounded by when it happened.
    """
    tools = {t["name"]: t for t in mcp.request("tools/list")["tools"]}
    description = tools["remember"]["inputSchema"]["properties"]["query"]["description"]
    assert "said:" in description
    assert "occurred:" in description


def test_recall_schema_tells_the_model_how_to_bound_by_another_fact(mcp):
    """The recall tool's query description shows a when inside a bound.

    The parenthesised when is the one form of a bound an agent cannot guess
    from since:7d, so the description must spell it, with the clock a when may
    name after its colon.
    """
    tools = {t["name"]: t for t in mcp.request("tools/list")["tools"]}
    description = tools["recall"]["inputSchema"]["properties"]["query"]["description"]
    assert "until:(when " in description
    assert "since:(when:said " in description


def test_recall_output_schema_declares_every_time_a_response_carries(mcp):
    """The recall output schema names the times of a hit and of a when.

    A client that validates against the schema must find the keys the daemon
    may send. None of them is required beyond the write time: a fact given no
    times carries neither said nor occurred, and only a when carries a start.
    """
    tools = {t["name"]: t for t in mcp.request("tools/list")["tools"]}
    results = tools["recall"]["outputSchema"]["properties"]["results"]

    assert {"start", "end"} <= set(results["properties"])
    assert results["required"] == ["count", "hits"]

    hit = results["properties"]["hits"]["items"]
    assert {"said", "occurred"} <= set(hit["properties"])
    assert hit["required"] == ["value", "timestamp", "score"]
    assert hit["properties"]["occurred"]["items"]["required"] == ["start"]
