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

"""Fixtures for the Python SDK integration suite.

Everything a test needs is a fixture here, and fixtures are the *only* way a
test gets it: a test module never imports from this file. A test directory is
not a package — importing across it depends on how pytest happened to put the
directory on sys.path, and it smuggles setup into modules that should read as
assertions. Values that look like constants (urls, graph ids, seed facts) are
fixtures for the same reason.

Exercises the real ``fraise_sdk`` client against a live server named by
FRAISE_URL — inside the docker compose network that is http://fraise:9876 —
waiting for its health check before any test runs.

Graph allocation. Tests pin their writes to specific graphs so result counts
stay deterministic, including across reruns against a long-lived server (a
fact is keyed by its value, so rewrites are idempotent). Files run in any
order, so tests sharing a graph must not depend on each other's facts. Keep
this map current when claiming a graph:

    0  remember/recall round trip           (client_test.py)
    1  vectors written through the client   (client_test.py, models_test.py)
    2  builder-generated query strings      (query_test.py)
    3  facts backing the response shape     (models_test.py)
"""

import hashlib
import math
import os
import time

import pytest
from fraise_sdk.client import FraiseClient

# The values behind the fixtures below. Private by convention *and* by the
# leading underscore: nothing outside this file reads them.
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
# HTTP. No apostrophes: the grammar cannot escape one inside a quoted phrase.
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
