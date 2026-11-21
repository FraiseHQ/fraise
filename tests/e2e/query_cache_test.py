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

"""Plan-cache keying, end to end.

On a hash hit, the engine uses the cached query object (engine.Plan). So the
cache key must fold in everything that changes the answer, including the bound
vector, the since/until bounds and the explain prefix. If Recall.Hash left
them out, two recalls that differ only in those fields would share a key. The
second would silently run with the first one's values.

Every test sends the colliding pair back to back. The first query's cached
entry is then the one the second would wrongly reuse.
"""

import pytest


def test_identical_recall_text_rebinds_vector(query, vector):
    """Two recalls with byte-identical query text but different `vec:$v`
    bindings must each search with their own vector.

    Each fact is stored with its own embedding. The recall term matches no
    stored text, so only the vector seeds the search. The top hit then shows
    which vector was used. With a stale cached plan, the second recall would
    surface the alpha fact, because the cached query object would still carry
    the alpha vector.
    """
    graph = 6
    fact_a = "the cache probe fact alpha"
    fact_b = "the cache probe fact bravo"
    vec_a = vector(value=0.9)
    vec_b = vector(value=-0.9)

    for fact, vec in ((fact_a, vec_a), (fact_b, vec_b)):
        status, body = query(
            f"remember@{graph} '{fact}' vec:$v topic:cacheprobe",
            parameters={"v": vec},
        )
        assert status == 200, body.get("error")

    text = f"recall@{graph} zzzcacheprobe vec:$v"

    status, body = query(text, parameters={"v": vec_a})
    assert status == 200, body.get("error")
    hits = body["results"]["hits"]
    assert hits
    assert hits[0]["value"] == fact_a, (
        f"recall with vector A should surface the alpha fact; got {hits}"
    )

    status, body = query(text, parameters={"v": vec_b})
    assert status == 200, body.get("error")
    hits = body["results"]["hits"]
    assert hits
    assert hits[0]["value"] == fact_b, (
        f"recall with vector B still seeded by vector A (stale cached plan); got {hits}"
    )


def test_recall_time_bound_not_reused_across_queries(query):
    """Two recalls that differ only in their `until:` literal must each apply
    their own bound.

    The fact is written now. So `until:0s` (bound = now) includes it, and
    `until:1w` (bound = a week ago) excludes it. If both queries shared a cache
    key, the second would run with the first's bound and return the fact
    anyway.
    """
    graph = 6
    fact = "the cache probe fact charlie"

    status, body = query(f"remember@{graph} '{fact}' topic:cacheprobe")
    assert status == 200, body.get("error")

    status, body = query(f"recall@{graph} charlie until:0s")
    assert status == 200, body.get("error")
    values = [hit["value"] for hit in body["results"]["hits"]]
    assert fact in values, f"until:0s should include the fresh fact; got {values}"

    status, body = query(f"recall@{graph} charlie until:1w")
    assert status == 200, body.get("error")
    values = [hit["value"] for hit in body["results"]["hits"]]
    assert fact not in values, (
        "until:1w returned a fact written today — the engine reused the "
        f"previous query's time bound (stale cached plan); got {values}"
    )


@pytest.mark.parametrize("explained", [[False, True, False], [True, False, True]])
def test_explain_prefix_not_reused_across_queries(query, explained):
    """A recall and the same recall explained each answer in their own envelope.

    The two statements differ only in the prefix, so they plan to the same
    recall but for its explain flag. Were the flag left out of the cache key,
    whichever ran first would answer for both: a plain recall would carry
    the explain blocks, or an explained one would come back without them.
    The pair runs in both orders, and back again, so either cached entry is
    the one the next statement would wrongly reuse.
    """
    graph = 6
    fact = "the cache probe fact delta"

    status, body = query(f"remember@{graph} '{fact}' topic:cacheprobe")
    assert status == 200, body.get("error")

    for explain in explained:
        text = f"{'explain ' if explain else ''}recall@{graph} delta"
        status, body = query(text)
        assert status == 200, body.get("error")
        results = body["results"]
        values = [hit["value"] for hit in results["hits"]]
        assert fact in values, f"{text!r}: want the fresh fact; got {values}"
        assert ("explain" in results) is explain, (
            f"{text!r}: the query's explain block does not follow the prefix "
            f"(stale cached plan); got {results}"
        )
        for hit in results["hits"]:
            assert ("explain" in hit) is explain, (
                f"{text!r}: the hit's explain block does not follow the prefix "
                f"(stale cached plan); got {hit}"
            )
