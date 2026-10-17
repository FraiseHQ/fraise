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

"""Fixtures for the Fraise Python SDK suite, unit and integration alike.

Every fixture the suite uses lives here, even one that a single test file asks
for. A test module holds assertions only. A fixture defined there would hide
setup where nobody looks for it. Test modules never import from this file. The
only way out is a fixture, injected through a test's arguments, so the values
behind the fixtures are private and start with an underscore.

The file has two halves. The mocked half patches the client's own
`requests.Session` at its import site, so unit tests need no server and no
daemon. The live half follows it and backs the tests marked ``integration``.
It gives a real client for the daemon named by FRAISE_URL, health-checked
before first use. `-m "not integration"` is the unit run and touches nothing
live. `-m integration` needs the daemon up.
"""

import asyncio
import copy
import hashlib
import json
import math
import os
import time
from unittest.mock import MagicMock, patch

import pytest
from fraise_sdk.client import DEFAULT_BASE_URL, FraiseClient
from fraise_sdk.models import RecallResult
from fraise_sdk.providers import Anchor


def pytest_configure(config):
    # Register the marker for `-m integration` and `-m "not integration"`, so
    # pytest does not warn about an unknown mark.
    config.addinivalue_line(
        "markers",
        "integration: drives the real SDK against a live server; needs the daemon",
    )


_QUERY_URL = f"{DEFAULT_BASE_URL}/api/v1/q"
_EXPLAIN_URL = f"{DEFAULT_BASE_URL}/api/v1/explain"
_STATS_URL = f"{DEFAULT_BASE_URL}/api/v1/stats"
_NO_HITS = {"results": {"count": 0, "hits": []}}

# Recorded from a live server, not written by hand, so the parser is tested
# against the shape the server really sends. The graph behind both explain
# responses holds three facts filed under entity:polly. Two of them mention
# "polly".
#
# `recall polly entity:polly depth:2`: two hits matched the text and were also
# reached through the polly anchor. The third matched nothing itself and was
# reached through the anchor alone, so it is a graph-only hit.
_EXPLAIN_RESPONSE = {
    "results": {
        "count": 3,
        "hits": [
            {
                "value": "polly whistles at dawn",
                "timestamp": "2026-09-30T22:57:46.062614+01:00",
                "score": 1.0499525788004205,
                "contributions": [
                    {
                        "source": "text",
                        "score": 1.0499527612542656,
                        "rank": 0,
                        "count": 1,
                    },
                    {
                        "source": "graph",
                        "score": 1.9820803996745116,
                        "rank": 0,
                        "via": "polly",
                        "degree": 3,
                        "count": 2,
                    },
                ],
            },
            {
                "value": "polly eats sunflower seeds",
                "timestamp": "2026-09-30T22:57:46.021977+01:00",
                "score": 0.9321274330290991,
                "contributions": [
                    {
                        "source": "text",
                        "score": 0.9321276384202458,
                        "rank": 1,
                        "count": 1,
                    },
                    {
                        "source": "graph",
                        "score": 1.9820803996745116,
                        "rank": 0,
                        "via": "polly",
                        "degree": 3,
                        "count": 2,
                    },
                ],
            },
            {
                "value": "the parrot sleeps in the kitchen",
                "timestamp": "2026-09-30T22:57:45.982021+01:00",
                "score": 0.04374802007585447,
                "contributions": [
                    {
                        "source": "graph",
                        "score": 1.9820803996745116,
                        "rank": 0,
                        "via": "polly",
                        "degree": 3,
                        "count": 2,
                    },
                ],
            },
        ],
        "background": 0.4857013396824596,
    }
}

