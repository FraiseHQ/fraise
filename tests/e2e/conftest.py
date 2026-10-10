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

"""Shared fixtures for the Fraise end-to-end suite.

The suite targets the server named by FRAISE_URL (`make test-e2e` points it
at the port docker compose publishes; unset, it is http://localhost:9876) and
waits for its health check before any test runs.

Graph allocation. Tests pin their writes to specific graphs so result counts
stay deterministic, including across reruns against a long-lived server (a
fact is keyed by its value, so rewrites are idempotent). Files run in any
order, so tests sharing a graph must not depend on each other's facts. Keep
this map current when claiming a graph:

    0  comet + quasar shared-keyword facts
       + monsoon/geyser ranking clusters   (recall_test.py, scoring_test.py)
    1  loose remembers + concurrent load   (recall_test.py, concurrency_test.py,
                                            parser_test.py, api_test.py)
    2  union-across-keywords facts
       + pulsar and storm explain probes   (recall_test.py, explain_test.py)
    3  parrot round trip
       + real-embedding documents          (recall_test.py, vectors_test.py)
    4  vector dimension + forest bound     (vectors_test.py, stats_test.py)
    5  bird facts + anchored recall
       + the topic-named fact
       + keyword-anchor + case-fold probes (concurrency_test.py, recall_test.py)
    6  vector round trip + cache probes
       + krakatoa fusion cluster
       + the emoji fact                    (vectors_test.py, query_cache_test.py,
                                            recall_test.py)
    7  planet star
       + lantern/almanac depth-lane probe
       + tidepool + saltmarsh anchor probes (recall_test.py)
       never given a vector: vectors_test.py recalls it with one
    8  never written: the empty-graph probes (api_test.py, and the
       SDK suite's empty_graph fixture)
    9  the SDK suite's exact stats counts (its stats_graph fixture);
       no e2e test writes here

Graph 8 is claimed by staying empty. A recall of a graph holding nothing is
answered 204 with no body, as distinct from the 200 and empty result set a
populated graph returns when nothing matched, and the only way to test that is
a graph no test ever writes to. Do not write to graph 8.
"""

import os
import time

import numpy as np
import pytest
import requests

BASE_URL = os.environ.get("FRAISE_URL", "http://localhost:9876").rstrip("/")

WAIT_TIMEOUT_SECONDS = 30
REQUEST_TIMEOUT_SECONDS = 15

# The dimension every plain-vector test writes with. The first vector inserted
# into a graph fixes that graph's index dimension, and graph 4 takes writes
# from two files (see the map above), so they must agree on it — which is why
# this lives here and not in a single test file.
VECTOR_DIM = 128


def pytest_configure(config):
    # Register the marker so `-m "not embeddings"` works and pytest doesn't warn
    # about an unknown mark. Tests carrying it need a real embedding model.
    config.addinivalue_line(
        "markers",
        "embeddings: requires a HuggingFace embedding model (sentence-transformers); skippable",
    )


@pytest.fixture(scope="session")
def base_url():
    """Base URL of a Fraise server that is confirmed to be up."""
    deadline = time.monotonic() + WAIT_TIMEOUT_SECONDS
    last_error = None
    while time.monotonic() < deadline:
        try:
            response = requests.get(f"{BASE_URL}/", timeout=2)
            if response.status_code == 200:
                return BASE_URL
            last_error = f"health check returned {response.status_code}"
        except requests.RequestException as exc:
            last_error = str(exc)
        time.sleep(0.5)
    pytest.fail(f"fraise server not reachable at {BASE_URL}: {last_error}")


# The value primed into every claimed graph. It shares no term with any fact
# or keyword the suite searches for, so it can never be a hit and never shifts
# a pinned count — it exists only to make the graph non-empty.
_PRIMER = "zzzprimerzzz marks this graph as written to"


@pytest.fixture(scope="session")
def num_graphs(get):
    """How many graphs the running server allocated.

    Read from /api/v1/stats, which reports one snapshot per graph, rather than
    hardcoded: the suite's config sets the count (see tests/fraise.config.toml)
    and a test naming an out-of-range selector must follow it, not a number
    that silently becomes valid the next time the count changes.

    Returns:
        The allocated graph count; valid selectors are 0..count-1.
    """
    return len(get("/api/v1/stats").json()["graphs"])


