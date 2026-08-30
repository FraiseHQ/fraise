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
the values behind them are private by convention and by the leading
underscore.

The file has two halves. The mocked half patches the client's own
`requests.Session` at its import site, so unit tests run with no server and
no daemon. The live half (from the "live server" banner down) backs the
tests marked ``integration``: a real client against the daemon named by
FRAISE_URL, health-checked before the first test. `-m "not integration"` is
the unit run and touches nothing live; `-m integration` needs the daemon up.
"""

import hashlib
import json
import math
import os
import time
from unittest.mock import MagicMock, patch

import pytest
from fraise_sdk.client import DEFAULT_BASE_URL, FraiseClient


def pytest_configure(config):
    # Register the marker so `-m integration` / `-m "not integration"` work
    # and pytest doesn't warn about an unknown mark.
    config.addinivalue_line(
        "markers",
        "integration: drives the real SDK against a live server; needs the daemon",
    )


_QUERY_URL = f"{DEFAULT_BASE_URL}/api/v1/q"
_NO_HITS = {"results": {"count": 0, "hits": []}}

# The shape the server sends for the grammar's one surviving ambiguity: a
# leading recall term that spells a keyword ran as a term search, and the
# warning names the clause it nearly is.
_SERVER_WARNING = (
    'parse warning at column 15: term "since" is also a keyword: write '
    "since:<value> if a clause was meant, or quote it ('since') to search "
    "for the word"
)


# -- addresses and payloads --------------------------------------------------


@pytest.fixture(scope="session")
def query_url():
    """The URL every query the client sends must be posted to."""
    return _QUERY_URL


@pytest.fixture(scope="session")
def no_hits():
    """A response body carrying an empty recall result."""
    return dict(_NO_HITS)


@pytest.fixture(scope="session")
def server_warning():
    """A parse warning exactly as the server words it."""
    return _SERVER_WARNING


# -- the patched session -----------------------------------------------------


def _arm(session, body: dict, status_code: int = 200) -> MagicMock:
    """Arm ``session`` to answer the next POST with ``body``.

    Private: tests reach this through the ``respond`` fixture. The ``session``
    fixture needs it before any fixture argument could be injected, which is
    why it exists as a function at all.
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

    Patching rather than injecting keeps the client's own construction path —
    the one every caller takes — under test.

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
    """Callable arming a session to answer the next POST with a given body.

    Returns:
        ``callable(session, body, status_code=200) -> MagicMock`` — the mock
        response, for tests that want to assert on it directly.
    """
    return _arm


def _arm_get(session, body, status_code: int = 200) -> MagicMock:
    """Arm ``session`` to answer the next GET with ``body``.

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
    """Callable arming a session to answer the next GET with a given body.

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


# -- embedders ---------------------------------------------------------------


# The unit suite's embedder shape: len(text), 4 times — deterministic, so a
# test can predict the vector the client will send. Private, not a fixture:
# the `encode` fixture name belongs to the live half's real embedder below,
# and no unit test asks for this directly; callable_embedder is its channel.
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


# -- live server (integration fixtures) --------------------------------------
# Everything below backs the tests marked `integration`: a real client
# against the daemon named by FRAISE_URL. Nothing here runs — no waiting,
# no writes — unless an integration test actually requests a fixture.


_FRAISE_URL = os.environ.get("FRAISE_URL", "http://localhost:9876")
_WAIT_TIMEOUT_SECONDS = 30

_ROUND_TRIP_GRAPH = 0
_VECTOR_GRAPH = 1
_QUERY_GRAPH = 2
_MODELS_GRAPH = 3

# The dimension every vector in this suite is written with. The first vector
# inserted into a graph fixes that graph's dimension, and more than one file
# writes vectors, so they must agree — which is why this lives here and not in
# a single test file.
_VECTOR_DIM = 8

# A keyword no fact in any graph contains.
_NO_MATCH = "zzznomatchzzz"

# A server nothing is listening on: port 1 is reserved (tcpmux) and never bound
# in the test environment, so connecting to it fails immediately rather than
# hanging until a timeout.
_DEAD_URL = "http://127.0.0.1:1"

# Four facts on one topic hub, each with a unique keyword. Recall returns facts
# rather than hubs, so from any seed fact the hub is one hop away (invisible)
# and its siblings are two — which makes result counts an exact function of
# depth. Mirrors the star the e2e suite uses, through the SDK instead of raw
# HTTP.
_INSTRUMENT_TOPIC = "instruments"
_INSTRUMENT_FACTS = {
    "cello": "the cello is bowed and tuned in fifths",
    "flute": "the flute is blown across a lip plate",
    "timpani": "the timpani is struck with felt mallets",
    "harp": "the harp is plucked with both hands",
}

# Two facts on a shared hub, so a single recall returns more than one hit and
# the ordering and count assertions have something to work with.
_TIDE_TOPIC = "tides"
_TIDE_FACTS = {
    "spring": "a spring tide follows the new moon",
    "neap": "a neap tide follows the first quarter",
}


def _encode(text: str) -> list[float]:
    """Encode ``text`` as a deterministic unit vector of _VECTOR_DIM floats.

    Not a stand-in for anything: an integration test has to drive the client's
    real embedding path, and the SDK's extension point *is* any
    ``callable(text) -> Sequence[float]``, so this is a genuine implementation
    of that public contract rather than a mock of one. Deriving the components
    from a digest keeps it stable across runs and processes, which is what lets
    a test store a fact and then recall it by re-encoding the same text.

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
    """Block until the server answers its health check, or fail the session.

    Ends the run through ``pytest.fail`` when the server never comes up: every
    test here needs it, so one clear message beats a cascade of connection
    errors.

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


# -- addresses and constants -------------------------------------------------


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


# -- clients -----------------------------------------------------------------


@pytest.fixture(scope="session")
def client():
    """A FraiseClient pointed at a server confirmed to be up.

    Yields:
            FraiseClient: fraise client.
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
            FraiseClient: fraise client with an embedder attached.
    """
    fraise = FraiseClient(_FRAISE_URL, embedder=_encode)
    _await_server(fraise)
    yield fraise
    fraise.close()


@pytest.fixture(scope="session")
def recalled_values(client):
    """A helper that returns the values a single-keyword recall finds, seed only.

    Args:
        client: the plain client the helper recalls through.

    Returns:
        A ``callable(keyword, graph) -> list[str]`` of ranked values.
    """

    def _recalled_values(keyword: str, graph: int) -> list[str]:
        return [hit.value for hit in client.recall(keyword, graph=graph, depth=1)]

    return _recalled_values


# -- graphs ------------------------------------------------------------------


@pytest.fixture(scope="session")
def round_trip_graph():
    """The graph the plain remember/recall tests write to.

    Returns:
        The graph id.
    """
    return _ROUND_TRIP_GRAPH


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


# -- recall results ----------------------------------------------------------


@pytest.fixture(scope="module")
def tide_result(client):
    """Store the tide facts and return the RecallResult a two-hit recall parses.

    Shared by every response-shape assertion so they all read the same live
    response rather than each provoking their own.

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
    """Return a RecallResult that the vector index seeded, not the text index.

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
