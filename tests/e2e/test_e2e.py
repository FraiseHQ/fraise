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

"""End-to-end tests for the Fraise HTTP API.

Run by the `e2e` docker compose service against the `fraise` service over
the compose network (see docker-compose.yml and `make test-e2e`).
"""

import random
from concurrent.futures import ThreadPoolExecutor

import pytest
import requests
import numpy as np

REQUEST_TIMEOUT_SECONDS = 15


def test_health_check(base_url):
    response = requests.get(f"{base_url}/", timeout=REQUEST_TIMEOUT_SECONDS)

    assert response.status_code == 200
    assert response.json() == {"status": "ok"}


def test_query_rejects_malformed_json(base_url):
    response = requests.post(
        f"{base_url}/api/v1/q",
        data="{not json",
        headers={"Content-Type": "application/json"},
        timeout=REQUEST_TIMEOUT_SECONDS,
    )

    assert response.status_code == 400


def test_query_rejects_unparsable_query(query):
    status, body = query("bogus nonsense")

    assert status == 400
    assert body.get("error"), "expected a parse error message"


def test_query_rejects_out_of_range_graph(base_url):
    """A selector past the allocated graph range is a fast client error, not a
    hang. Graph 9 is above the eight graphs the store allocates."""
    response = requests.post(
        f"{base_url}/api/v1/q",
        json={"query": "recall@9 anything"},
        timeout=REQUEST_TIMEOUT_SECONDS,
    )

    assert response.status_code == 400
    assert response.json().get("error"), "expected an out-of-range error message"


def test_recall_on_empty_graph(query):
    status, body = query("recall nothingindexedyet")
    assert status == 200
    results = body["results"]
    assert results is not None
    assert results["count"] == 0
    assert results["hits"] == []


def test_recall_with_clauses(query):
    status, body = query("recall@2 anna bob entity:alice topic:job top:10 depth:5")
    assert status == 200
    assert body["results"] is not None


def test_remember_is_accepted(query):
    status, _ = query("remember@1 'anne loves the color orange' topic:color entity:anne")
    assert status == 200


def test_remember_then_recall(query):
    """The real end-to-end round trip: store a fact, then find it."""
    status, body = query("remember@3 'the parrot is turquoise' topic:color")
    assert status == 200, body.get("error")

    status, body = query("recall@3 parrot")
    assert status == 200, body.get("error")
    assert body["results"]["count"] > 0, "recall found nothing, want the remembered fact"


def test_parameterised_query_not_implemented(base_url):
    response = requests.post(
        f"{base_url}/api/v1/qp", json={}, timeout=REQUEST_TIMEOUT_SECONDS
    )

    assert response.status_code == 501


def test_concurrent_queries(base_url):
    """Hammer the endpoint with parallel reads and writes to shake out
    scheduler deadlocks and races in the request path."""

    def send(i: int):
        text = "recall anna"
        if i % 4 == 0:
            text = f"remember@1 'concurrent fact {i}' topic:load"
        response = requests.post(
            f"{base_url}/api/v1/q",
            json={"query": text},
            timeout=REQUEST_TIMEOUT_SECONDS,
        )
        return i, response.status_code

    with ThreadPoolExecutor(max_workers=10) as pool:
        results = list(pool.map(send, range(20)))

    failures = [(i, status) for i, status in results if status != 200]
    assert not failures, f"non-200 responses: {failures}"


# A batch of distinct facts, each carrying a unique keyword so a recall can
# target exactly one of them. They share a topic so the write path also
# exercises relationship creation.
BIRD_FACTS = {
    "parrot": "the parrot is turquoise",
    "raven": "the raven is midnight black",
    "canary": "the canary is bright yellow",
    "flamingo": "the flamingo is pink",
    "peacock": "the peacock is iridescent",
    "robin": "the robin has a red breast",
    "magpie": "the magpie loves shiny things",
    "owl": "the owl hunts at night",
}