@pytest.fixture(scope="session", autouse=True)
def primed_graphs(query):
    """Write one unsearchable fact into every graph the suite claims but 8.

    A recall of a graph holding nothing is answered 204 with no body. Tests
    about parsing, warnings or limits issue recalls that match nothing without
    writing first, so their status would otherwise depend on whether another
    file had already written to that graph, and files run in any order.

    Priming makes "the graph is populated" true before any test runs, which is
    also the state a real caller queries in. Graph 8 is left out: it is the one
    graph the 204 itself is tested against.
    """
    for graph in range(8):
        status, body = query(f"remember@{graph} '{_PRIMER}'")
        assert status == 200, f"priming graph {graph}: {body.get('error')}"


@pytest.fixture(scope="session")
def request_timeout():
    """Per-request timeout for tests that make raw HTTP calls themselves."""
    return REQUEST_TIMEOUT_SECONDS


@pytest.fixture(scope="session")
def get(base_url):
    """Callable GETting a path on the server, returning the raw Response."""

    def _get(path: str):
        return requests.get(f"{base_url}{path}", timeout=REQUEST_TIMEOUT_SECONDS)

    return _get


def _body(response) -> dict:
    """Decode a response body, or {} when it carried none.

    A 204 — a recall of a graph holding nothing — is a success with no body at
    all, so the fixtures that return (status, body) would otherwise raise on
    the one status a test most wants to assert about.
    """
    if not response.content:
        return {}
    return response.json()


@pytest.fixture(scope="session")
def query(base_url):
    """Callable posting a raw query string to /api/v1/q.

    Returns (status_code, decoded JSON body).
    """

    def _query(text: str, parameters: dict[str, object] | None = None):
        data = {"query": text}

        if parameters:
            data["parameters"] = parameters

        response = requests.post(
            f"{base_url}/api/v1/q",
            json=data,
            timeout=REQUEST_TIMEOUT_SECONDS,
        )
        return response.status_code, _body(response)

    return _query


@pytest.fixture(scope="session")
def explain(base_url):
    """Callable posting a raw query string to /api/v1/explain.

    Same request shape as the `query` fixture, different endpoint: the
    response's hits carry a per-source contribution breakdown. Returns
    (status_code, decoded JSON body).
    """

    def _explain(text: str, parameters: dict[str, object] | None = None):
        data = {"query": text}

        if parameters:
            data["parameters"] = parameters

        response = requests.post(
            f"{base_url}/api/v1/explain",
            json=data,
            timeout=REQUEST_TIMEOUT_SECONDS,
        )
        return response.status_code, _body(response)

    return _explain


@pytest.fixture(scope="session")
def vector_dim():
    """The suite-wide plain-vector dimension (see VECTOR_DIM)."""
    return VECTOR_DIM


@pytest.fixture(scope="session")
def vector():
    """Callable building a flat list of `dim` floats, the shape the API expects
    for a vector parameter.
    """

    def _vector(dim: int = VECTOR_DIM, value: float = 1.0) -> list[float]:
        return np.full(dim, value, dtype=float).tolist()

    return _vector


@pytest.fixture(scope="session")
def recall_count(query):
    """Callable running a recall that must succeed and returning its hit count.

    Returns:
        ``callable(text) -> int``.
    """

    def _recall_count(text: str) -> int:
        status, body = query(text)
        assert status == 200, body.get("error")
        return body["results"]["count"]

    return _recall_count


@pytest.fixture(scope="session")
def recall_ranking(query):
    """Callable running a recall that must succeed and returning its hit values,
    best-ranked first.

    Values, not scores: a score decays with the fact's age at the instant the
    search runs, so two identical recalls a millisecond apart legitimately score
    the same fact differently. It is the ranking those scores produce that has to
    be reproducible.

    Returns:
        ``callable(text) -> list[str]``.
    """

    def _recall_ranking(text: str) -> list[str]:
        status, body = query(text)
        assert status == 200, body.get("error")
        return [hit["value"] for hit in body["results"]["hits"]]

    return _recall_ranking


