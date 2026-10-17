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

"""Vector search, end to end.

Covers writes carrying embeddings, recall seeded by the vector index alone,
parameter and dimension validation, and semantic search with a real HuggingFace
model.

The API takes the embedding out-of-band. `vec:$v` in the query is a
placeholder, bound by name to the flat float list in `parameters["v"]`. The
`vector` fixture builds those lists at the suite-wide dimension (conftest
VECTOR_DIM). That fixes the index dimension of the graphs written here.
"""

import pytest


def test_remember_with_vector_is_accepted(query, vector):
    """A remember carrying a vector parameter is accepted.

    It exercises the write path's vector-index insert (stream.Commit ->
    VectorIndex.Insert).
    """
    status, body = query(
        "remember@6 'the parrot is turquoise' vec:$v topic:color",
        parameters={"v": vector()},
    )

    assert status == 200, body.get("error")


def test_remember_vector_then_recall_by_vector(query, vector):
    """A fact stored with an embedding is recalled by that embedding alone.

    This is the vector round trip. The recall term appears in no stored fact,
    so the text index yields no seeds. The fact can only surface if the vector
    index seeds the search from `vec:$v`. That proves vector search works end
    to end, not only that the write was accepted.
    """
    graph = 6
    phrase = "the kingfisher is electric blue"
    embedding = vector(value=0.5)

    status, body = query(
        f"remember@{graph} '{phrase}' vec:$v topic:color",
        parameters={"v": embedding},
    )
    assert status == 200, body.get("error")

    status, body = query(
        f"recall@{graph} zzznomatch vec:$v", parameters={"v": embedding}
    )
    assert status == 200, body.get("error")
    values = [hit["value"] for hit in body["results"]["hits"]]
    assert phrase in values, (
        f"vector recall did not surface the remembered fact; got {values}"
    )


def test_recall_missing_vector_parameter_is_rejected(query):
    """A query that names `vec:$v` without a matching parameter is a 400.

    Binding fails at parse time. The 400 names the missing placeholder. It is
    not a 500 or a silent empty result.
    """
    status, body = query("recall@6 parrot vec:$v")

    assert status == 400
    error = body.get("error", "")
    assert "$v" in error, (
        f"expected the error to name the missing parameter, got {error!r}"
    )


def test_remember_vector_incompatible_size_is_rejected(query, vector, vector_dim):
    """A write whose vector size differs from the graph's dimension is a 400.

    The first vector inserted into a graph fixes its embedding dimension. A
    later vector of another size is a 400 naming the expected and supplied
    dimensions. It is a client error, not a 500, so callers can tell their bad
    input from a server fault.

    Both writes go to graph 4. Its other writes, the forest-bound test in
    stats_test.py and the dimension probes below, use the same suite-wide
    dimension. So the dimension is the same whichever write comes first, even
    across reruns against a long-lived server.
    """
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


