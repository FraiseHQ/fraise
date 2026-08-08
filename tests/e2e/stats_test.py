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

"""The stats endpoint: per-graph shape snapshots, and the gauges as a window
into storage invariants that queries alone cannot observe.
"""


def test_stats_reports_per_graph_snapshots(get):
    """GET /api/v1/stats returns one snapshot per graph with the shape gauges
    (nodes, edges, vectors, forest entries).
    """
    response = get("/api/v1/stats")

    assert response.status_code == 200
    body = response.json()
    graphs = body["graphs"]
    assert len(graphs) > 0
    for i, g in enumerate(graphs):
        assert g["id"] == i
        for key in ("order", "size", "nodes", "vectors", "forest_entries"):
            assert key in g, f"graphs[{i}] missing {key!r}"


def test_vector_forest_stays_bounded_under_writes(get, query, vector):
    """Sustained writes must not bloat the vector forest: forest_entries stays
    within the flush-factor bound (2x) of live vectors.

    Regression test for the quadratic-bloat bug where every write re-inserted
    all staged vectors into the live forest (~W^2/2 entries after W writes);
    with 40 writes the pre-fix forest holds ~800 entries for ~40 vectors, so
    the 2x bound fails loudly on a regression. The bound is a per-graph
    invariant, so writes from other tests on this graph don't disturb it.
    """
    graph = 4
    writes = 40
    for i in range(writes):
        status, body = query(
            f"remember@{graph} 'bounded forest fact {i}' vec:$v topic:bloat",
            parameters={"v": vector(value=float(i + 1))},
        )
        assert status == 200, body.get("error")

    response = get("/api/v1/stats")
    assert response.status_code == 200

    g = response.json()["graphs"][graph]
    assert g["vectors"] >= writes
    assert g["forest_entries"] <= 2 * g["vectors"], (
        f"forest holds {g['forest_entries']} entries for {g['vectors']} live "
        f"vectors — exceeds the flush-factor bound, vector index is bloating"
    )
