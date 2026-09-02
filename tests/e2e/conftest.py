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

"""Shared fixtures for the Fraise end-to-end suite.

The suite targets the server named by FRAISE_URL — inside the docker compose
network that is http://fraise:9876 — and waits for its health check before
any test runs.

Graph allocation. Tests pin their writes to specific graphs so result counts
stay deterministic, including across reruns against a long-lived server (a
fact is keyed by its value, so rewrites are idempotent). Files run in any
order, so tests sharing a graph must not depend on each other's facts. Keep
this map current when claiming a graph:

    0  comet + quasar shared-keyword facts
       + monsoon/geyser ranking clusters  (test_recall.py, scoring_test.py)
    1  loose remembers + concurrent load   (test_recall.py, test_concurrency.py)
    2  union-across-keywords facts
       + pulsar explain probes             (test_recall.py, explain_test.py)
    3  real-embedding documents            (test_vectors.py)
    4  vector dimension + forest bound     (test_vectors.py, test_stats.py)
    5  bird facts + anchored recall
       + the topic-named fact
       + keyword-anchor + case-fold probes (test_concurrency.py, test_recall.py)
    6  vector round trip + cache probes
       + krakatoa fusion cluster          (test_vectors.py, test_query_cache.py)
    7  planet star
       + lantern/almanac depth-lane probe
       + tidepool + saltmarsh anchor probes (test_recall.py)
"""

import os
import time

import numpy as np
import pytest
import requests

BASE_URL = os.environ.get("FRAISE_URL", "http://localhost:9876").rstrip("/")

WAIT_TIMEOUT_SECONDS = 30
REQUEST_TIMEOUT_SECONDS = 15

# The dimension every plain-vector test writes with. The first vector inserted
# into a graph fixes that graph's index dimension, and graph 4 takes writes
# from two files (see the map above), so they must agree on it — which is why
# this lives here and not in a single test file.
VECTOR_DIM = 128


def pytest_configure(config):
    # Register the marker so `-m "not embeddings"` works and pytest doesn't warn
    # about an unknown mark. Tests carrying it need a real embedding model.
    config.addinivalue_line(
        "markers",
        "embeddings: requires a HuggingFace embedding model (sentence-transformers); skippable",
    )


@pytest.fixture(scope="session")
def base_url():
    """Base URL of a Fraise server that is confirmed to be up."""
    deadline = time.monotonic() + WAIT_TIMEOUT_SECONDS
    last_error = None
    while time.monotonic() < deadline:
        try:
            response = requests.get(f"{BASE_URL}/", timeout=2)
            if response.status_code == 200:
                return BASE_URL
            last_error = f"health check returned {response.status_code}"
        except requests.RequestException as exc:
            last_error = str(exc)
        time.sleep(0.5)
    pytest.fail(f"fraise server not reachable at {BASE_URL}: {last_error}")


@pytest.fixture(scope="session")
def request_timeout():
    """Per-request timeout for tests that make raw HTTP calls themselves."""
    return REQUEST_TIMEOUT_SECONDS


@pytest.fixture(scope="session")
def get(base_url):
    """Callable GETting a path on the server, returning the raw Response."""

    def _get(path: str):
        return requests.get(f"{base_url}{path}", timeout=REQUEST_TIMEOUT_SECONDS)

    return _get


@pytest.fixture(scope="session")
def query(base_url):
    """Callable posting a raw query string to /api/v1/q.

    Returns (status_code, decoded JSON body).
    """

    def _query(text: str, parameters: dict[str, object] | None = None):
        data = {"query": text}

        if parameters:
            data["parameters"] = parameters

        response = requests.post(
            f"{base_url}/api/v1/q",
            json=data,
            timeout=REQUEST_TIMEOUT_SECONDS,
        )
        return response.status_code, response.json()

    return _query


