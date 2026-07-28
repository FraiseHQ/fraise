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


# Three facts that all contain the keyword "comet" but are otherwise unrelated:
# each carries a *different* topic, so nothing connects them in the graph except
# the shared word. A recall for that word must therefore surface all three
# purely through the text index.
#
# Every other recall test uses a keyword unique to a single fact, so the text
# search there always matches exactly one document — a regression that capped
# text search at one hit would pass the whole rest of the suite unnoticed. This
# test is the one that would catch it, on its own graph (0) so the count is
# fully determined here.
COMET_FACTS = {
    "the comet streaked past mars": "astronomy",
    "children watched the comet at dawn": "memory",
    "the comet will not return for centuries": "time",
}


def test_recall_returns_every_document_sharing_a_keyword(query):
    graph = 0
    for phrase, topic in COMET_FACTS.items():
        status, body = query(f"remember@{graph} '{phrase}' topic:{topic}")
        assert status == 200, body.get("error")

    status, body = query(f"recall@{graph} comet top:10")
    assert status == 200, body.get("error")

    values = {hit["value"] for hit in body["results"]["hits"]}
    assert set(COMET_FACTS) <= values, (
        "recall by a shared keyword must return every matching fact, not just "
        f"one; want all of {set(COMET_FACTS)}, got {values}"
    )


def test_recall_unions_matches_across_keywords(query):
    """A recall naming several keywords returns the union of their matches, so a
    single query yields multiple results.

    The two facts share no word and carry different topics, so nothing links
    them in the graph. "saturn" matches only the first, "neptune" only the
    second; recalling both keywords must return both facts. Graph 2 is only ever
    recalled elsewhere in the suite, never written, so these are its only facts.
    """
    graph = 2
    facts = {
        "saturn has bright rings": "rings",
        "neptune is a deep blue giant": "orbit",
    }
    for phrase, topic in facts.items():
        status, body = query(f"remember@{graph} '{phrase}' topic:{topic}")
        assert status == 200, body.get("error")

    status, body = query(f"recall@{graph} saturn neptune top:10")
    assert status == 200, body.get("error")

    values = {hit["value"] for hit in body["results"]["hits"]}
    assert set(facts) <= values, (
        f"recall across two keywords should union both matches; want {set(facts)}, got {values}"
    )


# Vector tests use graphs 4 and 6, which no other test writes to, so each
# graph's vector index dimension is fixed solely by these tests. The API takes
# the embedding out-of-band: `vec:$v` in the query is a placeholder bound by
# name from the flat float list in `parameters["v"]`.
VECTOR_DIM = 128


def _vector(dim: int, value: float = 1.0) -> list[float]:
    """A flat embedding of `dim` floats — the shape the API expects for a
    parameter. (np.ones(dim) is 1-D; .tolist() keeps it flat, not nested.)"""
    return np.full(dim, value, dtype=float).tolist()


def test_remember_with_vector_is_accepted(query):
    """A remember carrying a vector parameter is accepted and exercises the
    write path's vector-index insert (stream.Commit -> VectorIndex.Insert)."""
    status, body = query(
        "remember@6 'the parrot is turquoise' vec:$v topic:color",
        parameters={"v": _vector(VECTOR_DIM)},
    )

    assert status == 200, body.get("error")


def test_remember_vector_then_recall_by_vector(query):
    """The vector round trip: store a fact with an embedding, then recall it by
    that embedding alone.

    The recall term is a keyword that appears in no stored fact, so the text
    index yields no seeds. The fact can only surface if the vector index seeds
    the search from `vec:$v` — this is what proves vector search actually works
    end to end, not just that the write was accepted."""
    graph = 6
    phrase = "the kingfisher is electric blue"
    embedding = _vector(VECTOR_DIM, value=0.5)

    status, body = query(
        f"remember@{graph} '{phrase}' vec:$v topic:color",
        parameters={"v": embedding},
    )
    assert status == 200, body.get("error")

    # "zzznomatch" matches no fact's text, so only the vector can seed the recall.
    status, body = query(f"recall@{graph} zzznomatch vec:$v", parameters={"v": embedding})
    assert status == 200, body.get("error")
    values = [hit["value"] for hit in body["results"]["hits"]]
    assert phrase in values, (
        f"vector recall did not surface the remembered fact; got {values}"
    )


def test_recall_missing_vector_parameter_is_rejected(query):
    """A query references `vec:$v` but supplies no matching parameter. Binding
    fails at parse time, so the request is a 400 naming the missing placeholder,
    not a 500 or a silent empty result."""
    status, body = query("recall@6 parrot vec:$v")

    assert status == 400
    error = body.get("error", "")
    assert "$v" in error, f"expected the error to name the missing parameter, got {error!r}"


