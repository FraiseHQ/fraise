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

"""Client tests against a patched requests.Session, and against a live server.

The tests marked ``integration`` need the live server; the rest need none.
Setup lives in conftest.py; this file is assertions.
"""

import warnings
from unittest.mock import MagicMock

import pytest
import requests
from fraise_sdk import FraiseAPIError, FraiseClient, FraiseError, FraiseWarning
from fraise_sdk.constants import DEFAULT_TIMEOUT_SECONDS
from fraise_sdk.errors import FraiseQueryError
from fraise_sdk.providers import Anchor


def test_closing_closes_the_session_the_client_owns(session):
    """Leaving the context manager closes the session the client created."""
    with FraiseClient():
        pass
    session.close.assert_called_once_with()


def test_an_injected_session_is_left_open(respond, no_hits):
    """A session passed in is left open: the caller owns it, so the caller closes it."""
    injected = MagicMock()
    respond(injected, no_hits)
    with FraiseClient(session=injected):
        pass
    injected.close.assert_not_called()


def test_remember_posts_expected_query(session, query_url):
    """remember posts one query: the graph, the quoted fact and its topic."""
    FraiseClient().remember("the parrot is turquoise", graph=3, topics=["color"])
    session.post.assert_called_once_with(
        query_url,
        json={"query": "remember@3 'the parrot is turquoise' topic:color"},
        timeout=DEFAULT_TIMEOUT_SECONDS,
    )


def test_remember_with_vector_sends_parameters(session, sent):
    """A vector travels out of band: the query names ``vec:$v``, the parameters carry it."""
    FraiseClient().remember("kingfisher is blue", graph=6, vector=[0.5, 0.5])
    assert sent(session) == {
        "query": "remember@6 'kingfisher is blue' vec:$v",
        "parameters": {"v": [0.5, 0.5]},
    }


def test_a_bare_string_topic_is_rejected_before_any_request(session):
    """``topics="color"`` never reaches the wire.

    The builder refuses a bare string where a sequence is wanted, and the
    client raises that error before posting. The server would accept the
    letter-by-letter expansion and file the fact under one anchor per
    character, unretrievable by the anchor the caller named, so failing in the
    caller's own stack trace is the only useful answer.
    """
    with pytest.raises(FraiseQueryError, match="did you mean"):
        FraiseClient().remember("the parrot is turquoise", topics="color")
    session.post.assert_not_called()


def test_recall_parses_hits(session, respond, sent):
    """recall sends its keywords and clauses and parses the hits in the server's order."""
    respond(
        session,
        {
            "results": {
                "count": 2,
                "hits": [
                    {
                        "value": "mars is the red planet",
                        "score": 1.0,
                        "timestamp": "2026-01-01T00:00:00Z",
                    },
                    {
                        "value": "venus is hot",
                        "score": 0.42,
                        "timestamp": "2026-01-01T00:00:00Z",
                    },
                ],
            }
        },
    )
    result = FraiseClient().recall("mars", "venus", graph=7, top=10)

    assert sent(session)["query"] == "recall@7 mars venus top:10"
    assert result.count == 2
    assert [h.value for h in result] == ["mars is the red planet", "venus is hot"]
    assert result.hits[0].score == 1.0
    assert bool(result) is True


@pytest.mark.parametrize(
    "keywords, kwargs",
    [
        (["polly"], {"entities": ["polly"], "depth": 2}),
        ([], {"query": "where does the parrot sleep", "topics": ["birds"], "top": 3}),
        (["kettle"], {"graph": 4, "vector": [0.5, 0.5]}),
    ],
)
def test_explain_sends_the_recall_query_to_the_explain_route(
    session, query_url, explain_url, keywords, kwargs
):
    """explain posts exactly what recall posts, to the explain route instead.

    The two share one builder so an explanation is always of the ranking recall
    would return; if they ever sent different query strings or vectors, the
    breakdown would explain a different question.
    """
    client = FraiseClient()
    client.recall(*keywords, **kwargs)
    client.explain(*keywords, **kwargs)

    recalled, explained = session.post.call_args_list
    assert recalled.args == (query_url,)
    assert explained.args == (explain_url,)
    assert explained.kwargs == recalled.kwargs