@pytest.fixture(scope="session")
def ranked_hits(query):
    """Callable running a recall and checking the invariants every ranking keeps:
    scores positive and non-increasing down the list.

    Returns:
        ``callable(text, parameters=None) -> (values, scores)``, both best first.
    """

    def _ranked_hits(text: str, parameters: dict[str, object] | None = None):
        status, body = query(text, parameters=parameters)
        assert status == 200, body.get("error")
        hits = body["results"]["hits"]
        scores = [hit["score"] for hit in hits]
        assert all(s > 0 for s in scores), f"scores must be positive: {scores}"
        assert scores == sorted(scores, reverse=True), (
            f"scores must not increase down the ranking: {scores}"
        )
        return [hit["value"] for hit in hits], scores

    return _ranked_hits


@pytest.fixture(scope="session")
def assert_rejected():
    """Callable asserting a 400 whose message contains ``expected``,
    case-insensitively.

    Returns:
        ``callable(status, body, expected, query_text)``.
    """

    def _assert_rejected(status, body, expected, query_text):
        assert status == 400, (
            f"{query_text!r}: expected 400, got {status} — body {body!r}"
        )
        message = (body.get("error") or "").lower()
        assert message, f"{query_text!r}: 400 with an empty error message"
        assert expected.lower() in message, (
            f"{query_text!r}: error {body.get('error')!r} should mention {expected!r}"
        )

    return _assert_rejected


@pytest.fixture(scope="session")
def assert_accepted():
    """Callable asserting a query the parser must treat as valid.

    Returns:
        ``callable(status, body, query_text)``.
    """

    def _assert_accepted(status, body, query_text):
        assert status == 200, (
            f"{query_text!r}: expected 200, got {status} — {body.get('error')!r}"
        )

    return _assert_accepted


# Four facts sharing a single topic, each with a unique keyword. This is a
# star: every fact hangs off the same "planets" hub. A recall seeded from the
# star touches no anchor but that hub, which then holds exactly its fair share
# and transmits nothing (hub silence), so the exact result counts depend on
# the keywords and top, never on depth.
_PLANET_GRAPH = 7
_PLANET_TOPIC = "planets"
_PLANET_FACTS = {
    "mercury": "mercury is the smallest planet",
    "venus": "venus is the hottest planet",
    "mars": "mars is the red planet",
    "jupiter": "jupiter is the largest planet",
}

# The depth-lane probe. Transmission needs two touched anchors of different
# concentration: a lone anchor's observed mass is the background, so it holds
# no surplus and stays silent (which is what the planet star above shows). Here
# a tight "lanterns" cluster holds two of the query's three seeds while a
# larger "almanac" hub holds the third among its eight members, so the cluster
# clears the background and the hub does not. The cluster's third fact carries
# no query term at all: it can only be returned if an anchor transmitted to
# it, which makes it the probe that tells the floor lane from the graph lanes.
#
# Shares graph 7 with the planet star: no fact here contains "mercury" or
# "planet" and no planet fact contains "lantern", so neither set can appear in
# the other's recalls.
_LANTERN_TOPIC = "lanterns"
_LANTERN_SILENT = "the quay is silent at dawn"
_LANTERN_CLUSTER = (
    "the lantern glows on the quay",
    "lantern light guides the ferry",
    _LANTERN_SILENT,
)
_ALMANAC_TOPIC = "almanac"
_ALMANAC_HUB = ("a lantern in the old almanac",) + tuple(
    f"unrelated almanac entry {i}" for i in range(7)
)

# The anchor-seeding probe. A fact filed under a topic and an entity at once,
# between a fact filed under the topic alone and one under the entity alone,
# so a recall naming both anchors and nothing else has a union to assemble, a
# duplicate to suppress and a fact to rank first — the one filed under both.
# Written in this order, so newest first is the reverse of it. No fact here
# contains "planet", "mercury" or "lantern", and nothing else on graph 7 is
# filed under these anchors, so the results are exact.
_TIDEPOOL_TOPIC = "tidepool"
_TIDEPOOL_ENTITY = "limpet"
_TIDEPOOL_FACTS = {
    "topic": "the tide leaves the rockpool warm",
    "both": "the limpet clamps down as the water drops",
    "entity": "a limpet returns to its home scar",
}
_TIDEPOOL_ANCHORS = {
    "topic": f"topic:{_TIDEPOOL_TOPIC}",
    "both": f"topic:{_TIDEPOOL_TOPIC} entity:{_TIDEPOOL_ENTITY}",
    "entity": f"entity:{_TIDEPOOL_ENTITY}",
}

