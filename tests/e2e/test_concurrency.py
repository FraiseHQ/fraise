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

"""Parallel reads and writes against the request path, hunting scheduler
deadlocks, races, and data corruption that single-threaded round trips hide."""

import random
from concurrent.futures import ThreadPoolExecutor


def test_concurrent_queries(query):
    """Hammer the endpoint with parallel reads and writes to shake out
    scheduler deadlocks and races in the request path."""

    def send(i: int):
        text = "recall anna"
        if i % 4 == 0:
            text = f"remember@1 'concurrent fact {i}' topic:load"
        status, _ = query(text)
        return i, status

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


def test_many_writes_then_concurrent_reads(query):
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
        status, body = query(f"recall@{graph} {keyword}")
        return keyword, status, body

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
        assert phrase in values, (
            f"fact {keyword!r} missing after concurrency; got {values}"
        )