def test_explain_returns_each_hit_with_its_contributions(
    session, respond, explain_response
):
    """An explained recall comes back typed: contributions on every hit, and
    the background rate on the result."""
    respond(session, explain_response)

    result = FraiseClient().explain("polly", entities=["polly"], depth=2)

    assert result.count == 3
    assert all(hit.contributions for hit in result)
    assert result.background == 0.4857013396824596


def test_recall_empty_results(session):
    """A populated graph that matched nothing is an empty, falsey result.

    ``empty`` is False here: it reports on the graph, not on the result set,
    and this graph holds facts the query missed, so rephrasing is the thing to
    try. Compare the 204 test below, where rephrasing would never have helped.
    """
    result = FraiseClient().recall("nothingindexed")
    assert result.count == 0
    assert list(result) == []
    assert bool(result) is False
    assert result.empty is False


def test_recall_of_an_empty_graph_is_marked_empty(session, respond_no_content):
    """A 204 means the graph searched holds nothing, and carries no body.

    The distinction rides the status line, so a client that parsed only the
    body would report this identically to the miss above. The result is still
    a RecallResult, so a caller reading before its first write needs no None
    check.
    """
    respond_no_content(session)

    result = FraiseClient().recall("nothingindexed")

    assert result.empty is True
    assert result.count == 0
    assert list(result) == []
    assert bool(result) is False


def test_a_bodiless_204_reaches_query_as_an_empty_body(session, respond_no_content):
    """The raw escape hatch answers a 204 with ``{}`` rather than raising.

    ``query`` returns the decoded body and a 204 has none, so ``{}`` is the
    truthful answer. Telling an empty graph from a miss takes ``recall``,
    which reads the status this method does not return.
    """
    respond_no_content(session)

    assert FraiseClient().query("recall nothingindexed") == {}


def test_remember_accepts_the_write_acknowledgement(session, respond):
    """A write is answered with its own shape, not a recall's envelope.

    The server acknowledges with ``{"status": "ok"}``, so a stored fact never
    reads like a recall that matched nothing. The client must accept that
    body without reaching for ``results``, which it does not carry.
    """
    respond(session, {"status": "ok"})

    assert FraiseClient().remember("the parrot is turquoise") is None


def test_recall_surfaces_server_warnings(session, respond, server_warning):
    """A server warning reaches the caller on both channels: listed on the
    result for programmatic use, and emitted as a FraiseWarning so it is
    visible by default without any code changes.

    The armed response mimics ``recall ferry the``: the query ran — hits and
    all — while the server flagged a stop word that can never match. Warnings
    ride beside the results, they do not replace them.
    """
    respond(
        session,
        {
            "results": {
                "count": 1,
                "hits": [{"value": "the ferry docks at dawn", "score": 1.0}],
            },
            "warnings": [server_warning],
        },
    )

    with pytest.warns(FraiseWarning, match="is a stop word") as record:
        result = FraiseClient().recall("ferry", "the")

    assert record[0].filename == __file__
    assert result.warnings == [server_warning]
    assert result.count == 1


def test_a_reserved_word_keyword_warns_at_the_callers_line(session, sent):
    """``recall("since", "7d")`` is sent quoted and warns before it is sent.

    The warning is decided in the query builder, several calls below this
    line, and still names this file: the caller's own call is the line they
    can change, and a warning pointing into the SDK would leave them looking
    for it.
    """
    with pytest.warns(FraiseWarning, match="is a reserved word") as record:
        FraiseClient().recall("since", "7d")

    assert record[0].filename == __file__
    assert sent(session)["query"] == "recall@0 'since' 7d"


def test_recall_without_warnings_is_silent(session):
    """A response with no warnings field yields an empty list and emits
    nothing — the common, unambiguous path must stay quiet, so a caller who
    escalates warnings to errors is not tripped by clean queries.
    """
    with warnings.catch_warnings():
        warnings.simplefilter("error")
        result = FraiseClient().recall("zebras")

    assert result.warnings == []


def test_raw_query_emits_server_warnings(session, respond, server_warning):
    """The raw query() escape hatch emits FraiseWarning too: warnings are
    emitted on the path every request takes, so query() and the typed helpers
    share one channel without plumbing of their own.
    """
    respond(
        session, {"results": {"count": 0, "hits": []}, "warnings": [server_warning]}
    )

    with pytest.warns(FraiseWarning, match="is a stop word"):
        body = FraiseClient().query("recall@0 ferry the")

    assert body["warnings"] == [server_warning]


