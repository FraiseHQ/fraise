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
"""Fixtures for the fraise-mcp suite, unit and integration alike.

Every fixture the suite uses lives here, and test modules never import from
this file: the only way out is a fixture, injected through a test's arguments.

The mocked half builds the server over a ``MagicMock`` client and embedder,
and calls its tools in process through the MCP client. So the unit tests run
the protocol path, tool errors included, with no daemon and no model. The live
half follows it and backs the tests marked ``integration``. It launches the
``fraise-mcp`` command as a subprocess against the daemon named by FRAISE_URL,
one subprocess per call, so every call is a restart of the server.
`-m "not integration"` is the unit run and touches nothing live.
`-m integration` needs the daemon up, and downloads the default model once.
"""

import asyncio
import os
import sys
import time
from pathlib import Path
from unittest.mock import MagicMock

import pytest
from fraise_mcp.server import build_server
from fraise_sdk.client import FraiseClient
from mcp import Client, StdioServerParameters


def pytest_configure(config):
    # Register the marker for `-m integration` and `-m "not integration"`, so
    # pytest does not warn about an unknown mark.
    config.addinivalue_line(
        "markers",
        "integration: drives the fraise-mcp command against a live daemon",
    )


# What the unit suite's embedder answers for any text. Two floats are enough
# to see the vector arrive in the request parameters.
_VECTOR = [0.5, 0.25]

_HITS = {
    "results": {
        "count": 2,
        "hits": [
            {
                "value": "the barometer falls before the storm",
                "timestamp": "2026-10-01T09:00:00Z",
                "score": 1.23456,
            },
            {
                "value": "the harbour closes in a gale",
                "timestamp": "2026-09-30T09:00:00Z",
                "score": 0.5,
            },
        ],
    }
}


@pytest.fixture
def fraise_client():
    """A mock ``FraiseClient`` whose query answers a two-hit recall.

    Returns:
        A ``MagicMock``. Set ``query.return_value`` or ``query.side_effect`` to
        answer otherwise.
    """
    client = MagicMock()
    client.query.return_value = _HITS
    return client


@pytest.fixture
def embedder():
    """A mock embedder answering the same vector for any text."""
    return MagicMock(return_value=list(_VECTOR))


@pytest.fixture(scope="session")
def vector():
    """The vector the mock embedder answers."""
    return list(_VECTOR)


@pytest.fixture
def call_tool():
    """Callable calling a tool on an in-process server through the MCP client.

    Returns:
        ``callable(server, name, query) -> CallToolResult``.
    """

    async def _call(server, name, query):
        async with Client(server) as client:
            return await client.call_tool(name, {"query": query})

    def _call_tool(server, name, query):
        return asyncio.run(_call(server, name, query))

    return _call_tool


@pytest.fixture
def list_tools():
    """Callable listing an in-process server's tools through the MCP client.

    Returns:
        ``callable(server) -> list[Tool]``.
    """

    async def _list(server):
        async with Client(server) as client:
            return await client.list_tools()

    def _list_tools(server):
        return asyncio.run(_list(server)).tools

    return _list_tools


@pytest.fixture
def server_over(fraise_client, embedder):
    """Callable building the server over the mock client.

    Returns:
        ``callable(vectors=True) -> MCPServer``, with the mock embedder, or
        none when ``vectors`` is false.
    """

    def _server_over(vectors=True):
        return build_server(fraise_client, embedder if vectors else None)

    return _server_over


# Everything below backs the tests marked `integration`. Nothing here waits or
# launches anything unless an integration test requests a fixture.

_FRAISE_URL = os.environ.get("FRAISE_URL", "http://localhost:9876")
_WAIT_TIMEOUT_SECONDS = 30

# The suite runs against a daemon of its own (make test-integration-mcp brings
# one up), so it claims graphs without the e2e allocation map. The first vector
# written to a graph fixes its dimension, so the graph the default model writes
# to is never written with another. The control graph never takes a vector.
_VECTOR_GRAPH = 0
_CONTROL_GRAPH = 1

# A fact and a question that share no word once stemmed, colour against color,
# favourite against favorite. Only a vector can match them, so the same recall
# tells the run with vectors apart from the run without.
_FACT = "my favourite colour is teal"
_QUESTION = "favorite color"


@pytest.fixture(scope="session")
def fraise_url():
    """The daemon's url: what FRAISE_URL names, or the local default."""
    deadline = time.monotonic() + _WAIT_TIMEOUT_SECONDS
    with FraiseClient(_FRAISE_URL) as client:
        while not client.health():
            if time.monotonic() > deadline:
                pytest.fail(f"fraise server not reachable at {_FRAISE_URL}")
            time.sleep(0.5)
    return _FRAISE_URL


@pytest.fixture(scope="session")
def spelling_pair():
    """A fact and a question only a vector can match, as ``(fact, question)``."""
    return _FACT, _QUESTION


@pytest.fixture(scope="session")
def vector_graph():
    """The graph the default model's vectors are written to."""
    return _VECTOR_GRAPH


@pytest.fixture(scope="session")
def control_graph():
    """The graph written without vectors."""
    return _CONTROL_GRAPH


@pytest.fixture(scope="session")
def call_command(fraise_url):
    """Callable calling one tool on a fresh ``fraise-mcp`` subprocess.

    Each call launches the command the console script installs, beside the
    running interpreter, so the server under test is the one ``uvx fraise-mcp``
    runs. It exits when the call returns.

    Returns:
        ``callable(name, query, *flags) -> CallToolResult``.
    """
    command = str(Path(sys.executable).with_name("fraise-mcp"))

    async def _call(name, query, flags):
        # The client hands a subprocess a minimal environment by default. An
        # agent host passes its own, and with it the model cache's location.
        params = StdioServerParameters(
            command=command, args=["--addr", fraise_url, *flags], env=dict(os.environ)
        )
        async with Client(params) as client:
            return await client.call_tool(name, {"query": query})

    def _call_command(name, query, *flags):
        return asyncio.run(_call(name, query, flags))

    return _call_command
