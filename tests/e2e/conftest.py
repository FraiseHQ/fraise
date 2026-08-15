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
    7  planet star                         (test_recall.py)
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