# The default-top probe: one anchor holding more facts than the daemon's
# configured default-top (10 in tests/fraise.config.toml), so an anchor-only
# recall with no top: clause is visibly capped. Shares graph 7 with the probes
# above and contains none of their words.
DEFAULT_TOP = 10
_SALTMARSH_TOPIC = "saltmarsh"
_SALTMARSH_FACTS = tuple(f"saltmarsh channel {i} was surveyed" for i in range(12))


@pytest.fixture(scope="session")
def planet_facts():
    """The planet star's facts, keyed by the unique keyword each contains."""
    return dict(_PLANET_FACTS)


@pytest.fixture(scope="module")
def planets_graph(query):
    """Populate a graph with the planet star and return its id.

    A fact is keyed by its value, so these writes are idempotent: re-running
    the suite against a long-lived server leaves the counts unchanged.
    """
    for phrase in _PLANET_FACTS.values():
        status, body = query(
            f"remember@{_PLANET_GRAPH} '{phrase}' topic:{_PLANET_TOPIC}"
        )
        assert status == 200, body.get("error")
    return _PLANET_GRAPH


@pytest.fixture(scope="session")
def lantern_silent():
    """The cluster fact containing no query term.

    It can only be recalled if an anchor transmitted mass to it, so it is what
    separates the floor lane from the excess lane in a result set.
    """
    return _LANTERN_SILENT


@pytest.fixture(scope="module")
def lantern_graph(query):
    """Seed the depth-lane cluster and its diluting hub, returning the graph id.

    Idempotent for the same reason as the planet star: facts are keyed by
    value.
    """
    for phrase in _LANTERN_CLUSTER:
        status, body = query(
            f"remember@{_PLANET_GRAPH} '{phrase}' topic:{_LANTERN_TOPIC}"
        )
        assert status == 200, body.get("error")
    for phrase in _ALMANAC_HUB:
        status, body = query(
            f"remember@{_PLANET_GRAPH} '{phrase}' topic:{_ALMANAC_TOPIC}"
        )
        assert status == 200, body.get("error")
    return _PLANET_GRAPH


@pytest.fixture(scope="session")
def tidepool_anchors():
    """The anchor-seeding probe's (topic, entity) pair."""
    return _TIDEPOOL_TOPIC, _TIDEPOOL_ENTITY


@pytest.fixture(scope="session")
def tidepool_facts():
    """The anchor-seeding probe's facts, keyed by what each is filed under
    ("topic", "both", "entity"), in write order."""
    return dict(_TIDEPOOL_FACTS)


@pytest.fixture(scope="module")
def tidepool_graph(query):
    """Seed the anchor-seeding probe and return its graph id.

    Idempotent for the same reason as the planet star: facts are keyed by
    value, and a rewrite refreshes the timestamp in the same order.
    """
    for filed, phrase in _TIDEPOOL_FACTS.items():
        status, body = query(
            f"remember@{_PLANET_GRAPH} '{phrase}' {_TIDEPOOL_ANCHORS[filed]}"
        )
        assert status == 200, body.get("error")
    return _PLANET_GRAPH


@pytest.fixture(scope="session")
def default_top():
    """The daemon's configured default-top (tests/fraise.config.toml)."""
    return DEFAULT_TOP


@pytest.fixture(scope="session")
def saltmarsh_facts():
    """The default-top probe's facts, in write order."""
    return tuple(_SALTMARSH_FACTS)


@pytest.fixture(scope="module")
def saltmarsh_graph(query):
    """Seed the default-top probe and return its graph id. Idempotent: facts
    are keyed by value."""
    for phrase in _SALTMARSH_FACTS:
        status, body = query(
            f"remember@{_PLANET_GRAPH} '{phrase}' topic:{_SALTMARSH_TOPIC}"
        )
        assert status == 200, body.get("error")
    return _PLANET_GRAPH


# Three facts that all contain the keyword "comet" but are otherwise unrelated:
# each carries a *different* topic, so nothing connects them in the graph except
# the shared word. A recall for that word must therefore surface all three
# purely through the text index. No other fact on graph 0 contains "comet", so
# the expected set is fully determined here.
_COMET_FACTS = {
    "the comet streaked past mars": "astronomy",
    "children watched the comet at dawn": "memory",
    "the comet will not return for centuries": "time",
}

