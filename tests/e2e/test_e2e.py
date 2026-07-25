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


def test_recall_on_empty_graph(query):
    status, body = query("recall nothingindexedyet")
    assert status == 200
    results = body["results"]
    assert results is not None
    assert results["Count"] == 0
    assert results["Hits"] == []


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
    assert body["results"]["Count"] > 0, "recall found nothing, want the remembered fact"


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
        hits = body["results"]["Hits"]
        values = [hit["Node"]["Value"] for hit in hits]
        assert BIRD_FACTS[keyword] in values, (
            f"recall {keyword!r} lost its fact under load; got {values}"
        )

    # 4. After the storm the graph must be intact: each fact still retrievable
    #    on its own.
    for keyword, phrase in BIRD_FACTS.items():
        status, body = query(f"recall@{graph} {keyword}")
        assert status == 200, body.get("error")
        values = [hit["Node"]["Value"] for hit in body["results"]["Hits"]]
        assert phrase in values, f"fact {keyword!r} missing after concurrency; got {values}"
