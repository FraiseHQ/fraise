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

"""Fixtures for the Fraise Python SDK suite — unit and integration alike.

Every fixture the suite uses lives here, including the ones a single test file
asks for: a test module is assertions, and a fixture defined among them hides
setup where nobody looks for it. Test modules never import from this file —
the only channel out is a fixture, injected through a test's arguments — so
the values behind them are private, marked by a leading underscore.

The file has two halves. The mocked half patches the client's own
`requests.Session` at its import site, so unit tests run with no server and
no daemon. The live half, which follows it, backs the
tests marked ``integration``: a real client against the daemon named by
FRAISE_URL, health-checked before first use. `-m "not integration"` is the
unit run and touches nothing live; `-m integration` needs the daemon up.
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
    # Register the marker so `-m integration` / `-m "not integration"` work
    # and pytest doesn't warn about an unknown mark.
    config.addinivalue_line(
        "markers",
        "integration: drives the real SDK against a live server; needs the daemon",
    )


_QUERY_URL = f"{DEFAULT_BASE_URL}/api/v1/q"
_EXPLAIN_URL = f"{DEFAULT_BASE_URL}/api/v1/explain"
_STATS_URL = f"{DEFAULT_BASE_URL}/api/v1/stats"
_NO_HITS = {"results": {"count": 0, "hits": []}}

# Recorded from a live server, not written by hand, so the parser is tested
# against the shape the server really sends. The graph behind both: three facts
# filed under entity:polly, two of them mentioning "polly".
#
# `recall polly entity:polly depth:2` — two hits matched the text and were also
# reached through the polly anchor, and the third matched nothing itself and
# was reached through the anchor alone: a graph-only hit.
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

# `recall entity:polly` — seeded by the anchor alone, so every hit is an anchor
# sighting, and the background rate is zero, which the server omits.
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

# `GET /api/v1/stats`, recorded from a live server allocating three graphs.
# Graph 0 holds two facts filed under entity:polly, one also under topic:diet;
# graph 1 was never written to; graph 2 holds one fact with a vector and no
# anchors. The empty graph sits between two populated ones, so a parser that
# dropped or reordered it would misplace graph 2's row.
_STATS_RESPONSE = {
    "graphs": [
        {"id": 0, "order": 4, "size": 3, "nodes": 7, "vectors": 0, "forest_entries": 0},
        {"id": 1, "order": 0, "size": 0, "nodes": 0, "vectors": 0, "forest_entries": 0},
        {"id": 2, "order": 1, "size": 0, "nodes": 1, "vectors": 1, "forest_entries": 1},
    ]
}

# The shape the server sends for a query that ran with a term that cannot help
# it: "the" in "recall@0 ferry the" is a stop word, which stored facts never
# contain, and the warning says so at the term.
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

    Private: tests reach this through the ``respond`` fixture, and the
    ``session`` fixture calls it to arm its default answer.
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
        ``callable(session, body, status_code=200) -> MagicMock`` — the mock
        response, for tests that want to alter it directly.
    """
    return _arm


def _arm_no_content(session) -> MagicMock:
    """Arm ``session`` to answer POSTs with a bodiless 204.

    Private: tests reach this through the ``respond_no_content`` fixture. It
    is separate from ``_arm`` because a 204 is the one success with no body:
    decoding it raises, as ``requests`` does on an empty payload, so a client
    that decodes it unguarded fails here as it would against the server,
    rather than passing against a mock that returns ``{}``.
    """
    response = MagicMock(status_code=204, ok=True, text="")
    response.json.side_effect = ValueError("no JSON object could be decoded")
    session.post.return_value = response
    return response


@pytest.fixture
def respond_no_content():
    """Callable arming a session to answer POSTs with a bodiless 204.

    That is how the server reports a recall of a graph holding nothing, as
    distinct from a populated graph none of whose facts matched.

    Returns:
        ``callable(session) -> MagicMock`` — the mock response.
    """
    return _arm_no_content


