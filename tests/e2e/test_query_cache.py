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

"""Plan-cache keying end to end.

The engine substitutes a cached query object on a hash hit (engine.Plan), so
the cache key must fold in everything that changes the result set. These are
regression tests for the bug where Recall.Hash omitted the bound vector and
the since/until bounds: the second of two recalls identical in text but
different in those fields silently executed with the first one's values.

Both tests issue the colliding pair back to back so the first response is the
cached entry the second would wrongly reuse.
"""


def test_identical_recall_text_rebinds_vector(query, vector):
    """Two recalls with byte-identical query text but different `vec:$v`
    bindings must each search with their own vector.

    Each fact is stored with its own embedding; the recall term matches no
    stored text, so only the vector seeds the search and the top hit names
    which vector was actually used. Before the fix, the second recall surfaced
    the alpha fact: the cached query object still carried the alpha vector."""
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

    # Identical text both times — only the bound parameter differs.
    text = f"recall@{graph} zzzcacheprobe vec:$v depth:1"

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
    """Two recalls differing only in their `until:` literal must each apply
    their own bound.

    The fact is written now, so `until:0s` (bound = now) includes it and
    `until:1w` (bound = a week ago) excludes it. Before the fix both queries
    shared a cache key, so the second ran with the first's bound and returned
    the fact anyway."""
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