def test_api_error_surfaces_server_message(session, respond):
    """A non-2xx answer raises FraiseAPIError with the status and the server's message."""
    respond(session, {"error": "could not parse query"}, status_code=400)
    with pytest.raises(FraiseAPIError) as excinfo:
        FraiseClient().recall("bogus")
    assert excinfo.value.status_code == 400
    assert "could not parse query" in excinfo.value.message


def test_an_error_with_a_non_json_body_still_raises(session, respond):
    """An error status whose body is not JSON still raises FraiseAPIError,
    surfacing the raw text — a proxy's HTML error page reaches the caller
    instead of being swallowed into silence.
    """
    response = respond(session, {}, status_code=502)
    response.json.side_effect = ValueError("not json")
    response.text = "<html>bad gateway</html>"
    with pytest.raises(FraiseAPIError) as excinfo:
        FraiseClient().recall("anything")
    assert excinfo.value.status_code == 502
    assert "bad gateway" in excinfo.value.message


def test_unreachable_server_raises_fraise_error(session):
    """A connection failure raises FraiseError saying the server could not be reached."""
    session.post.side_effect = requests.ConnectionError("refused")
    with pytest.raises(FraiseError, match="could not reach fraise"):
        FraiseClient().recall("anything")


def test_timed_out_server_raises_a_distinct_fraise_error(session):
    """A timeout gets its own message, naming the timeout, so it reads
    differently from a plain connection failure and points at the fix
    (raise ``timeout=``) instead of "could not reach fraise".
    """
    session.post.side_effect = requests.Timeout("timed out")
    with pytest.raises(FraiseError, match="timed out") as excinfo:
        FraiseClient(timeout=5.0).recall("anything")
    assert "could not reach fraise" not in str(excinfo.value)
    assert "5.0s" in str(excinfo.value)


def test_health_is_true_on_200(session, respond_get):
    """A 200 from the health endpoint answers True."""
    respond_get(session, {"status": "ok"})
    assert FraiseClient().health() is True


def test_health_is_false_on_a_non_200(session, respond_get):
    """A non-200 answers False: degraded is not healthy."""
    respond_get(session, {}, status_code=503)
    assert FraiseClient().health() is False


def test_health_is_false_when_unreachable(session):
    """A transport error is swallowed into False, never raised.

    health() is the probe callers use to decide whether to bother, so it has
    to answer even when nothing is listening.
    """
    session.get.side_effect = requests.ConnectionError("refused")
    assert FraiseClient().health() is False


def test_server_version_reads_the_health_field(session, respond_get):
    """The version travels in the health endpoint's ``version`` field."""
    respond_get(session, {"status": "ok", "version": "0.1.0"})
    assert FraiseClient().server_version() == "0.1.0"


def test_server_version_is_none_when_unreachable(session):
    """An unreachable server reports no version rather than raising."""
    session.get.side_effect = requests.ConnectionError("refused")
    assert FraiseClient().server_version() is None


def test_server_version_is_none_on_a_non_200(session, respond_get):
    """An error status yields None even when the body carries a version:
    an error page's claims are not the server's."""
    respond_get(session, {"version": "0.1.0"}, status_code=500)
    assert FraiseClient().server_version() is None


def test_server_version_is_none_on_a_non_json_body(session, respond_get):
    """A body that fails to decode yields None — a proxy's HTML error page
    is not a version."""
    response = respond_get(session, {})
    response.json.side_effect = ValueError("not json")
    assert FraiseClient().server_version() is None


@pytest.mark.parametrize("body", [{}, {"version": ""}, {"version": 3}, []])
def test_server_version_is_none_without_a_usable_version_field(
    session, respond_get, body
):
    """A decoded body without a non-empty string version yields None —
    servers predating version reporting and shape surprises alike."""
    respond_get(session, body)
    assert FraiseClient().server_version() is None


def test_check_compatibility_warns_when_the_version_is_unknown(session):
    """No readable version warns and answers False instead of guessing."""
    session.get.side_effect = requests.ConnectionError("refused")
    with pytest.warns(UserWarning, match="could not determine"):
        assert FraiseClient().check_compatibility() is False


def test_check_compatibility_strict_raises_when_the_version_is_unknown(session):
    """strict=True turns the unknown-version warning into a FraiseError."""
    session.get.side_effect = requests.ConnectionError("refused")
    with pytest.raises(FraiseError, match="could not determine"):
        FraiseClient().check_compatibility(strict=True)