def _arm_get(session, body, status_code: int = 200) -> MagicMock:
    """Arm ``session`` to answer GETs with ``body``.

    Private: tests reach this through the ``respond_get`` fixture. Mirrors
    ``_arm``, which arms POST — the health and version probes are the
    client's only GETs.
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

    The ``session`` fixture arms POST only; health/version/compatibility
    tests arm the GET side through this.

    Returns:
        ``callable(session, body, status_code=200) -> MagicMock`` — the mock
        response, for tests that want to alter it directly.
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


# The unit suite's embedder: len(text), 4 times, so a test can predict the
# vector the client will send. Tests reach it through callable_embedder; the
# `encode` fixture is the live half's embedder below.
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
        # A plain callable has no .embed — deleting it is what sends
        # resolve_embedder down the callable branch instead of the Embedder one.
        del embedder.embed
        return embedder

    return _callable_embedder


# The models the embedders default to, pinned as literals so a changed default
# fails the provider tests instead of being followed by them.
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

    ``feature_extraction`` really returns a numpy array, of which the embedder
    uses only ``tolist()``, so the mock answers with an object whose
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
# gives, "Anne" an entity it gives in another casing, and "Lisbon airport" is
# not one plain word, so one remember shows the repeats dropped and the value
# quoted.
_EXTRACTED_ANCHORS = (
    Anchor(value="travel", type="topic"),
    Anchor(value="trips", type="topic"),
    Anchor(value="Anne", type="entity"),
    Anchor(value="Lisbon airport", type="entity"),
)


@pytest.fixture
def callable_extractor():
    """Callable building a mock of the bare ``callable(text) -> anchors`` shape.

    Returns:
        ``callable() -> MagicMock`` answering every text with the same four
        anchors — two topics, two entities. A fresh mock per call, so a test
        can assert on the text its own extractor was given.
    """

    def _callable_extractor() -> MagicMock:
        extractor = MagicMock(return_value=list(_EXTRACTED_ANCHORS))
        # A plain callable has no .extract — deleting it is what sends
        # resolve_extractor down the callable branch instead of the Extractor one.
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
        with ``hits``; with ``raises`` set, ``recall`` and ``remember`` raise it.
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

    The arguments reach ``on_invoke_tool`` as a JSON string, so a test covers
    the framework's own argument parsing and validation, not just the function
    the tool wraps. The framework is imported here, when a test asks for this,
    so the suite still collects without the 'openai' extra.

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


# Everything below backs the tests marked `integration`: a real client
# against the daemon named by FRAISE_URL. Nothing here runs — no waiting,
# no writes — unless an integration test actually requests a fixture.


_FRAISE_URL = os.environ.get("FRAISE_URL", "http://localhost:9876")
_WAIT_TIMEOUT_SECONDS = 30

_ROUND_TRIP_GRAPH = 0
_VECTOR_GRAPH = 1
_QUERY_GRAPH = 2
_MODELS_GRAPH = 3

# Claimed for the stats counts, which are exact, so nothing else may write
# here: any other fact would move them. The e2e suite's allocation map lists
# it for the same reason.
_STATS_GRAPH = 9

# Claimed by staying empty: a recall of a graph holding nothing is answered
# 204, which only a graph no test ever writes to can exercise. Both suites
# drive the same daemon and only ever read this graph, so it is the one the
# e2e suite's allocation map already reserves for its own empty-graph probe,
# rather than a second graph that map knows nothing about. Do not write to
# this graph.
_EMPTY_GRAPH = 8

# The dimension every vector in this suite is written with. The first vector
# inserted into a graph fixes that graph's dimension, and more than one file
# writes vectors, so they must agree.
_VECTOR_DIM = 8

# A keyword no fact in any graph contains.
_NO_MATCH = "zzznomatchzzz"

# A server nothing is listening on: port 1 is reserved (tcpmux) and never bound
# in the test environment, so connecting to it fails immediately rather than
# hanging until a timeout.
_DEAD_URL = "http://127.0.0.1:1"

# Four facts on one topic hub, each with a unique keyword, so result counts are
# exact: each keyword matches one fact, and the hub a single seed reaches holds
# only its fair share, so it transmits nothing in either graph lane. Mirrors
# the star the e2e suite uses, through the SDK instead of raw HTTP.
_INSTRUMENT_TOPIC = "instruments"
_INSTRUMENT_FACTS = {
    "cello": "the cello is bowed and tuned in fifths",
    "flute": "the flute is blown across a lip plate",
    "timpani": "the timpani is struck with felt mallets",
    "harp": "the harp is plucked with both hands",
}

# Two anchored facts and one anchorless fact with a vector, so a stats snapshot
# of their graph is exact: three facts, one topic and two entities make six
# vertices, and the facts' links to them five edges, which with the vertices are
# eleven stored nodes. Rewriting a fact rewrites the same nodes, so the counts
# hold across reruns against a long-lived server.
_STATS_FACTS = (
    ("the heron fishes at dusk", ["birds"], ["heron"]),
    ("the egret wades beside the heron", ["birds"], ["heron", "egret"]),
)
_STATS_VECTOR_FACT = "the marsh floods in spring"

# Two facts sharing the word "tide" and a topic hub, so a single recall returns
# more than one hit and the ordering and count assertions have something to
# work with.
_TIDE_TOPIC = "tides"
_TIDE_FACTS = {
    "spring": "a spring tide follows the new moon",
    "neap": "a neap tide follows the first quarter",
}


def _encode(text: str) -> list[float]:
    """Encode ``text`` as a deterministic unit vector of _VECTOR_DIM floats.

    A real implementation of the embedder contract (any
    ``callable(text) -> Sequence[float]``), not a mock, so integration tests
    drive the client's real embedding path. Deriving the components from a
    digest keeps the vector stable across runs and processes, so a test can
    store a fact and recall it by re-encoding the same text.

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

    Fails through ``pytest.fail`` when the server never comes up, so every test
    that needs it errors with one clear message rather than its own connection
    error.

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

    Separate from ``client`` on purpose: the plain client must keep proving that
    the SDK works with no embedder configured at all, which is how most callers
    start out.

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

    The recall runs at depth 0, so only the text and vector indices answer.

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

    The first vector into a graph decides that graph's dimension, so tests that
    depend on it already being _VECTOR_DIM — the mismatch tests in particular —
    must not race the write that establishes it.

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

    Shared by every response-shape assertion so they all read the same live
    response rather than each provoking their own. The recall carries the bare
    stop word "the" beside its phrase so the response also carries a warning,
    for the assertion that reads one; the term matches nothing and changes no
    hit.

    Args:
        client: the plain client to write and recall through.

    Returns:
        The RecallResult the server produced for a two-hit recall.
    """
    for phrase in _TIDE_FACTS.values():
        client.remember(phrase, graph=_MODELS_GRAPH, topics=[_TIDE_TOPIC])
    return client.recall("tide", "the", graph=_MODELS_GRAPH, depth=2)


@pytest.fixture(scope="module")
def vector_tide_result(embedding_client, vector_graph):
    """Return a RecallResult that the vector index seeded alongside the text index.

    Vector-seeded results travel a different path through the engine, so the
    envelope they arrive in is worth parsing separately from the text one.

    Args:
        embedding_client: the client that vectorises implicitly.
        vector_graph: the graph whose dimension is already established.

    Returns:
        The RecallResult of a recall whose query phrase was embedded.
    """
    fact = "the tide table is printed each spring"
    embedding_client.remember(fact, graph=vector_graph, topics=[_TIDE_TOPIC])
    return embedding_client.recall("tide", graph=vector_graph, query=fact)