@pytest.fixture(scope="session")
def explain(base_url):
    """Callable posting a raw query string to /api/v1/explain.

    Same request shape as the `query` fixture, different endpoint: the
    response's hits carry a per-source contribution breakdown. Returns
    (status_code, decoded JSON body).
    """

    def _explain(text: str, parameters: dict[str, object] | None = None):
        data = {"query": text}

        if parameters:
            data["parameters"] = parameters

        response = requests.post(
            f"{base_url}/api/v1/explain",
            json=data,
            timeout=REQUEST_TIMEOUT_SECONDS,
        )
        return response.status_code, response.json()

    return _explain


@pytest.fixture(scope="session")
def vector_dim():
    """The suite-wide plain-vector dimension (see VECTOR_DIM)."""
    return VECTOR_DIM


@pytest.fixture(scope="session")
def vector():
    """Callable building a flat embedding of `dim` floats — the shape the API
    expects for a parameter. (np.full(dim, ...) is 1-D; .tolist() keeps it
    flat, not nested.)
    """

    def _vector(dim: int = VECTOR_DIM, value: float = 1.0) -> list[float]:
        return np.full(dim, value, dtype=float).tolist()

    return _vector


# Four facts sharing a single topic, each with a unique keyword. This is a
# star: every fact hangs off the same "planets" hub. Recall returns facts, not
# the hub, so from a seed fact the hub is one hop away (depth 1, invisible) and
# the sibling facts are two hops away (depth 2). That makes the exact result
# counts a clean function of depth and top.
_PLANET_GRAPH = 7
_PLANET_TOPIC = "planets"
_PLANET_FACTS = {
    "mercury": "mercury is the smallest planet",
    "venus": "venus is the hottest planet",
    "mars": "mars is the red planet",
    "jupiter": "jupiter is the largest planet",
}

# The depth-lane probe. Transmission needs two touched anchors of different
# concentration: a lone anchor's observed mass IS the background, so it holds
# no surplus and stays silent (which is what the planet star above shows). Here
# a tight "lanterns" cluster holds two of the query's three seeds while a
# larger "almanac" hub holds one of eight members, so the cluster clears the
# background and the hub does not. The cluster's third fact carries no query
# term at all: it can only be returned if an anchor transmitted to it, which
# makes it the probe that tells the two retrieval lanes apart.
#
# Shares graph 7 with the planet star: no fact here contains "mercury" or
# "planet" and no planet fact contains "lantern", so neither set can appear in
# the other's recalls.
_LANTERN_TOPIC = "lanterns"
_LANTERN_SILENT = "the quay is silent at dawn"
_LANTERN_CLUSTER = (
    "the lantern glows on the quay",
    "lantern light guides the ferry",
    _LANTERN_SILENT,
)
_ALMANAC_TOPIC = "almanac"
_ALMANAC_HUB = ("a lantern in the old almanac",) + tuple(
    f"unrelated almanac entry {i}" for i in range(7)
)

# The anchor-seeding probe. A fact filed under a topic and an entity at once,
# between a fact filed under the topic alone and one under the entity alone,
# so a recall naming both anchors and nothing else has a union to assemble, a
# duplicate to suppress and a fact to rank first — the one filed under both.
# Written in this order, so newest first is the reverse of it. No fact here
# contains "planet", "mercury" or "lantern", and nothing else on graph 7 is
# filed under these anchors, so the results are exact.
_TIDEPOOL_TOPIC = "tidepool"
_TIDEPOOL_ENTITY = "limpet"
_TIDEPOOL_FACTS = {
    "topic": "the tide leaves the rockpool warm",
    "both": "the limpet clamps down as the water drops",
    "entity": "a limpet returns to its home scar",
}
_TIDEPOOL_ANCHORS = {
    "topic": f"topic:{_TIDEPOOL_TOPIC}",
    "both": f"topic:{_TIDEPOOL_TOPIC} entity:{_TIDEPOOL_ENTITY}",
    "entity": f"entity:{_TIDEPOOL_ENTITY}",
}