# Five facts that all contain "quasar" once and carry no topic:/entity: anchor,
# so each is an isolated node that no walk from another graph-0 fact can
# reach: a recall for "quasar" is answered by the text index alone. They share
# graph 0 with the comet facts above, which contain no "quasar".
_QUASAR_FACTS = (
    "the quasar catalogue was revised",
    "a quasar outshines its host galaxy",
    "radio astronomers logged the quasar",
    "the quasar sits behind a lensing cluster",
    "the quasar faded from the survey",
)

# A fact whose anchors are grammar keywords. "top" is also an ordinary English
# word, and an LLM extracting entities from prose will eventually emit it bare
# ("she reached the top" -> entities=["top"]). A parser that typed an anchor
# value by spelling alone would fail that write with a 400 the client could
# not anticipate. The invented marker "cairnprobe" is this fact's only link to
# the recalls in recall_test.py, so each assertion is scoped to this fact
# whatever else graph 5 holds.
_CAIRN_FACT = "the cairnprobe marks the top of the pass"

# Case folding. Terms and anchor values are identity, not prose: the parser
# folds them to lower case on the way in, so however a client capitalises an
# anchor, a single node accrues in the graph. The quoted fact is prose and is
# the one exception — it comes back spelled exactly as written.
_CASEPROBE_FACT = "The Caseprobe Expedition Reached the Summit in April."

# A fact whose whole text is also the name of its topic, plus a bystander fact
# carrying the same topic. Keys derived from the value alone would give the
# fact and the topic one key, so the topic node would never be stored and the
# bystander's IsAbout edge would land on the fact instead of on a topic hub.
# The bystander is what makes that visible: the two facts have no word in
# common and belong together only through the topic.
_LEDGER_TOPIC = "ledgerprobe"
_LEDGER_FACTS = ("ledgerprobe", "acme settles invoices quarterly")

# A batch of distinct facts, each carrying a unique keyword so a recall can
# target exactly one of them. They share a topic so the write path also
# exercises relationship creation.
_BIRD_FACTS = {
    "parrot": "the parrot is turquoise",
    "raven": "the raven is midnight black",
    "canary": "the canary is bright yellow",
    "flamingo": "the flamingo is pink",
    "peacock": "the peacock is iridescent",
    "robin": "the robin has a red breast",
    "magpie": "the magpie loves shiny things",
    "owl": "the owl hunts at night",
}


@pytest.fixture(scope="session")
def comet_facts():
    """The comet facts, each mapped to the topic it is filed under."""
    return dict(_COMET_FACTS)


@pytest.fixture(scope="session")
def quasar_facts():
    """The quasar facts, which carry no anchor."""
    return tuple(_QUASAR_FACTS)


@pytest.fixture(scope="session")
def cairn_fact():
    """The fact filed under topic:top and entity:top."""
    return _CAIRN_FACT


@pytest.fixture(scope="session")
def caseprobe_fact():
    """The mixed-case fact the case-folding probe writes and expects back."""
    return _CASEPROBE_FACT


@pytest.fixture(scope="session")
def ledger_topic():
    """The topic the ledger probe's first fact is named like."""
    return _LEDGER_TOPIC


@pytest.fixture(scope="session")
def ledger_facts():
    """The ledger probe's facts: the one named like its topic, then the bystander."""
    return tuple(_LEDGER_FACTS)


@pytest.fixture(scope="session")
def bird_facts():
    """The bird facts, keyed by the unique keyword each contains."""
    return dict(_BIRD_FACTS)


# The explain probes, both on graph 2.
#
# Two facts sharing a keyword and an entity. Both are text seeds for "pulsar";
# their shared entity is the query's only touched anchor, so it sits exactly
# at the background rate and transmits nothing — every hit's breakdown is a
# single text contribution. (Anchors are heard only when their members matched
# better than their size predicts; the storm probe below is the funded case.)
_EXPLAIN_GRAPH = 2
_PULSAR_FACTS = (
    "the pulsar spins thirty times a second",
    "the pulsar emits radio beams",
)
_PULSAR_ENTITY = "vela"

