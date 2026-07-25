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