@pytest.mark.parametrize("version", ["0.2.2", "99.0.0", "0.0.9", "0.1", "abc", "x.y.z"])
def test_check_compatibility_warns_outside_the_supported_range(
    session, respond_get, version
):
    """A version outside SUPPORTED_SERVER, whether above it, below it or too
    mangled to parse, warns and answers False rather than raising."""
    respond_get(session, {"version": version})
    with pytest.warns(UserWarning, match="outside this SDK's supported range"):
        assert FraiseClient().check_compatibility() is False


def test_check_compatibility_strict_raises_outside_the_supported_range(
    session, respond_get
):
    """strict=True turns the out-of-range warning into a FraiseError."""
    respond_get(session, {"version": "99.0.0"})
    with pytest.raises(FraiseError, match="outside this SDK's supported range"):
        FraiseClient().check_compatibility(strict=True)


@pytest.mark.parametrize("version", ["0.3.1", "v0.3.15", "v0.3.0"])
def test_check_compatibility_accepts_a_supported_version(session, respond_get, version):
    """An in-range version answers True with no warning, and a ``v`` prefix
    is spelling, not incompatibility. The literals sit inside SUPPORTED_SERVER
    and must move with it, like the live-server test below."""
    respond_get(session, {"version": version})
    with warnings.catch_warnings():
        warnings.simplefilter("error")
        assert FraiseClient().check_compatibility() is True


def test_configured_embedder_encodes_remember_value(session, sent, callable_embedder):
    """With an embedder, remember encodes the fact itself and sends its vector."""
    embedder = callable_embedder()
    FraiseClient(embedder=embedder).remember("the parrot is turquoise", graph=6)
    assert sent(session) == {
        "query": "remember@6 'the parrot is turquoise' vec:$v",
        "parameters": {"v": [23.0] * 4},  # len("the parrot is turquoise")
    }
    embedder.assert_called_once_with("the parrot is turquoise")


def test_configured_embedder_encodes_recall_keywords(session, sent, callable_embedder):
    """Without a query phrase, recall encodes its keywords joined by spaces."""
    embedder = callable_embedder()
    FraiseClient(embedder=embedder).recall("kingfisher", "blue", graph=6)
    assert sent(session)["query"] == "recall@6 kingfisher blue vec:$v"
    embedder.assert_called_once_with("kingfisher blue")


def test_recall_query_phrase_overrides_keywords_for_embedding(
    session, sent, callable_embedder
):
    """With a query phrase, the phrase is what gets encoded.

    The question itself travels as one quoted phrase term ahead of the
    keywords, never as bare words the grammar could read as clauses.
    """
    embedder = callable_embedder()
    FraiseClient(embedder=embedder).recall(
        "zzznomatch", graph=6, query="a sleepy kitten in the sun"
    )
    embedder.assert_called_once_with("a sleepy kitten in the sun")
    assert (
        sent(session)["query"]
        == "recall@6 'a sleepy kitten in the sun' zzznomatch vec:$v"
    )


def test_explicit_vector_wins_over_embedder(session, sent, callable_embedder):
    """An explicit vector is sent as given, and the embedder is not called."""
    embedder = callable_embedder()
    FraiseClient(embedder=embedder).remember("x is y", graph=6, vector=[0.1, 0.2])
    assert sent(session)["parameters"] == {"v": [0.1, 0.2]}
    embedder.assert_not_called()


def test_embed_false_skips_a_configured_embedder(session, sent, callable_embedder):
    """``embed=False`` stores the fact without a vector, embedder or not."""
    embedder = callable_embedder()
    FraiseClient(embedder=embedder).remember("x is y", graph=6, embed=False)
    assert "parameters" not in sent(session)
    embedder.assert_not_called()


def test_embed_true_without_embedder_raises(session):
    """``embed=True`` on a client with no embedder raises rather than sending no vector."""
    with pytest.raises(FraiseError, match="no embedder"):
        FraiseClient().remember("x is y", embed=True)


def test_no_embedder_sends_no_vector(session, sent):
    """A client without an embedder sends no vector parameters."""
    FraiseClient().remember("x is y", graph=6)
    assert "parameters" not in sent(session)