# `recall entity:polly`: the anchor alone seeds it, so every hit is an anchor
# sighting. The background rate is zero, so the server omits it.
_ANCHOR_EXPLAIN_RESPONSE = {
    "results": {
        "count": 3,
        "hits": [
            {
                "value": "polly whistles at dawn",
                "timestamp": "2026-09-30T22:57:46.062614+01:00",
                "score": 0.9999997409465425,
                "contributions": [
                    {
                        "source": "anchor",
                        "score": 1,
                        "rank": 0,
                        "via": "polly",
                        "degree": 3,
                        "count": 1,
                    },
                ],
            },
            {
                "value": "polly eats sunflower seeds",
                "timestamp": "2026-09-30T22:57:46.021977+01:00",
                "score": 0.999999694373341,
                "contributions": [
                    {
                        "source": "anchor",
                        "score": 1,
                        "rank": 0,
                        "via": "polly",
                        "degree": 3,
                        "count": 1,
                    },
                ],
            },
            {
                "value": "the parrot sleeps in the kitchen",
                "timestamp": "2026-09-30T22:57:45.982021+01:00",
                "score": 0.9999996485805727,
                "contributions": [
                    {
                        "source": "anchor",
                        "score": 1,
                        "rank": 0,
                        "via": "polly",
                        "degree": 3,
                        "count": 1,
                    },
                ],
            },
        ],
    }
}

# `GET /api/v1/stats`, recorded from a live server with three graphs. Graph 0
# holds two facts filed under entity:polly, and one is also under topic:diet.
# Graph 1 was never written to. Graph 2 holds one fact with a vector and no
# anchors. The empty graph sits between two populated ones, so a parser that
# dropped or reordered it would misplace graph 2's row.
_STATS_RESPONSE = {
    "graphs": [
        {
            "id": 0,
            "order": 4,
            "size": 3,
            "nodes": 7,
            "vectors": 0,
            "forest_entries": 0,
            "relevance": "bm25",
        },
        {
            "id": 1,
            "order": 0,
            "size": 0,
            "nodes": 0,
            "vectors": 0,
            "forest_entries": 0,
            "relevance": "bm25",
        },
        {
            "id": 2,
            "order": 1,
            "size": 0,
            "nodes": 1,
            "vectors": 1,
            "forest_entries": 1,
            "relevance": "bm25",
        },
    ]
}

# A warning as the server sends it for "recall@0 ferry the". The query ran, but
# "the" is a stop word, which stored facts never contain. The warning points at
# that term.
_SERVER_WARNING = (
    'parse warning at column 18: term "the" is a stop word: stored facts '
    "never contain it, so it cannot match"
)


@pytest.fixture(scope="session")
def query_url():
    """The URL a query is posted to, unless it is an explained recall."""
    return _QUERY_URL


@pytest.fixture(scope="session")
def explain_url():
    """The URL an explained recall must be posted to."""
    return _EXPLAIN_URL


@pytest.fixture(scope="session")
def stats_url():
    """The URL the per-graph snapshot is read from."""
    return _STATS_URL


@pytest.fixture
def stats_response():
    """A recorded stats response: three graphs, the middle one empty."""
    return copy.deepcopy(_STATS_RESPONSE)


@pytest.fixture
def explain_response():
    """A recorded explain response: two text-and-graph hits, one graph-only hit."""
    return copy.deepcopy(_EXPLAIN_RESPONSE)


@pytest.fixture
def anchor_explain_response():
    """A recorded explain response for an anchor-only recall, zero background."""
    return copy.deepcopy(_ANCHOR_EXPLAIN_RESPONSE)


@pytest.fixture(scope="session")
def no_hits():
    """A response body carrying an empty recall result."""
    return dict(_NO_HITS)


@pytest.fixture(scope="session")
def server_warning():
    """A parse warning exactly as the server words it."""
    return _SERVER_WARNING


def _arm(session, body: dict, status_code: int = 200) -> MagicMock:
    """Arm ``session`` to answer POSTs with ``body``.

    Tests reach this through the ``respond`` fixture. The ``session`` fixture
    calls it to arm its default answer.
    """
    response = MagicMock(
        status_code=status_code,
        ok=200 <= status_code < 300,
        text=json.dumps(body),
    )
    response.json.return_value = body
    session.post.return_value = response
    return response