def test_many_writes_then_concurrent_reads(query, base_url):
    """Store a batch of distinct facts, then read them back under heavy
    parallelism and verify every reader still gets its own fact intact.

    This stresses the read/write path together: it should surface data races
    or corruption in the scheduler/graph (missing hits, wrong values, 5xx, or
    a server that stops responding) that a single-threaded round trip hides.
    """
    graph = 5

    # 1. Write every fact. Writes are serialized server-side, so do them
    #    sequentially and confirm each is accepted before reading.
    for keyword, phrase in BIRD_FACTS.items():
        status, body = query(f"remember@{graph} '{phrase}' topic:birds")
        assert status == 200, body.get("error")

    # 2. Build a shuffled workload where each keyword is recalled several times,
    #    then fire them all concurrently.
    workload = [kw for kw in BIRD_FACTS for _ in range(8)]
    random.shuffle(workload)

    def recall(keyword: str):
        response = requests.post(
            f"{base_url}/api/v1/q",
            json={"query": f"recall@{graph} {keyword}"},
            timeout=REQUEST_TIMEOUT_SECONDS,
        )
        return keyword, response.status_code, response.json()

    with ThreadPoolExecutor(max_workers=16) as pool:
        results = list(pool.map(recall, workload))

    # 3. Every concurrent read must succeed and return its own fact.
    for keyword, status, body in results:
        assert status == 200, f"recall {keyword!r} failed: {body}"
        hits = body["results"]["hits"]
        values = [hit["value"] for hit in hits]
        assert BIRD_FACTS[keyword] in values, (
            f"recall {keyword!r} lost its fact under load; got {values}"
        )

    # 4. After the storm the graph must be intact: each fact still retrievable
    #    on its own.
    for keyword, phrase in BIRD_FACTS.items():
        status, body = query(f"recall@{graph} {keyword}")
        assert status == 200, body.get("error")
        values = [hit["value"] for hit in body["results"]["hits"]]
        assert phrase in values, f"fact {keyword!r} missing after concurrency; got {values}"


# Four facts sharing a single topic, each with a unique keyword. This is a
# star: every fact hangs off the same "planets" hub. Recall returns facts, not
# the hub, so from a seed fact the hub is one hop away (depth 1, invisible) and
# the sibling facts are two hops away (depth 2). That makes the exact result
# counts a clean function of depth and top.
PLANET_TOPIC = "planets"
PLANET_FACTS = {
    "mercury": "mercury is the smallest planet",
    "venus": "venus is the hottest planet",
    "mars": "mars is the red planet",
    "jupiter": "jupiter is the largest planet",
}


@pytest.fixture(scope="module")
def planets_graph(query):
    """Populate a dedicated graph with the planet star and return its id.

    A fact is keyed by its value, so these writes are idempotent: re-running the
    suite against a long-lived server leaves the counts unchanged. Graph 7 is
    used by no other test, so its contents are fully known here.
    """
    graph = 7
    for phrase in PLANET_FACTS.values():
        status, body = query(f"remember@{graph} '{phrase}' topic:{PLANET_TOPIC}")
        assert status == 200, body.get("error")
    return graph


def _recall_count(query, text):
    status, body = query(text)
    assert status == 200, body.get("error")
    return body["results"]["count"]


def test_recall_depth_controls_reach(planets_graph, query):
    """depth bounds how far the walk leaves the seed, and thus the count.

    Note: depth:0 is not exercised — the query parser treats a 0 as "unset" and
    substitutes the configured default, so it cannot be expressed.
    """
    g = planets_graph
    n = len(PLANET_FACTS)

    # depth 1: only the seed fact. The shared topic hub is one hop away, but the
    # hub is graph structure, not a result, so nothing else surfaces.
    assert _recall_count(query, f"recall@{g} mercury depth:1") == 1
    # depth 2: the walk crosses the hub and reaches every sibling fact.
    assert _recall_count(query, f"recall@{g} mercury depth:2") == n
    # A single-topic star has nothing beyond two hops, so deeper adds nothing.
    assert _recall_count(query, f"recall@{g} mercury depth:3") == n
    # With no depth clause the configured default (2) applies, so a bare recall
    # already reaches the whole star. This guards the default-depth wiring.
    assert _recall_count(query, f"recall@{g} mercury") == n


def test_recall_top_truncates_results(planets_graph, query):
    """top caps the number of ranked results returned, never pads."""
    g = planets_graph
    n = len(PLANET_FACTS)

    # At depth 2 all facts are reachable; top decides how many come back.
    assert _recall_count(query, f"recall@{g} mercury depth:2 top:1") == 1
    assert _recall_count(query, f"recall@{g} mercury depth:2 top:2") == 2
    assert _recall_count(query, f"recall@{g} mercury depth:2 top:3") == 3
    # top larger than the number available returns everything, not padding.
    assert _recall_count(query, f"recall@{g} mercury depth:2 top:10") == n


def test_recall_depth_one_returns_only_the_seed(planets_graph, query):
    """The depth:1 result is exactly the seed fact, by value."""
    g = planets_graph
    status, body = query(f"recall@{g} mercury depth:1")
    assert status == 200, body.get("error")
    values = [hit["value"] for hit in body["results"]["hits"]]
    assert values == [PLANET_FACTS["mercury"]]


def test_vector_calls(query):
    DIM: int = 128
    vector = np.ones((1,DIM)).tolist()

    body, status = query("remember@3 'the parrot is turquoise' vec:$v topic:color",
        parameters = {"v": vector}
    )


def test_vector_calls_incompatible_size(query):
    DIM: int = 128
    vector = np.ones((1,DIM)).tolist()

    body, status = query("remember@3 'the parrot is turquoise' vec:$v topic:color",
        parameters = {"v": vector}
    )