def test_embedder_object_is_called_through_its_embed_method(session, sent):
    """An Embedder is called through ``embed``, not ``__call__``, which only delegates."""
    embedder = MagicMock()
    embedder.embed.return_value = [1.0, 2.0, 3.0]
    FraiseClient(embedder=embedder).remember("hello world", graph=6)
    assert sent(session)["parameters"] == {"v": [1.0, 2.0, 3.0]}
    embedder.embed.assert_called_once_with("hello world")
    embedder.assert_not_called()


def test_configured_extractor_files_the_fact_under_its_anchors(
    session, sent, callable_extractor
):
    """Extracted anchors follow the given ones, and the text is stored verbatim.

    The extractor reads the text exactly as passed, apostrophe included. Of
    what it finds, "travel" was given already and "Anne" was given in another
    casing; the server folds anchors to lower case, so both would be repeats
    and are dropped. "Lisbon airport" is not one plain word and is quoted.
    """
    extractor = callable_extractor()
    FraiseClient(extractor=extractor).remember(
        "Anne's flight lands at Lisbon airport", topics=["travel"], entities=["anne"]
    )

    extractor.assert_called_once_with("Anne's flight lands at Lisbon airport")
    assert sent(session)["query"] == (
        "remember@0 'Anne''s flight lands at Lisbon airport' "
        "topic:travel topic:trips entity:anne entity:'Lisbon airport'"
    )


@pytest.mark.parametrize(
    "behaviour",
    [
        {"side_effect": RuntimeError("rate limited")},
        {"side_effect": FraiseError("anchor extraction answer unparseable")},
        {"return_value": None},
        {"return_value": ["travel"]},
    ],
)
def test_a_failed_extraction_costs_the_anchors_never_the_fact(session, sent, behaviour):
    """Whatever goes wrong in the extractor, the fact is still stored.

    A raised error, or an answer that is not a list of anchors, leaves the
    fact with only the anchors it was given, and a FraiseWarning names the
    failure at the caller's line. Losing a message to its tagging would be
    the worse trade: the anchors only help find a fact that has to exist.
    """
    extractor = MagicMock(**behaviour)
    del extractor.extract
    with pytest.warns(FraiseWarning, match="anchor extraction failed") as record:
        FraiseClient(extractor=extractor).remember(
            "the kettle whistles", topics=["kitchen"]
        )

    assert record[0].filename == __file__
    assert sent(session)["query"] == "remember@0 'the kettle whistles' topic:kitchen"


def test_extract_false_skips_a_configured_extractor(session, sent, callable_extractor):
    """``extract=False`` stores the fact under the given anchors alone."""
    extractor = callable_extractor()
    FraiseClient(extractor=extractor).remember(
        "the kettle whistles", topics=["kitchen"], extract=False
    )

    extractor.assert_not_called()
    assert sent(session)["query"] == "remember@0 'the kettle whistles' topic:kitchen"


def test_extract_true_without_extractor_raises(session):
    """Asking for extraction with nothing to extract with fails before sending."""
    with pytest.raises(FraiseError, match="no extractor"):
        FraiseClient().remember("the kettle whistles", extract=True)
    session.post.assert_not_called()


def test_extractor_object_is_called_through_its_extract_method(session, sent):
    """An Extractor is called by its named method, never through ``__call__``.

    It exposes both, and ``__call__`` delegates to ``extract``; taking the
    named method is what keeps a subclass that overrides one of them honest.
    """
    extractor = MagicMock()
    extractor.extract.return_value = [Anchor(value="birds", type="topic")]
    FraiseClient(extractor=extractor).remember("the heron fishes at dawn")

    extractor.extract.assert_called_once_with("the heron fishes at dawn")
    extractor.assert_not_called()
    assert sent(session)["query"] == "remember@0 'the heron fishes at dawn' topic:birds"


def test_recall_never_extracts(session, callable_extractor):
    """Extraction belongs to the write path: a recall's text is never tagged."""
    extractor = callable_extractor()
    FraiseClient(extractor=extractor).recall("heron")

    extractor.assert_not_called()


@pytest.mark.integration
def test_client_works_as_a_context_manager(fraise_url):
    """A client built by ``with`` reaches the server inside the block."""
    with FraiseClient(fraise_url) as fraise:
        assert fraise.health() is True