@pytest.fixture
def session():
    """The session the client builds for itself, patched at its import site.

    Patching rather than injecting keeps the client's own construction path
    under test.

    Yields:
        The mock session every FraiseClient built in the test will use, armed
        to answer with an empty recall result.
    """
    with patch("fraise_sdk.client.requests.Session") as session_class:
        patched = session_class.return_value
        _arm(patched, _NO_HITS)
        yield patched


@pytest.fixture
def respond():
    """Callable arming a session to answer POSTs with a given body.

    Returns:
        ``callable(session, body, status_code=200) -> MagicMock``. It returns
        the mock response, so a test can alter it directly.
    """
    return _arm


def _arm_no_content(session) -> MagicMock:
    """Arm ``session`` to answer POSTs with a bodiless 204.

    Tests reach this through the ``respond_no_content`` fixture. It is separate
    from ``_arm`` because a 204 is the one success with no body. Decoding it
    raises, as ``requests`` does on an empty payload. So a client that decodes
    it unguarded fails here as it would against the server. A mock that
    returned ``{}`` would let it pass.
    """
    response = MagicMock(status_code=204, ok=True, text="")
    response.json.side_effect = ValueError("no JSON object could be decoded")
    session.post.return_value = response
    return response


@pytest.fixture
def respond_no_content():
    """Callable arming a session to answer POSTs with a bodiless 204.

    The server answers 204 to a recall of a graph that holds nothing. A
    populated graph where no fact matched answers 200 with an empty result.

    Returns:
        ``callable(session) -> MagicMock``, which returns the mock response.
    """
    return _arm_no_content


def _arm_get(session, body, status_code: int = 200) -> MagicMock:
    """Arm ``session`` to answer GETs with ``body``.

    Tests reach this through the ``respond_get`` fixture. It mirrors ``_arm``,
    which arms POST. The client sends GETs only for health, version and stats.
    """
    response = MagicMock(
        status_code=status_code,
        ok=200 <= status_code < 300,
        text=json.dumps(body),
    )
    response.json.return_value = body
    session.get.return_value = response
    return response


@pytest.fixture
def respond_get():
    """Callable arming a session to answer GETs with a given body.

    The ``session`` fixture arms POST only. The health, version, compatibility
    and stats tests arm GET through this.

    Returns:
        ``callable(session, body, status_code=200) -> MagicMock``. It returns
        the mock response, so a test can alter it directly.
    """
    return _arm_get


@pytest.fixture
def sent():
    """Callable returning the JSON payload of the single POST a session made.

    Returns:
        ``callable(session) -> dict``, which also asserts exactly one POST
        happened.
    """

    def _sent(session) -> dict:
        session.post.assert_called_once()
        return session.post.call_args.kwargs["json"]

    return _sent


# The unit suite's embedder returns len(text) four times, so a test can predict
# the vector the client sends. Tests reach it through callable_embedder. The
# live half's embedder is the `encode` fixture below.
def _len_encode(text: str) -> list[float]:
    return [float(len(text))] * 4


@pytest.fixture
def callable_embedder():
    """Callable building a mock of the bare ``callable(text) -> vector`` shape.

    Returns:
        ``callable() -> MagicMock``. A fresh mock per call, so a test using two
        embedders can tell their call records apart.
    """

    def _callable_embedder() -> MagicMock:
        embedder = MagicMock(side_effect=_len_encode)
        # A plain callable has no .embed. Deleting it sends resolve_embedder
        # down the callable branch instead of the Embedder one.
        del embedder.embed
        return embedder

    return _callable_embedder


# The models the embedders default to, pinned as literals. A changed default
# then fails the provider tests instead of being followed by them.
_OPENAI_DEFAULT_MODEL = "text-embedding-3-small"
_HUGGINGFACE_DEFAULT_MODEL = "sentence-transformers/all-MiniLM-L6-v2"


@pytest.fixture(scope="session")
def openai_default_model():
    """The embedding model OpenAIEmbedder uses when none is given."""
    return _OPENAI_DEFAULT_MODEL


