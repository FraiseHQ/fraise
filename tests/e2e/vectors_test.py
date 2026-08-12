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

"""Vector search end to end: writes carrying embeddings, recall seeded by the
vector index alone, parameter/dimension validation, and semantic search with a
real HuggingFace model.

The API takes the embedding out-of-band: `vec:$v` in the query is a
placeholder bound by name from the flat float list in `parameters["v"]`. The
`vector` fixture builds those lists at the suite-wide dimension (conftest
VECTOR_DIM), which fixes the index dimension of the graphs written here.
"""

import pytest


def test_remember_with_vector_is_accepted(query, vector):
    """A remember carrying a vector parameter is accepted and exercises the
    write path's vector-index insert (stream.Commit -> VectorIndex.Insert).
    """
    status, body = query(
        "remember@6 'the parrot is turquoise' vec:$v topic:color",
        parameters={"v": vector()},
    )

    assert status == 200, body.get("error")


def test_remember_vector_then_recall_by_vector(query, vector):
    """The vector round trip: store a fact with an embedding, then recall it by
    that embedding alone.

    The recall term is a keyword that appears in no stored fact, so the text
    index yields no seeds. The fact can only surface if the vector index seeds
    the search from `vec:$v` — this is what proves vector search actually works
    end to end, not just that the write was accepted.
    """
    graph = 6
    phrase = "the kingfisher is electric blue"
    embedding = vector(value=0.5)

    status, body = query(
        f"remember@{graph} '{phrase}' vec:$v topic:color",
        parameters={"v": embedding},
    )
    assert status == 200, body.get("error")

    # "zzznomatch" matches no fact's text, so only the vector can seed the recall.
    status, body = query(
        f"recall@{graph} zzznomatch vec:$v", parameters={"v": embedding}
    )
    assert status == 200, body.get("error")
    values = [hit["value"] for hit in body["results"]["hits"]]
    assert phrase in values, (
        f"vector recall did not surface the remembered fact; got {values}"
    )


def test_recall_missing_vector_parameter_is_rejected(query):
    """A query references `vec:$v` but supplies no matching parameter. Binding
    fails at parse time, so the request is a 400 naming the missing placeholder,
    not a 500 or a silent empty result.
    """
    status, body = query("recall@6 parrot vec:$v")

    assert status == 400
    error = body.get("error", "")
    assert "$v" in error, (
        f"expected the error to name the missing parameter, got {error!r}"
    )


def test_remember_vector_incompatible_size_is_rejected(query, vector, vector_dim):
    """The first vector inserted into a graph fixes that graph's embedding
    dimension; a later vector of a different size is a 400 naming the expected
    and supplied dimensions — a client error, not a 500, so callers can tell
    their bad input from a server fault.

    Both writes go to graph 4, whose only other writes (the forest-bound test
    in test_stats.py) use the same suite-wide dimension, so the first insert
    here establishes the dimension deterministically even across reruns against
    a long-lived server.
    """
    # Establish the graph's dimension.
    status, body = query(
        "remember@4 'establish the vector dimension' vec:$v topic:size",
        parameters={"v": vector()},
    )
    assert status == 200, body.get("error")

    # A vector of a different size must be rejected, not silently dropped.
    status, body = query(
        "remember@4 'a differently sized vector' vec:$v topic:size",
        parameters={"v": vector(vector_dim // 2)},
    )
    assert status == 400, (
        f"expected the mismatched-dimension write to be a client error, got {status}: {body}"
    )
    error = body.get("error", "")
    assert f"expects {vector_dim}, got {vector_dim // 2}" in error, (
        f"expected the error to name the expected vs supplied dimension, got {error!r}"
    )


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
    except OSError as exc:
        # transformers raises OSError ("Can't load the model for ...") when the
        # weights are not available: no network, no cache, or a blocked Hub. That
        # is an environment limitation, not a test failure, so skip.
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
        assert isinstance(hit["score"], (int, float)), (
            f"score not numeric: {hit['score']!r}"
        )
    assert any(isinstance(hit["score"], float) for hit in hits), (
        f"expected floating-point scores, got {[type(h['score']).__name__ for h in hits]}"
    )


# Three facts engineered so the text and vector rankings disagree: the text
# index's clear favourite has no embedding, the vector index's exact nearest
# matches no query term, and one fact sits second in BOTH lists. The vector
# values cluster around -0.5, away from every other embedding on graph 6
# (1.0, 0.9, 0.5, -0.9), so the vector ranks below are fixed whatever else
# the suite has written.
KRAKATOA_TEXT = "krakatoa ash fell for days"  # both terms; no embedding
KRAKATOA_BOTH = "the ash cloud crossed the ocean"  # one term; near embedding
KRAKATOA_VEC = "sensors recorded the pressure wave"  # no terms; exact embedding


def test_recall_fuses_disagreeing_text_and_vector_by_consensus(query, vector):
    """When text and vector disagree, the fact both sources agree on wins.

    Text ranks: KRAKATOA_TEXT matches both query terms (rank 0), KRAKATOA_BOTH
    one (rank 1). Vector ranks: KRAKATOA_VEC is the query embedding exactly
    (rank 0), KRAKATOA_BOTH is the next-nearest (rank 1). Under rank fusion
    Σ 1/(60+rank) the consensus fact scores 1/61 + 1/61, above the 1/60 of
    either source's favourite — the fused order is topped by a fact that leads
    neither list. depth:1 keeps resident graph-6 facts from adding walk
    sightings, and residents rank no better than 2 in the vector list, so they
    score at most 1/62, below all three.
    """
    graph = 6
    writes = (
        (KRAKATOA_TEXT, None),
        (KRAKATOA_BOTH, vector(value=-0.45)),
        (KRAKATOA_VEC, vector(value=-0.5)),
    )
    for phrase, embedding in writes:
        if embedding is None:
            status, body = query(f"remember@{graph} '{phrase}'")
        else:
            status, body = query(
                f"remember@{graph} '{phrase}' vec:$v", parameters={"v": embedding}
            )
        assert status == 200, body.get("error")

    status, body = query(
        f"recall@{graph} krakatoa ash vec:$v depth:1 top:10",
        parameters={"v": vector(value=-0.5)},
    )
    assert status == 200, body.get("error")

    values = [hit["value"] for hit in body["results"]["hits"]]
    assert values[0] == KRAKATOA_BOTH, (
        f"the fact text and vector agree on must top the fused order; got {values}"
    )
    # The two single-source favourites tie at 1/60 and land next, in either
    # order (recency decay perturbs the tie by write timing).
    assert set(values[1:3]) == {
        KRAKATOA_TEXT,
        KRAKATOA_VEC,
    }, f"both single-source favourites should follow the consensus fact; got {values}"