@pytest.mark.integration
def test_an_injected_session_still_reaches_the_server_after_close(fraise_url):
    """Closing the client leaves a caller-supplied session usable.

    The caller owns what the caller passed in. Asserted by using the session
    after the client is closed, rather than by inspecting a mock's close call.
    """
    session = requests.Session()
    with FraiseClient(fraise_url, session=session) as fraise:
        assert fraise.health() is True
    assert session.get(f"{fraise_url}/", timeout=10).status_code == 200
    session.close()


@pytest.mark.integration
def test_client_connects(client):
    """The client reaches the server and reads back a version string."""
    assert client.health() is True
    assert client.server_version() is not None


@pytest.mark.integration
def test_health_is_false_when_the_server_is_unreachable(dead_url):
    """health() swallows the transport error instead of raising.

    It is the probe callers use to decide whether to bother, so it has to
    answer even when nothing is listening.
    """
    assert FraiseClient(dead_url).health() is False


@pytest.mark.integration
def test_server_version_is_none_when_the_server_is_unreachable(dead_url):
    """An unreachable server reports no version rather than raising."""
    assert FraiseClient(dead_url).server_version() is None


@pytest.mark.integration
def test_explain_breaks_down_the_ranking_recall_returns(
    instrument_graph, instrument_topic, client
):
    """Against the live server, explain ranks what recall ranks and says why.

    The same arguments reach the same facts in the same order, since both run
    one pipeline; only explain carries the breakdown, and every hit it returns
    was seen by at least one source. Read-only: recalls write nothing.
    """
    kwargs = {"graph": instrument_graph, "topics": [instrument_topic], "depth": 2}
    recalled = client.recall("cello", **kwargs)
    explained = client.explain("cello", **kwargs)

    assert [hit.value for hit in explained] == [hit.value for hit in recalled]
    assert explained.count >= 1
    assert all(hit.contributions for hit in explained)
    assert "text" in {c.source for c in explained.hits[0].contributions}
    assert explained.background is not None
    assert recalled.background is None


@pytest.mark.integration
def test_check_compatibility_accepts_the_live_server(client):
    """The running server falls inside SUPPORTED_SERVER, silently.

    SUPPORTED_SERVER is a hardcoded range, so this fails the day the image
    moves outside it.
    """
    with warnings.catch_warnings(record=True) as caught:
        warnings.simplefilter("always")
        assert client.check_compatibility() is True
    assert caught == []


@pytest.mark.integration
def test_check_compatibility_strict_does_not_raise(client):
    """Strict mode passes against a supported server instead of raising."""
    assert client.check_compatibility(strict=True) is True


@pytest.mark.integration
def test_remember_then_recall_returns_the_fact(client, round_trip_graph):
    """The round trip: a fact stored through the SDK is found through the SDK."""
    client.remember("the kettle whistles when the water boils", graph=round_trip_graph)
    result = client.recall("kettle", graph=round_trip_graph)
    assert "the kettle whistles when the water boils" in [h.value for h in result]


@pytest.mark.integration
def test_a_fact_is_found_under_the_anchors_extracted_for_it(
    fraise_url, round_trip_graph, callable_extractor
):
    """A fact the extractor tagged is recalled by those anchors, verbatim.

    No anchors were given, so they are the extractor's, and a recall by the
    topic or by the multi-word entity returns the fact exactly as written.
    """
    fact = "Anne's flight lands at Lisbon airport at noon"
    with FraiseClient(fraise_url, extractor=callable_extractor()) as client:
        client.remember(fact, graph=round_trip_graph)
        by_topic = client.recall(graph=round_trip_graph, topics=["trips"])
        by_entity = client.recall(graph=round_trip_graph, entities=["lisbon airport"])

    assert fact in [hit.value for hit in by_topic]
    assert fact in [hit.value for hit in by_entity]


@pytest.mark.integration
def test_a_failed_extraction_still_stores_the_fact(fraise_url, round_trip_graph):
    """An extractor that fails costs the anchors: the fact itself is stored.

    It is found by its own words afterwards, so a failed extraction never
    loses the message it was reading.
    """
    extractor = MagicMock(side_effect=RuntimeError("rate limited"))
    del extractor.extract
    fact = "the lighthouse keeper logs every passing ship"
    with FraiseClient(fraise_url, extractor=extractor) as client:
        with pytest.warns(FraiseWarning, match="anchor extraction failed"):
            client.remember(fact, graph=round_trip_graph)
        result = client.recall("lighthouse", graph=round_trip_graph)

    assert fact in [hit.value for hit in result]