@pytest.fixture
def embeddings_client():
    """Callable building a mock ``openai.OpenAI`` answering one embedding.

    Returns:
        ``callable(embedding=(0.1, 0.2, 0.3)) -> MagicMock`` whose
        ``embeddings.create`` answers with ``embedding``.
    """

    def _embeddings_client(embedding=(0.1, 0.2, 0.3)) -> MagicMock:
        client = MagicMock()
        client.embeddings.create.return_value = MagicMock(
            data=[MagicMock(embedding=list(embedding))]
        )
        return client

    return _embeddings_client


@pytest.fixture(scope="session")
def huggingface_default_model():
    """The embedding model HuggingFaceEmbedder uses when none is given."""
    return _HUGGINGFACE_DEFAULT_MODEL


@pytest.fixture
def inference_client():
    """Callable building a mock ``huggingface_hub.InferenceClient``.

    The real ``feature_extraction`` returns a numpy array, and the embedder
    uses only its ``tolist()``. So the mock answers with an object whose
    ``tolist()`` returns the values.

    Returns:
        ``callable(values=(0.1, 0.2, 0.3)) -> MagicMock``.
    """

    def _inference_client(values=(0.1, 0.2, 0.3)) -> MagicMock:
        client = MagicMock()
        client.feature_extraction.return_value = MagicMock(
            **{"tolist.return_value": list(values)}
        )
        return client

    return _inference_client


# What the suite's extractor finds in any text. "travel" repeats a topic a test
# gives. "Anne" repeats an entity it gives, in another casing. "Lisbon airport"
# is not one plain word. So one remember shows the repeats dropped and the
# value quoted.
_EXTRACTED_ANCHORS = (
    Anchor(value="travel", type="topic"),
    Anchor(value="trips", type="topic"),
    Anchor(value="Anne", type="entity"),
    Anchor(value="Lisbon airport", type="entity"),
)


@pytest.fixture
def callable_extractor():
    """Callable building a mock of the bare ``callable(text, context) -> anchors`` shape.

    Returns:
        ``callable() -> MagicMock``. The mock answers every text with the same
        four anchors, two topics and two entities, whatever its context. Each
        call builds a fresh mock, so a test can assert on the text and the
        context its own extractor was given.
    """

    def _callable_extractor() -> MagicMock:
        extractor = MagicMock(return_value=list(_EXTRACTED_ANCHORS))
        # A plain callable has no .extract. Deleting it sends resolve_extractor
        # down the callable branch instead of the Extractor one.
        del extractor.extract
        return extractor

    return _callable_extractor


@pytest.fixture
def chat_client():
    """Callable building a mock ``openai.OpenAI`` answering one chat completion.

    The suite makes no vendor calls, so the model's answer is scripted where
    the real client puts it: ``choices[0].message.content`` and
    ``finish_reason``.

    Returns:
        ``callable(content, finish_reason="stop") -> MagicMock``, where
        ``content`` is the answer's text, or ``None`` for a refusal.
    """

    def _chat_client(content: str | None, finish_reason: str = "stop") -> MagicMock:
        client = MagicMock()
        client.chat.completions.create.return_value = MagicMock(
            choices=[
                MagicMock(
                    message=MagicMock(content=content), finish_reason=finish_reason
                )
            ]
        )
        return client

    return _chat_client


@pytest.fixture
def mock_client():
    """Callable building a mock FraiseClient for the agent-framework tools.

    Returns:
        ``callable(hits=(), raises=None) -> MagicMock`` whose ``recall`` answers
        with ``hits``. With ``raises`` set, ``recall`` and ``remember`` raise it.
    """

    def _mock_client(hits=(), raises=None) -> MagicMock:
        client = MagicMock()
        client.recall.return_value = RecallResult(count=len(hits), hits=list(hits))
        if raises is not None:
            client.recall.side_effect = raises
            client.remember.side_effect = raises
        return client

    return _mock_client


