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

"""Opinionated ranking properties: the promises a recall's scores must keep,
whatever scorer is in charge. Each test states an opinion about what good
memory retrieval looks like — better matches outrank worse ones, the fact you
asked about outranks the ones merely connected to it, an anchor speaks only
when its members matched better than its size predicts — and pins the fused
order, not the plumbing. If a future scorer breaks one of these, that is a
product decision to make consciously, not a regression to discover in an
agent's answers.

(The one promise not pinned here is "recent memories outrank older ones" at
equal relevance: remember stamps time.Now(), so an HTTP test cannot create the
weeks-old fact needed to observe decay. That property is pinned by the unit
suite's decay tests.)
"""


def _ranked_hits(query, text, parameters=None):
    """Values and scores of a recall, checking the ranking invariants every
    response must satisfy: scores positive and non-increasing down the list.
    """
    status, body = query(text, parameters=parameters)
    assert status == 200, body.get("error")
    hits = body["results"]["hits"]
    scores = [hit["score"] for hit in hits]
    assert all(s > 0 for s in scores), f"scores must be positive: {scores}"
    assert scores == sorted(scores, reverse=True), (
        f"scores must not increase down the ranking: {scores}"
    )
    return [hit["value"] for hit in hits], scores


def test_matching_more_of_the_query_outranks_matching_less(query):
    """A fact matching both query terms must strictly outrank a fact matching
    one — relevance is the first-order signal, and coverage of the query is
    the text index's measure of it.
    """
    graph = 0
    both = "the monsoon flooded the delta"
    one = "the monsoon arrived early"
    for phrase in (both, one):
        status, body = query(f"remember@{graph} '{phrase}'")
        assert status == 200, body.get("error")

    values, scores = _ranked_hits(query, f"recall@{graph} monsoon delta depth:1")
    assert values == [both, one], f"two matching terms must beat one; got {values}"
    assert scores[0] > scores[1], (
        f"the better match must win strictly, not by tiebreak: {scores}"
    )


def test_the_fact_asked_about_outranks_its_neighbourhood(query):
    """The fact that actually matches the query must top the expansion pulled
    in around it. Graph context enriches an answer; it must never bury the
    answer itself.

    Expansion requires surplus: a small cluster concentrating the query's
    mass next to a bigger fair-share hub is the smallest shape where an
    anchor speaks. The cluster's silent member is funded — attenuated α² —
    and lands behind both facts that actually matched; the hub's memos stay
    out entirely.
    """
    graph = 0
    direct = (
        "the geyser erupted at dawn in the caldera",
        "the geyser steam drifted over the basin",
    )
    silent = "sulphur stains the terraces"
    for phrase in (*direct, silent):
        status, body = query(f"remember@{graph} '{phrase}' topic:thermal")
        assert status == 200, body.get("error")
    for i in range(8):
        status, body = query(
            f"remember@{graph} 'archive geyser note {i}' topic:archive"
        )
        assert status == 200, body.get("error")

    values, scores = _ranked_hits(query, f"recall@{graph} geyser caldera top:20")
    assert silent in values, f"the cluster's surplus must fund its member; got {values}"
    assert values[0] == direct[0], (
        f"the direct two-term match must outrank everything funded; got {values}"
    )
    assert values.index(silent) > values.index(direct[1]), (
        f"funded expansion must follow the facts that matched: {values}"
    )