@pytest.mark.integration
def test_a_lone_seed_hub_stays_silent(instrument_graph, instrument_topic, client):
    """A single seed's topic hub holds exactly its fair share, so it transmits
    nothing and its siblings never surface, in either graph lane.

    This is hub silence through the SDK: an anchor is heard only when its
    members matched better than its size predicts. The topic is named because
    the anchor round runs only through an anchor the recall names. depth=1
    admits an anchor only well above its fair share and depth=2 any anchor
    above it, and the hub clears neither bar.
    """
    for depth in (1, 2):
        hits = client.recall(
            "cello", graph=instrument_graph, topics=[instrument_topic], depth=depth
        )
        assert hits.count == 1, f"depth={depth}: a fair-share hub must not transmit"


@pytest.mark.integration
def test_top_caps_the_number_of_results(instrument_graph, instrument_facts, client):
    """``top`` truncates a recall that would otherwise return every match.

    Each fact matches its own instrument keyword, so seeding with all four
    matches every fact. The filler words the facts share ("the", "is") are
    stop words, stripped at index time, so no single common term can match
    them all.
    """
    capped = client.recall(*instrument_facts, graph=instrument_graph, top=2)
    assert capped.count == 2
    assert client.recall(
        *instrument_facts, graph=instrument_graph, top=10
    ).count == len(instrument_facts)


@pytest.mark.integration
def test_a_topic_filter_narrows_a_keyword_recall(
    instrument_graph, instrument_topic, client
):
    """A topic filter alongside a keyword is accepted and narrows the result
    to the tagged match itself.
    """
    filtered = client.recall(
        "cello", graph=instrument_graph, topics=[instrument_topic], depth=2
    )
    assert filtered.count == 1


@pytest.mark.integration
def test_recall_is_reachable_with_an_anchor_and_no_keyword(
    instrument_graph, instrument_topic, client
):
    """``client.recall(topics=[...])``, "everything about X", reaches the
    server instead of failing in the parser.

    An anchor seeds a recall on its own, so a caller need not pad it with a
    keyword they do not mean. The count is not asserted: this pins that the
    question can be asked, not what it returns.
    """
    result = client.recall(graph=instrument_graph, topics=[instrument_topic])
    assert result.count >= 0


@pytest.mark.parametrize("words", [["kettle", "top"], ["since", "7d"], ["Top"]])
@pytest.mark.integration
def test_a_keyword_spelled_search_word_is_not_a_clause(words, client, round_trip_graph):
    """A search word that spells a keyword is a word, wherever it is passed.

    The server reads a bare reserved word as syntax in every position and in
    any casing, so ``recall("since", "7d")`` built bare would be a 400 asking
    whether ``since:7d`` was meant. The builder quotes it and warns, so
    neither the position nor the casing a caller passes a word in decides
    whether their query parses, and the other reading is still named.
    Read-only: recalls write nothing.
    """
    with pytest.warns(FraiseWarning, match="is a reserved word"):
        result = client.recall(*words, graph=round_trip_graph)
    assert result.count >= 0


@pytest.mark.integration
def test_recall_without_a_match_is_empty(client, round_trip_graph, no_match):
    """A keyword no fact contains yields an empty, falsey result."""
    result = client.recall(no_match, graph=round_trip_graph)
    assert result.count == 0
    assert bool(result) is False


@pytest.mark.integration
def test_an_unambiguous_recall_carries_no_warnings(client, round_trip_graph, no_match):
    """A recall with nothing keyword-shaped in it warns about nothing: the
    list is empty and no FraiseWarning is emitted, so callers who escalate
    warnings to errors pass clean queries untouched.
    """
    with warnings.catch_warnings():
        warnings.simplefilter("error", FraiseWarning)
        result = client.recall(no_match, graph=round_trip_graph)

    assert result.warnings == []


@pytest.mark.integration
def test_a_configured_embedder_stores_and_finds_a_fact_by_vector(
    vector_graph, embedding_client
):
    """The implicit-vector path end to end.

    ``remember`` encodes the value, ``recall`` encodes the query phrase, and
    the server matches the two.
    """
    fact = "the barometer falls before a storm"
    embedding_client.remember(fact, graph=vector_graph)
    result = embedding_client.recall("barometer", graph=vector_graph, query=fact)
    assert fact in [h.value for h in result]