@pytest.fixture
def invoke_function_tool():
    """Callable running an OpenAI Agents ``FunctionTool`` as the runtime would.

    The arguments reach ``on_invoke_tool`` as a JSON string. So a test covers
    the framework's own argument parsing and validation, not just the function
    the tool wraps. The framework is imported only when a test asks for this
    fixture, so the suite still collects without the 'openai' extra.

    Returns:
        ``callable(tool, **arguments) -> str``, the tool's answer.
    """
    from agents.tool_context import ToolContext

    def _invoke(tool, **arguments):
        payload = json.dumps(arguments)
        context = ToolContext(
            context=None,
            tool_name=tool.name,
            tool_call_id="test-call",
            tool_arguments=payload,
        )
        return asyncio.run(tool.on_invoke_tool(context, payload))

    return _invoke


@pytest.fixture
def invoke_mcp_tool():
    """Callable running a Claude Agent SDK tool's handler as its MCP server would.

    Returns:
        ``callable(tool, **arguments) -> dict``, the MCP content payload.
    """

    def _invoke(tool, **arguments):
        return asyncio.run(tool.handler(arguments))

    return _invoke


@pytest.fixture
def mcp_text():
    """Callable reading the text of an MCP content payload.

    Returns:
        ``callable(payload) -> str``, the text of the payload's first block.
    """

    def _text(payload):
        return payload["content"][0]["text"]

    return _text


# Everything below backs the tests marked `integration`, with a real client
# against the daemon named by FRAISE_URL. Nothing here waits or writes unless
# an integration test requests a fixture.


_FRAISE_URL = os.environ.get("FRAISE_URL", "http://localhost:9876")
_WAIT_TIMEOUT_SECONDS = 30

_ROUND_TRIP_GRAPH = 0
_VECTOR_GRAPH = 1
_QUERY_GRAPH = 2
_MODELS_GRAPH = 3

# Claimed for the exact stats counts. Nothing else may write here, because any
# other fact would move them. The e2e suite's allocation map lists it for the
# same reason.
_STATS_GRAPH = 9

# Claimed by staying empty. A recall of a graph that holds nothing is answered
# 204, and only a graph no test writes to can show that. Both suites drive the
# same daemon and only read this graph. So this is the graph the e2e suite's
# allocation map reserves for its own empty-graph probe. A second empty graph
# would be one that map does not know about. Do not write to this graph.
_EMPTY_GRAPH = 8

# The dimension of every vector this suite writes. The first vector inserted
# into a graph fixes that graph's dimension. More than one file writes vectors,
# so they must agree.
_VECTOR_DIM = 8

# A keyword no fact in any graph contains.
_NO_MATCH = "zzznomatchzzz"

# A server nothing listens on. Port 1 is reserved (tcpmux) and never bound in
# the test environment, so a connection fails at once instead of hanging until
# a timeout.
_DEAD_URL = "http://127.0.0.1:1"

# Four facts on one topic hub, each with a unique keyword, so result counts are
# exact. Each keyword matches one fact. The hub a single seed reaches holds
# only its fair share, so it transmits nothing in either graph lane. This
# mirrors the e2e suite's star, written through the SDK instead of raw HTTP.
_INSTRUMENT_TOPIC = "instruments"
_INSTRUMENT_FACTS = {
    "cello": "the cello is bowed and tuned in fifths",
    "flute": "the flute is blown across a lip plate",
    "timpani": "the timpani is struck with felt mallets",
    "harp": "the harp is plucked with both hands",
}

# Two anchored facts and one anchorless fact with a vector, so a stats snapshot
# of their graph is exact. Three facts, one topic and two entities make six
# vertices. The facts' links to the anchors make five edges. Vertices and edges
# together make eleven stored nodes. Rewriting a fact rewrites the same nodes,
# so the counts hold across reruns against a long-lived server.
_STATS_FACTS = (
    ("the heron fishes at dusk", ["birds"], ["heron"]),
    ("the egret wades beside the heron", ["birds"], ["heron", "egret"]),
)
_STATS_VECTOR_FACT = "the marsh floods in spring"

