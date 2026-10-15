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


def test_the_fact_asked_about_outranks_its_neighbourhood(query, ranked_hits):
    """The fact that matches the query must top the expansion around it.

    Graph context enriches an answer. It must never bury the answer itself.

    Expansion needs surplus. The smallest shape where an anchor speaks has two
    anchors. A small cluster concentrates the query's mass, and a bigger hub
    holds no more than its fair share. The cluster's silent member is funded,
    attenuated by α², and lands behind both facts that matched. The hub
    transmits nothing, so its notes, which all contain "geyser", rank on their
    own text match alone. Both topics are named, because the graph is entered
    only through an anchor the recall names.
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

    values, scores = ranked_hits(
        f"recall@{graph} geyser caldera topic:thermal topic:archive top:20"
    )
    assert silent in values, f"the cluster's surplus must fund its member; got {values}"
    assert values[0] == direct[0], (
        f"the direct two-term match must outrank everything funded; got {values}"
    )
    assert values.index(silent) > values.index(direct[1]), (
        f"funded expansion must follow the facts that matched: {values}"
    )