@pytest.mark.integration
def test_an_explicit_vector_overrides_the_embedder(
    vector_graph, vector_dim, embedding_client
):
    """An explicit ``vector`` is what reaches the server, not the embedder's.

    Made observable by handing over a vector of the wrong width: the server
    refuses it. Had the client fallen back to encoding the value, the vector
    sent would have been the graph's width and the write would have succeeded,
    so this failing is what proves the override.
    """
    with pytest.raises(FraiseAPIError):
        embedding_client.remember(
            "the sundial needs no winding",
            graph=vector_graph,
            vector=[0.5] * (vector_dim // 2),
        )


@pytest.mark.integration
def test_embed_false_stores_a_fact_without_a_vector(
    vector_graph, client, recalled_values
):
    """Opting out of embedding still produces a valid write.

    Read back through the plain client so the assertion is a pure keyword
    recall — an embedding client would also vector-match its way to every other
    fact in the graph.
    """
    fact = "the hourglass measures three minutes"
    client.remember(fact, graph=vector_graph, embed=False)
    assert fact in recalled_values("hourglass", vector_graph)


@pytest.mark.integration
def test_a_mismatched_vector_dimension_is_rejected(vector_graph, vector_dim, client):
    """A vector of the wrong width is a 400 naming the expected and supplied
    dimensions: a client error the caller can correct, not a server fault to
    retry.
    """
    with pytest.raises(FraiseAPIError) as excinfo:
        client.remember(
            "a vector of the wrong width",
            graph=vector_graph,
            vector=[0.5] * (vector_dim // 2),
        )
    assert excinfo.value.status_code == 400
    assert f"expects {vector_dim}, got {vector_dim // 2}" in excinfo.value.message


@pytest.mark.integration
def test_query_returns_the_decoded_body(client, instrument_graph, no_match):
    """The raw escape hatch returns the server's decoded JSON body.

    It takes the populated graph, not the bare id: a recall of a graph holding
    nothing is answered 204 with no body at all, so an unpopulated graph would
    make this assert against ``{}``.
    """
    body = client.query(f"recall@{instrument_graph} {no_match}")
    assert body["results"] == {"count": 0, "hits": []}


@pytest.mark.integration
def test_query_returns_the_write_acknowledgement(client, round_trip_graph):
    """A write answers with its own shape, not a recall's empty envelope.

    Pinned against the live server because it is a wire contract:
    ``{"status": "ok"}`` is what tells a stored fact from a search that found
    nothing.
    """
    body = client.query(
        f"remember@{round_trip_graph} 'the acknowledgement is its own shape'"
    )

    assert body == {"status": "ok"}


@pytest.mark.integration
def test_recall_of_an_empty_graph_says_the_graph_is_empty(client, empty_graph):
    """A recall of a graph nothing was ever written to sets ``empty``.

    The server carries this in the status line (204) and it survives into the
    result. Without it the caller sees the same empty result as a query that
    missed, and debugs the query when it should be checking whether it ever
    wrote.
    """
    result = client.recall("zebras", graph=empty_graph)

    assert result.empty is True
    assert result.count == 0
    assert list(result) == []


@pytest.mark.integration
def test_recall_that_missed_on_a_populated_graph_is_not_empty(
    client, instrument_graph, no_match
):
    """The other half of the pair, against the live server.

    Same empty result set, opposite ``empty`` — the graph holds facts,
    the keyword just matched none of them.
    """
    result = client.recall(no_match, graph=instrument_graph)

    assert result.empty is False
    assert result.count == 0


@pytest.mark.integration
def test_query_surfaces_a_server_error_as_an_api_error(client):
    """A rejected query becomes FraiseAPIError carrying the server's message.

    The error envelope the client unpacks is the server's, not a fixture's, so
    this fails if the server stops reporting failures as ``{"error": ...}``.
    """
    with pytest.raises(FraiseAPIError) as excinfo:
        client.query("this is not a query")
    assert excinfo.value.status_code == 400
    assert "parse error" in excinfo.value.message


@pytest.mark.integration
def test_an_unreachable_server_raises_fraise_error(dead_url, round_trip_graph):
    """A transport failure arrives as FraiseError, not a requests exception.

    A refused connection is the deterministic way to reach that branch; a
    timeout races a local server that answers faster than the clock.
    """
    with pytest.raises(FraiseError, match="could not reach fraise"):
        FraiseClient(dead_url).query(f"recall@{round_trip_graph} anything")