# Two facts that share the word "tide" and a topic hub. A single recall then
# returns more than one hit, so the ordering and count assertions have
# something to check.
_TIDE_TOPIC = "tides"
_TIDE_FACTS = {
    "spring": "a spring tide follows the new moon",
    "neap": "a neap tide follows the first quarter",
}


def _encode(text: str) -> list[float]:
    """Encode ``text`` as a deterministic unit vector of _VECTOR_DIM floats.

    This is a real embedder (any ``callable(text) -> Sequence[float]``), not a
    mock, so integration tests drive the client's real embedding path. The
    components come from a digest, so the vector is the same across runs and
    processes. A test can store a fact and recall it by encoding the same text
    again.

    Args:
        text: the text to encode.

    Returns:
        A unit-length vector of _VECTOR_DIM floats.
    """
    digest = hashlib.sha256(text.encode()).digest()
    raw = [byte / 255.0 for byte in digest[:_VECTOR_DIM]]
    norm = math.sqrt(sum(x * x for x in raw)) or 1.0
    return [x / norm for x in raw]


def _await_server(fraise: FraiseClient) -> None:
    """Block until the server answers its health check, or fail.

    It calls ``pytest.fail`` when the server never comes up. Every test that
    needs the server then errors with one clear message, not its own
    connection error.

    Args:
        fraise: the client whose health check to poll.
    """
    deadline = time.monotonic() + _WAIT_TIMEOUT_SECONDS
    while time.monotonic() < deadline:
        if fraise.health():
            return
        time.sleep(0.5)
    fraise.close()
    pytest.fail(f"fraise server not reachable at {_FRAISE_URL}")


@pytest.fixture(scope="session")
def fraise_url():
    """The base url of the server under test.

    Returns:
        The url FRAISE_URL names, or the local default.
    """
    return _FRAISE_URL


@pytest.fixture(scope="session")
def dead_url():
    """A base url nothing is listening on, for the transport-failure paths.

    Returns:
        A url whose connection is refused immediately.
    """
    return _DEAD_URL


@pytest.fixture(scope="session")
def no_match():
    """A keyword no fact in any graph contains.

    Returns:
        The keyword to seed a deliberately empty recall with.
    """
    return _NO_MATCH


@pytest.fixture(scope="session")
def vector_dim():
    """The width every vector in this suite is written with.

    Returns:
        The dimension each vector-carrying graph is fixed at.
    """
    return _VECTOR_DIM


@pytest.fixture(scope="session")
def encode():
    """The embedder the suite drives the client's real vector path with.

    Returns:
        A deterministic ``callable(text) -> list[float]``.
    """
    return _encode


@pytest.fixture(scope="session")
def instrument_topic():
    """The topic hub the instrument facts hang off.

    Returns:
        The topic name.
    """
    return _INSTRUMENT_TOPIC


@pytest.fixture(scope="session")
def instrument_facts():
    """The facts making up the instrument star.

    Returns:
        A mapping of keyword to the fact containing it.
    """
    return dict(_INSTRUMENT_FACTS)


@pytest.fixture(scope="session")
def client():
    """A FraiseClient pointed at a server confirmed to be up.

    Yields:
        The client, closed at the end of the session.
    """
    fraise = FraiseClient(_FRAISE_URL)
    _await_server(fraise)
    yield fraise
    fraise.close()


@pytest.fixture(scope="session")
def embedding_client():
    """A FraiseClient that vectorises implicitly through the suite's embedder.

    It is kept apart from ``client`` on purpose. The plain client must keep
    proving that the SDK works with no embedder at all, which is how most
    callers start.

    Yields:
        The client, closed at the end of the session.
    """
    fraise = FraiseClient(_FRAISE_URL, embedder=_encode)
    _await_server(fraise)
    yield fraise
    fraise.close()


@pytest.fixture(scope="session")
def recalled_values(client):
    """A helper that returns the values a single-keyword recall finds.

    The recall names no anchor, so it runs on the text and vector indices alone.

    Args:
        client: the plain client the helper recalls through.

    Returns:
        A ``callable(keyword, graph) -> list[str]`` of ranked values.
    """

    def _recalled_values(keyword: str, graph: int) -> list[str]:
        return [hit.value for hit in client.recall(keyword, graph=graph, depth=0)]

    return _recalled_values


