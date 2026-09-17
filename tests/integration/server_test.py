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

"""The bridge's identity and registration (pkg/mcp/server.go) over real stdio:
what an MCP client learns about fraise before it ever calls a tool.
"""


def test_initialize_names_the_server(mcp):
    """The handshake identifies the bridge as fraise with a real version.

    ``claude mcp add`` surfaces exactly this — a wrong name here is what a
    user sees in their client's server list.
    """
    result = mcp.handshake_result
    assert result["serverInfo"]["name"] == "fraise"
    assert result["serverInfo"]["version"], "the version must not be empty"


def test_tools_list_carries_both_tools_with_their_schemas(mcp):
    """tools/list names recall and remember, each carrying the wire schemas.

    The schemas mirror the HTTP query API: query required on the way in, and
    on the way out whatever that side of the API answers with — results for a
    recall, the status token for a write, which the server keeps apart so a
    stored fact is not the same bytes as a recall that matched nothing. A
    client that validates against these — as the Go SDK itself does — depends
    on them being here and matching the daemon.
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

    An agent learns the anchor-only shape from nothing but the description it
    is handed, so the phrase has to be there for it to ask "what do I know
    about this topic" before it knows what to search for.
    """
    tools = {t["name"]: t for t in mcp.request("tools/list")["tools"]}
    assert "anchors alone" in tools["recall"]["description"].lower()
