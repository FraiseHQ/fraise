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

"""Opinionated ranking properties: the promises a recall's scores must keep.

They hold whatever scorer is in charge. Each test states an opinion about good
memory retrieval, and pins the fused order, not the plumbing:

- Better matches outrank worse ones.
- The fact you asked about outranks the facts merely connected to it.
- An anchor speaks only when its members matched better than its size
  predicts.

If a future scorer breaks one of these, that is a product decision to make
consciously. It is not a regression to discover in an agent's answers.

One promise is not pinned here: at equal relevance, recent memories outrank
older ones. remember stamps time.Now(), so an HTTP test cannot create the
weeks-old fact needed to observe decay. The unit suite's decay tests pin that
property.
"""


def test_matching_more_of_the_query_outranks_matching_less(query, ranked_hits):
    """A fact matching both query terms must strictly outrank a one-term match.

    Relevance is the first-order signal, and query coverage is how the text
    index measures it.
    """
    graph = 0
    both = "the monsoon flooded the delta"
    one = "the monsoon arrived early"
    for phrase in (both, one):
        status, body = query(f"remember@{graph} '{phrase}'")
        assert status == 200, body.get("error")

    values, scores = ranked_hits(f"recall@{graph} monsoon delta")
    assert values == [both, one], f"two matching terms must beat one; got {values}"
    assert scores[0] > scores[1], (
        f"the better match must win strictly, not by tiebreak: {scores}"
    )


def test_the_fact_asked_about_outranks_its_neighbourhood(
    query, recall_ranking, unfiled_topic
):
    """The fact that actually matches the query must top the expansion pulled
    in around it. Graph context enriches an answer; it must never bury the
    answer itself.

    Expansion requires surplus: a small cluster concentrating the query's
    mass next to a bigger hub holding no more than its fair share is the
    smallest shape where an anchor speaks. The cluster's silent member is
    funded by the cluster's surplus and lands behind both facts that matched;
    the hub transmits nothing, so its notes, which all contain "geyser", rank
    on their own text match alone. The graph is entered only once the recall
    names an anchor, and a named anchor raises its facts instead of being
    judged by the round, so the recall names a topic nothing is filed under
    and leaves both topics to the round. The two topics are two facets, so
    the spread (db.aggregate) seats the hub's best note before the cluster's
    second fact; the order within the cluster is the ranking's.
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

    values = recall_ranking(
        f"recall@{graph} geyser caldera topic:{unfiled_topic} top:20"
    )
    assert silent in values, f"the cluster's surplus must fund its member; got {values}"
    assert values[0] == direct[0], (
        f"the direct two-term match must outrank everything funded; got {values}"
    )
    assert values.index(silent) > values.index(direct[1]), (
        f"funded expansion must follow the facts that matched: {values}"
    )