@pytest.mark.parametrize(
    "text",
    [
        "recall@4 dimension vec:$v",
        "recall@4 vec:$v",
        "recall@4 topic:size vec:$v",
    ],
)
def test_recall_vector_incompatible_size_is_rejected(query, vector, vector_dim, text):
    """A recall vector of the wrong width is a 400 naming both dimensions.

    It fails exactly as a write does. A 200 from the text index alone would
    silently drop the semantic half of the question. "dimension" matches the
    fact written below, so the first shape would come back with a plausible
    hit. A vector on its own, and a vector beside an anchor, fail the same way.

    The write fixes graph 4 at the suite-wide dimension, idempotently, as in
    test_remember_vector_incompatible_size_is_rejected.
    """
    status, body = query(
        "remember@4 'establish the vector dimension' vec:$v topic:size",
        parameters={"v": vector()},
    )
    assert status == 200, body.get("error")

    status, body = query(text, parameters={"v": vector(vector_dim // 2)})
    assert status == 400, (
        f"{text!r}: expected a client error, not an answer without the vector, "
        f"got {status}: {body}"
    )
    error = body.get("error", "")
    assert f"expects {vector_dim}, got {vector_dim // 2}" in error, (
        f"{text!r}: expected the error to name both dimensions, got {error!r}"
    )


def test_explain_rejects_a_recall_vector_of_the_wrong_size(
    query, explain, vector, vector_dim
):
    """/explain answers a mismatched recall vector with the same 400 as /q.

    Both run the same pipeline. An explanation of a ranking the vector never
    entered would explain the wrong question.
    """
    status, body = query(
        "remember@4 'establish the vector dimension' vec:$v topic:size",
        parameters={"v": vector()},
    )
    assert status == 200, body.get("error")

    status, body = explain(
        "recall@4 dimension vec:$v", parameters={"v": vector(vector_dim // 2)}
    )
    assert status == 400, f"expected a client error, got {status}: {body}"
    assert f"expects {vector_dim}, got {vector_dim // 2}" in body.get("error", "")


def test_recall_vector_on_a_graph_without_vectors_is_answered(
    query, vector, planets_graph, planet_facts
):
    """A recall vector on a graph with no vectors is answered, not rejected.

    A graph no vector was ever written to has no dimension to disagree with.
    The vector seeds nothing, and the text match comes back as it would without
    it. The planet graph never gets a vector (see the graph map in
    conftest.py), so any width stands in for a vector from another model.
    """
    keyword, fact = next(iter(planet_facts.items()))
    status, body = query(
        f"recall@{planets_graph} {keyword} vec:$v", parameters={"v": vector(3)}
    )
    assert status == 200, body.get("error")
    values = [hit["value"] for hit in body["results"]["hits"]]
    assert fact in values, f"want the text match {fact!r}; got {values}"


@pytest.mark.embeddings
def test_vector_search_with_real_embeddings(query, embedding_docs):
    """End-to-end semantic search with a real HuggingFace embedding model.

    The test embeds a few documents with a small model, loaded through the
    plain transformers API (AutoTokenizer + AutoModel, mean-pooled). It stores
    each document with its vector. Then it recalls with the embedding of a
    phrase related in meaning. The closest document must come back, and the
    API's scores must be floating-point numbers.

    It can be skipped three ways. The `embeddings` marker deselects it
    (`pytest -m "not embeddings"`). importorskip skips it when transformers or
    torch is not installed. It also skips when the model cannot be loaded, for
    example in an offline CI container that cannot download it from the
    HuggingFace Hub.
    """
    transformers = pytest.importorskip("transformers")
    torch = pytest.importorskip("torch")

    model_id = "google/bert_uncased_L-2_H-128_A-2"
    try:
        tokenizer = transformers.AutoTokenizer.from_pretrained(model_id)
        model = transformers.AutoModel.from_pretrained(model_id)
    except OSError as exc:
        # transformers raises OSError ("Can't load the model for ...") when the
        # weights are unavailable: no network, no cache, or a blocked Hub. That
        # is a limit of the environment, not a test failure, so skip.
        pytest.skip(f"embedding model unavailable: {exc}")
    model.eval()

    def embed(text: str) -> list[float]:
        # Plain transformers gives per-token hidden states. A sentence
        # embedding is their attention-masked mean, L2-normalised, the standard
        # pooling for this model. .tolist() yields a flat list of Python
        # floats, the shape the API binds to vec:$v.
        enc = tokenizer(text, padding=True, truncation=True, return_tensors="pt")
        with torch.no_grad():
            hidden = model(**enc).last_hidden_state
        mask = enc["attention_mask"].unsqueeze(-1).float()
        pooled = (hidden * mask).sum(1) / mask.sum(1).clamp(min=1e-9)
        pooled = torch.nn.functional.normalize(pooled, p=2, dim=1)
        return pooled[0].tolist()

    graph = 3
    for topic, text in embedding_docs.items():
        status, body = query(
            f"remember@{graph} '{text}' vec:$v topic:{topic}",
            parameters={"v": embed(text)},
        )
        assert status == 200, body.get("error")

    # A phrase close in meaning to the cat document, sharing none of its words.
    # The recall keyword matches no stored text, and the recall names no
    # anchor. So only the vector index decides the result.
    query_vec = embed("a sleepy kitten dozing in the afternoon sun")
    status, body = query(
        f"recall@{graph} zzznomatch vec:$v depth:0",
        parameters={"v": query_vec},
    )

    assert status == 200, body.get("error")

    hits = body["results"]["hits"]
    assert hits, "vector search returned no hits"
    assert hits[0]["value"] == embedding_docs["cat"], (
        f"nearest document should be the cat one; got {hits[0]['value']!r}"
    )

    # Every score is a JSON number, and the ranking produces fractional scores.
    # JSON renders an exact 1.0 as `1`, which decodes to a Python int. So the
    # top score may be an int while lower ranks are floats. The test asserts
    # that all scores are numeric and that real floats appear.
    for hit in hits:
        assert isinstance(hit["score"], (int, float)), (
            f"score not numeric: {hit['score']!r}"
        )
    assert any(isinstance(hit["score"], float) for hit in hits), (
        f"expected floating-point scores, got {[type(h['score']).__name__ for h in hits]}"
    )


def test_recall_fuses_text_and_vector_additively(
    query, vector, explain, krakatoa_facts
):
    """The excess scorer fuses channels by adding mass, not by rank votes.

    A fact seen by both channels scores the sum of what each observed. A fact
    seen by one scores that channel's mass alone. Under rank votes, a fact
    leading neither list can top both leaders by consensus. Large hubs also
    manufacture consensus from their size.
    """
    graph = 6
    writes = (
        (krakatoa_facts["text"], None),
        (krakatoa_facts["both"], vector(value=-0.45)),
        (krakatoa_facts["vector"], vector(value=-0.5)),
    )
    for phrase, embedding in writes:
        if embedding is None:
            status, body = query(f"remember@{graph} '{phrase}'")
        else:
            status, body = query(
                f"remember@{graph} '{phrase}' vec:$v", parameters={"v": embedding}
            )
        assert status == 200, body.get("error")

    status, body = explain(
        f"recall@{graph} krakatoa ash vec:$v depth:0 top:10",
        parameters={"v": vector(value=-0.5)},
    )
    assert status == 200, body.get("error")
    hits = {h["value"]: h for h in body["results"]["hits"]}

    both = hits[krakatoa_facts["both"]]
    sources = sorted(c["source"] for c in both["contributions"])
    assert sources == [
        "text",
        "vector",
    ], f"the two-channel fact must carry both observations: {both['contributions']}"
    assert abs(both["score"] - sum(c["score"] for c in both["contributions"])) < 1e-3, (
        f"fusion is additive: {both}"
    )

    text_only = hits[krakatoa_facts["text"]]
    assert [c["source"] for c in text_only["contributions"]] == ["text"], text_only
    vec_only = hits[krakatoa_facts["vector"]]
    assert [c["source"] for c in vec_only["contributions"]] == ["vector"], vec_only