# The default-top probe: one anchor holding more facts than the daemon's
# configured default-top (10 in tests/fraise.config.toml), so an anchor-only
# recall with no top: clause is visibly capped. Shares graph 7 with the probes above and
# contains none of their words.
_DEFAULT_TOP = 10
_SALTMARSH_TOPIC = "saltmarsh"
_SALTMARSH_FACTS = tuple(f"saltmarsh channel {i} was surveyed" for i in range(12))


@pytest.fixture(scope="session")
def planet_facts():
    """The planet star's facts, keyed by the unique keyword each contains."""
    return dict(_PLANET_FACTS)


@pytest.fixture(scope="module")
def planets_graph(query):
    """Populate a graph with the planet star and return its id.

    A fact is keyed by its value, so these writes are idempotent: re-running
    the suite against a long-lived server leaves the counts unchanged.
    """
    for phrase in _PLANET_FACTS.values():
        status, body = query(
            f"remember@{_PLANET_GRAPH} '{phrase}' topic:{_PLANET_TOPIC}"
        )
        assert status == 200, body.get("error")
    return _PLANET_GRAPH


@pytest.fixture(scope="session")
def lantern_silent():
    """The cluster fact containing no query term.

    It can only be recalled if an anchor transmitted mass to it, so it is what
    separates the floor lane from the excess lane in a result set.
    """
    return _LANTERN_SILENT


@pytest.fixture(scope="module")
def lantern_graph(query):
    """Seed the depth-lane cluster and its diluting hub, returning the graph id.

    Idempotent for the same reason as the planet star: facts are keyed by
    value.
    """
    for phrase in _LANTERN_CLUSTER:
        status, body = query(
            f"remember@{_PLANET_GRAPH} '{phrase}' topic:{_LANTERN_TOPIC}"
        )
        assert status == 200, body.get("error")
    for phrase in _ALMANAC_HUB:
        status, body = query(
            f"remember@{_PLANET_GRAPH} '{phrase}' topic:{_ALMANAC_TOPIC}"
        )
        assert status == 200, body.get("error")
    return _PLANET_GRAPH


@pytest.fixture(scope="session")
def tidepool_anchors():
    """The anchor-seeding probe's (topic, entity) pair."""
    return _TIDEPOOL_TOPIC, _TIDEPOOL_ENTITY


@pytest.fixture(scope="session")
def tidepool_facts():
    """The anchor-seeding probe's facts, keyed by what each is filed under
    ("topic", "both", "entity"), in write order."""
    return dict(_TIDEPOOL_FACTS)


@pytest.fixture(scope="module")
def tidepool_graph(query):
    """Seed the anchor-seeding probe and return its graph id.

    Idempotent for the same reason as the planet star: facts are keyed by
    value, and a rewrite refreshes the timestamp in the same order.
    """
    for filed, phrase in _TIDEPOOL_FACTS.items():
        status, body = query(
            f"remember@{_PLANET_GRAPH} '{phrase}' {_TIDEPOOL_ANCHORS[filed]}"
        )
        assert status == 200, body.get("error")
    return _PLANET_GRAPH


@pytest.fixture(scope="session")
def default_top():
    """The daemon's configured default-top (tests/fraise.config.toml)."""
    return _DEFAULT_TOP


@pytest.fixture(scope="session")
def saltmarsh_facts():
    """The default-top probe's facts, in write order."""
    return tuple(_SALTMARSH_FACTS)


@pytest.fixture(scope="module")
def saltmarsh_graph(query):
    """Seed the default-top probe and return its graph id. Idempotent: facts
    are keyed by value."""
    for phrase in _SALTMARSH_FACTS:
        status, body = query(
            f"remember@{_PLANET_GRAPH} '{phrase}' topic:{_SALTMARSH_TOPIC}"
        )
        assert status == 200, body.get("error")
    return _PLANET_GRAPH