def test_remember_vector_incompatible_size_is_rejected(query):
    """The first vector inserted into a graph fixes that graph's embedding
    dimension; a later vector of a different size is rejected.

    Both writes go to graph 4, which no other test touches, so the first insert
    here establishes the dimension deterministically even across reruns against
    a long-lived server.

    NOTE: the current server surfaces this as a 500 with a generic
    "Error while comitting stream." message — the Commit error (which does name
    the expected vs actual dimension) is flattened into ErrCommitFailed by the
    scheduler's Stage step before it reaches the handler. This asserts today's
    behavior; tighten to 400 with the dimension detail if that path is fixed."""
    # Establish the graph's dimension.
    status, body = query(
        "remember@4 'establish the vector dimension' vec:$v topic:size",
        parameters={"v": _vector(VECTOR_DIM)},
    )
    assert status == 200, body.get("error")

    # A vector of a different size must be rejected, not silently dropped.
    status, body = query(
        "remember@4 'a differently sized vector' vec:$v topic:size",
        parameters={"v": _vector(VECTOR_DIM // 2)},
    )
    assert status == 500, f"expected the mismatched-dimension write to fail, got {status}: {body}"
    assert body.get("error"), "expected an error message on the rejected write"


# Documents on three clearly distinct subjects. A real sentence embedding places
# each far from the others, so a query close in meaning to one of them retrieves
# that one by vector alone. Graph 3 carries no other vectors, so its embedding
# dimension is fixed by this test.
# No apostrophes: a single-quoted query phrase has no escaping, so an apostrophe
# would close the phrase early and fail to parse.
EMBEDDING_DOCS = {
    "cat": "the tabby cat curled up and slept on the warm windowsill",
    "space": "the mars rover drilled into red rock to collect samples",
    "bread": "he kneaded the dough and baked a fresh loaf of sourdough",
}


@pytest.mark.embeddings
def test_vector_search_with_real_embeddings(query):
    """End-to-end semantic search with a real HuggingFace embedding model.

    Embeds a few documents with a small model loaded through the vanilla
    transformers API (AutoTokenizer + AutoModel, mean-pooled), stores each with
    its vector, then recalls with the embedding of a semantically related phrase
    and checks the closest document comes back — and that the scores the API
    returns are floating-point numbers.

    Skippable three ways: the `embeddings` marker (`pytest -m "not embeddings"`),
    importorskip when transformers/torch are not installed, and a skip when the
    model itself cannot be loaded — e.g. an offline CI container that cannot
    download it from the HuggingFace Hub.
    """
    transformers = pytest.importorskip("transformers")
    torch = pytest.importorskip("torch")

    model_id = "google/bert_uncased_L-2_H-128_A-2"
    try:
        tokenizer = transformers.AutoTokenizer.from_pretrained(model_id)
        model = transformers.AutoModel.from_pretrained(model_id)
    except Exception as exc:
        # Installed but the weights are not available (no network, no cache,
        # blocked Hub). That is an environment limitation, not a test failure.
        pytest.skip(f"embedding model unavailable: {exc}")
    model.eval()

    def embed(text: str) -> list[float]:
        # Vanilla transformers gives per-token hidden states; a sentence
        # embedding is the attention-masked mean over tokens, L2-normalized —
        # the standard pooling for this model. .tolist() yields a flat list of
        # Python floats, the shape the API binds to vec:$v.
        enc = tokenizer(text, padding=True, truncation=True, return_tensors="pt")
        with torch.no_grad():
            hidden = model(**enc).last_hidden_state
        mask = enc["attention_mask"].unsqueeze(-1).float()
        pooled = (hidden * mask).sum(1) / mask.sum(1).clamp(min=1e-9)
        pooled = torch.nn.functional.normalize(pooled, p=2, dim=1)
        return pooled[0].tolist()

    graph = 3
    for topic, text in EMBEDDING_DOCS.items():
        status, body = query(
            f"remember@{graph} '{text}' vec:$v topic:{topic}",
            parameters={"v": embed(text)},
        )
        assert status == 200, body.get("error")

    # A phrase close in meaning to the cat document, sharing none of its words.
    # The recall keyword matches no stored text, and depth:1 keeps the walk on
    # the seeds, so only the vector index decides the result.
    query_vec = embed("a sleepy kitten dozing in the afternoon sun")
    status, body = query(
        f"recall@{graph} zzznomatch vec:$v depth:1",
        parameters={"v": query_vec},
    )

    print(50*"-")
    print(body)
    print(50*"-")

    assert status == 200, body.get("error")

    hits = body["results"]["hits"]
    assert hits, "vector search returned no hits"
    assert hits[0]["value"] == EMBEDDING_DOCS["cat"], (
        f"nearest document should be the cat one; got {hits[0]['value']!r}"
    )

    # Outputs are floats. Every score is a JSON number, and the ranking produces
    # genuine fractional scores. (JSON renders an exact 1.0 as `1`, which decodes
    # to a Python int, so the top score may be int while lower ranks are float —
    # assert both that all are numeric and that real floats appear.)
    for hit in hits:
        assert isinstance(hit["score"], (int, float)), f"score not numeric: {hit['score']!r}"
    assert any(isinstance(hit["score"], float) for hit in hits), (
        f"expected floating-point scores, got {[type(h['score']).__name__ for h in hits]}"
    )