@pytest.fixture(scope="session")
def round_trip_graph():
    """The graph the plain remember/recall tests write to.

    Returns:
        The graph id.
    """
    return _ROUND_TRIP_GRAPH


@pytest.fixture(scope="session")
def empty_graph():
    """A graph no test ever writes to, so a recall of it finds nothing stored.

    Returns:
        The graph id.
    """
    return _EMPTY_GRAPH


@pytest.fixture(scope="session")
def query_graph():
    """The graph the builder-generated query strings are sent against.

    Returns:
        The graph id.
    """
    return _QUERY_GRAPH


@pytest.fixture(scope="session")
def models_graph():
    """The graph holding the facts that back the response-shape assertions.

    Returns:
        The graph id.
    """
    return _MODELS_GRAPH


@pytest.fixture(scope="module")
def stats_graph(client):
    """Write the stats facts to their own graph and return its id.

    Args:
        client: the plain client to write through.

    Returns:
        The graph id, now holding six vertices, five edges, eleven nodes and
        one vector.
    """
    for phrase, topics, entities in _STATS_FACTS:
        client.remember(phrase, graph=_STATS_GRAPH, topics=topics, entities=entities)
    client.remember(
        _STATS_VECTOR_FACT, graph=_STATS_GRAPH, vector=_encode(_STATS_VECTOR_FACT)
    )
    return _STATS_GRAPH


@pytest.fixture(scope="module")
def instrument_graph(client):
    """Populate the round-trip graph with the instrument star and return its id.

    A fact is keyed by its value, so these writes are idempotent and a rerun
    against a long-lived server leaves the counts unchanged.

    Args:
        client: the plain client to write through.

    Returns:
        The graph id holding the star.
    """
    for phrase in _INSTRUMENT_FACTS.values():
        client.remember(phrase, graph=_ROUND_TRIP_GRAPH, topics=[_INSTRUMENT_TOPIC])
    return _ROUND_TRIP_GRAPH


@pytest.fixture(scope="module")
def vector_graph(embedding_client):
    """Fix the vector graph's embedding dimension with a real write, return its id.

    The first vector into a graph decides that graph's dimension. Tests that
    need it to be _VECTOR_DIM already, such as the mismatch tests, must not
    race the write that sets it.

    Args:
        embedding_client: the client whose embedder sets the dimension.

    Returns:
        The graph id whose dimension is now _VECTOR_DIM.
    """
    embedding_client.remember("the tuning fork sounds a natural A", graph=_VECTOR_GRAPH)
    return _VECTOR_GRAPH


@pytest.fixture(scope="module")
def tide_result(client):
    """Store the tide facts and return the RecallResult a two-hit recall parses.

    Every response-shape assertion shares it, so they all read the same live
    response instead of each sending its own recall.

    Args:
        client: the plain client to write and recall through.

    Returns:
        The RecallResult the server produced for a two-hit recall.
    """
    for phrase in _TIDE_FACTS.values():
        client.remember(phrase, graph=_MODELS_GRAPH, topics=[_TIDE_TOPIC])
    return client.recall("tide", graph=_MODELS_GRAPH, depth=2)


@pytest.fixture(scope="module")
def vector_tide_result(embedding_client, vector_graph):
    """Return a RecallResult that the vector index seeded alongside the text index.

    Vector-seeded results take a different path through the engine. So their
    envelope is worth parsing apart from the text one.

    Args:
        embedding_client: the client that vectorises implicitly.
        vector_graph: the graph whose dimension is already established.

    Returns:
        The RecallResult of a recall whose query phrase was embedded.
    """
    fact = "the tide table is printed each spring"
    embedding_client.remember(fact, graph=vector_graph, topics=[_TIDE_TOPIC])
    return embedding_client.recall("tide", graph=vector_graph, query=fact)