# The surplus probe: a small "weather" cluster concentrates the query's mass
# while a larger "archive" hub holds no more than its fair share of it, so
# exactly one anchor speaks and its silent member is funded by transmission
# alone. The explain recalls name both topics so the filter keeps the hub's
# memos admissible: their absence is then its silence and not the filter's
# doing.
_STORM_CLUSTER_TOPIC = "weather"
_STORM_HUB_TOPIC = "archive"
# No query term: funded or invisible.
_STORM_SILENT_MEMBER = "the harbour is calm tonight"
_STORM_CLUSTER = (
    "the barometer falls before the storm",
    "storm clouds gather at sea",
    _STORM_SILENT_MEMBER,
)
_STORM_HUB = ("a storm of paperwork",) + tuple(
    f"unrelated archive memo {i}" for i in range(7)
)

_ALPHA = 0.5  # the server's per-edge attenuation; α² on the two-edge path


@pytest.fixture
def pulsar_graph(query):
    """Write the pulsar facts under the pulsar entity and return their graph id.

    Rewritten for every test that asks, which refreshes their timestamps just
    before the test reads them.
    """
    for phrase in _PULSAR_FACTS:
        status, body = query(
            f"remember@{_EXPLAIN_GRAPH} '{phrase}' entity:{_PULSAR_ENTITY}"
        )
        assert status == 200, body.get("error")
    return _EXPLAIN_GRAPH


@pytest.fixture(scope="session")
def pulsar_entity():
    """The entity both pulsar facts are filed under: the query's only anchor."""
    return _PULSAR_ENTITY


@pytest.fixture
def storm_graph(query):
    """Write the storm cluster and the archive hub under their topics, and
    return their graph id.

    Rewritten for every test that asks, which refreshes their timestamps just
    before the test reads them.
    """
    for phrase in _STORM_CLUSTER:
        status, body = query(
            f"remember@{_EXPLAIN_GRAPH} '{phrase}' topic:{_STORM_CLUSTER_TOPIC}"
        )
        assert status == 200, body.get("error")
    for phrase in _STORM_HUB:
        status, body = query(
            f"remember@{_EXPLAIN_GRAPH} '{phrase}' topic:{_STORM_HUB_TOPIC}"
        )
        assert status == 200, body.get("error")
    return _EXPLAIN_GRAPH


@pytest.fixture(scope="session")
def storm_cluster_topic():
    """The storm cluster's topic: the anchor that transmits its surplus."""
    return _STORM_CLUSTER_TOPIC


@pytest.fixture(scope="session")
def storm_hub_topic():
    """The archive hub's topic: an anchor at its fair share, which stays silent."""
    return _STORM_HUB_TOPIC


@pytest.fixture(scope="session")
def storm_silent_member():
    """The cluster fact with no query term, which only transmission can surface."""
    return _STORM_SILENT_MEMBER


@pytest.fixture(scope="session")
def storm_hub():
    """The archive hub's facts; only the first contains a query term."""
    return tuple(_STORM_HUB)


@pytest.fixture(scope="session")
def alpha():
    """The server's per-edge attenuation α, applied as α² on the two-edge path."""
    return _ALPHA


# Documents on three clearly distinct subjects. A real sentence embedding places
# each far from the others, so a query close in meaning to one of them retrieves
# that one by vector alone. Graph 3 carries no other vectors, so its embedding
# dimension is fixed by the real-embeddings test.
_EMBEDDING_DOCS = {
    "cat": "the tabby cat curled up and slept on the warm windowsill",
    "space": "the mars rover drilled into red rock to collect samples",
    "bread": "he kneaded the dough and baked a fresh loaf of sourdough",
}

# The text-and-vector fusion probe on graph 6, keyed by the channels that see
# each fact.
_KRAKATOA_FACTS = {
    "text": "krakatoa ash fell for days",  # both terms; no embedding
    "both": "the ash cloud crossed the ocean",  # one term; near embedding
    "vector": "sensors recorded the pressure wave",  # no terms; exact embedding
}


@pytest.fixture(scope="session")
def embedding_docs():
    """The real-embedding documents, keyed by the topic each is filed under."""
    return dict(_EMBEDDING_DOCS)


@pytest.fixture(scope="session")
def krakatoa_facts():
    """The fusion probe's facts, keyed by the channels that see each:
    "text", "both" or "vector"."""
    return dict(_KRAKATOA_FACTS)
