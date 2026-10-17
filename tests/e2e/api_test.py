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

"""HTTP surface and request validation.

Covers the health check, malformed request bodies, query strings the parser
must reject as client errors, and the grammar boundary that keeps reserved
words usable as ordinary data.
"""

import pytest
import requests


def test_health_check(get):
    """The health check answers 200 with ``status: ok`` and the server's version.

    The version is the SDK's compatibility handshake, so it must be present.
    """
    response = get("/")

    assert response.status_code == 200
    assert response.json()["status"] == "ok"
    assert response.json().get("version") is not None


def test_query_rejects_malformed_json(base_url, request_timeout):
    """A body that is not JSON is a 400, before any query is parsed."""
    response = requests.post(
        f"{base_url}/api/v1/q",
        data="{not json",
        headers={"Content-Type": "application/json"},
        timeout=request_timeout,
    )

    assert response.status_code == 400


def test_query_rejects_unparsable_query(query):
    """A query the parser rejects is a 400 carrying an error message."""
    status, body = query("bogus nonsense")

    assert status == 400
    assert body.get("error"), "expected a parse error message"


def test_query_rejects_out_of_range_graph(query, num_graphs):
    """A selector past the allocated range is a fast client error, not a hang.

    The selector comes from the server's own allocation. So it stays out of
    range whatever count the suite's config sets.
    """
    status, body = query(f"recall@{num_graphs} zebras")

    assert status == 400
    assert body.get("error"), "expected an out-of-range error message"


# Graph selector validation has two layers, each with its own error. The parser
# rejects anything that does not fit the uint8 selector type. Without that
# check, uint8 narrowing would wrap @256 to graph 0, a tenant isolation hole.
# The handler rejects a selector that fits the type but names a graph the store
# never allocated. The tests below pin each layer's error by message, so a
# regression in one cannot hide behind the other.


def test_query_rejects_selector_that_would_wrap(query):
    """The parser rejects a selector above the uint8 range before narrowing it.

    Narrowed, @256 would wrap to graph 0 and @300 to graph 44. The query would
    then run silently against another tenant's graph.
    """
    for q in ("remember@256 'secret plan' topic:x", "recall@300 zebras"):
        status, body = query(q)

        assert status == 400, f"{q!r}: expected 400, got {status}"
        assert "out of range" in body.get("error", ""), (
            f"{q!r}: expected the parser's out-of-range error, got {body.get('error')!r}"
        )


def test_query_rejects_non_integer_selector(query):
    """A non-numeric selector is a parse error, not a silent fallback to a
    default graph.
    """
    status, body = query("recall@abc zebras")

    assert status == 400
    assert body.get("error"), "expected a parse error message"


def test_query_rejects_valid_uint8_selector_above_num_graphs(query):
    """@255 fits the selector type, so the parser passes it to the handler.

    The handler must then reject it against the allocated graph count, with its
    own error. This is the layer boundary: the parser checks the type, and the
    handler checks the allocation.
    """
    status, body = query("recall@255 zebras")

    assert status == 400
    assert "does not exist" in body.get("error", ""), (
        f"expected the handler's does-not-exist error, got {body.get('error')!r}"
    )


def test_wrapping_selector_write_does_not_leak_to_graph_zero(query):
    """A rejected remember@256 leaves no trace on graph 0.

    Graph 0 is where @256 would wrap, so this guards against the wrap itself.
    The test only reads graph 0, so it does not disturb that graph's facts.
    """
    status, _ = query("remember@256 'wrapprobe should never land' topic:wrapprobe")
    assert status == 400

    status, body = query("recall@0 wrapprobe")
    # The test asserts only the probe's absence. Both success shapes show it:
    # 200 with no hits, or the 204 of an empty graph. The suite primes graph 0,
    # so 200 is what arrives. Pinning 200 would tie this wrap test to the
    # primer.
    assert status in (200, 204), body.get("error")
    assert body.get("results", {}).get("count", 0) == 0, (
        "a rejected @256 write leaked onto graph 0 — uint8 wrap regression"
    )


@pytest.mark.parametrize(
    "text",
    [
        "recall x depth:abc",
        "recall x top:abc",
        "recall x top:99999999999999999999",  # top overflows int
        "recall x since:yesterday",
        "recall x until:soon",
        "recall@abc x",
        "recall@999 x",  # selector overflows uint8 (would wrap to @231)
    ],
)
def test_query_rejects_invalid_modifier_value(query, text):
    """An invalid depth/top/since/until/selector value is a 400 with a message.

    It never falls back silently to the default or to no constraint. Agents
    correct themselves from the error. A result scoped differently, with no
    error, is the worst failure mode for this product.
    """
    status, body = query(text)

    assert status == 400, f"{text!r} should be rejected, got {status}: {body}"
    assert body.get("error"), f"expected a parse error message for {text!r}"


# A keyed field is `key:value`, and the parser checks the ':', never skips it.
# Skipping it would shift every later token one role to the left. "recall x
# since 7d 30d" would answer with a 30d bound and no error. "recall x topic
# food extra" would filter by nothing. The tests below cover the whole
# keyed-field family, because each clause has its own parse helper
# (parseIntField, parseAnchorField, parseTimeValue). The contract must hold
# across all of them.


@pytest.mark.parametrize(
    "text,expected",
    [
        ("recall zebras topic food", "write topic:food"),
        ("recall zebras topic food extra", "write topic:food"),
        ("recall zebras entity alice", "write entity:alice"),
        ("recall zebras since 7d", "write since:7d"),
        ("recall zebras until 30d", "write until:30d"),
        ("recall zebras top 5", "write top:5"),
        ("recall@0 top", "write top:<value>"),  # nothing follows, so no value to name
        ("recall zebras depth 2", "write depth:2"),
        (
            "recall zebras topic:food entity alice",  # one good field, one bad
            "write entity:alice",
        ),
        (
            "remember@5 'ulysse moved to quimper' topic relocation",
            "write topic:relocation",
        ),
        ("remember@5 'ulysse moved to quimper' entity ulysse", "write entity:ulysse"),
    ],
)
def test_query_rejects_missing_field_separator(query, text, expected):
    """A keyed field written without its ':' is a 400 naming the fix.

    The message writes the clause the caller meant. Its value is the word
    written after the keyword (topic:food), or <value> when nothing follows.
    Anything else is the silent-parse failure mode: the query runs, returns
    200, and answers a question nobody asked.
    """
    status, body = query(text)

    assert status == 400, f"{text!r} should be rejected, got {status}: {body}"
    assert expected in body.get("error", "").lower(), (
        f"{text!r}: expected an error naming the missing ':', got {body.get('error')!r}"
    )


@pytest.mark.parametrize(
    "text",
    [
        "recall zebras since 7d 30d",
        "recall zebras until 7d 30d",
        "recall zebras since:7d until 30d 60d",
    ],
)
def test_query_rejects_shifted_time_bound(query, text):
    """These shapes would parse cleanly if the separator were skipped.

    They are why a missing ':' is an error, not a typo to tolerate. With the
    separator skipped, `since 7d 30d` would take `7d` as the separator and
    `30d` as the bound. The 200 would carry results scoped four times wider
    than asked. An agent would have no signal to notice that, so these must be
    rejected, not interpreted on a best-effort basis.
    """
    status, body = query(text)

    assert status == 400, (
        f"{text!r} parsed instead of failing — the token shift is back, and the "
        f"result is scoped by the wrong bound: {body}"
    )


def test_missing_separator_write_does_not_land(query):
    """A rejected remember must not have written anything.

    The 400 covers the parse, and this test covers execution. A missing colon
    that mis-assigned the anchors instead of failing would commit the fact
    under the wrong topic. No later recall would find it there.
    """
    status, _ = query("remember@5 'colonprobe should never land' topic colonprobe")
    assert status == 400

    status, body = query("recall@5 colonprobe")
    # As above, either success shape shows the probe is absent. That is all
    # this test claims.
    assert status in (200, 204), body.get("error")
    assert body.get("results", {}).get("count", 0) == 0, (
        "a rejected write landed on graph 5 — the parse error did not stop execution"
    )


# A reserved word is syntax only where a clause can start. In value position,
# after a field's ':', it reads as an ordinary word. So an entity called "top"
# needs no quoting. As a search term, "top" is quoted. Two rules hold that
# line. A keyword immediately followed by ':' is always a field. A keyword is
# lower case only. With any upper case where a clause could start, it is an
# error naming the casing, never a term. Glued to a ':', it runs as that
# clause, with a warning naming the casing. The tests below pin every side: the
# shapes that parse, the keyword-colon shapes that must stay rejected, the
# mis-cased shapes that must never pass silently as data, and the casing
# warnings.


@pytest.mark.parametrize(
    "text",
    [
        "recall@0 'Top'",  # upper case is legal in data position, and folds to the same word
        "recall@0 'top' top:3",  # same spelling as term and clause, told apart by the quotes and the ':'
        "recall@0 shelf entity:top",  # a keyword as an anchor value, on the read side
        "recall@0 shelf topic:top",
        "recall@0 shelf entity:Top",  # an anchor value may carry any casing, keyword spelling or not
        "recall@0 shelf entity:since topic:recall",  # every keyword, not just "top"
    ],
)
def test_query_accepts_keyword_in_value_position(query, text):
    """A keyword after a field's ':', or quoted as a recall term, is data.

    The query parses and runs. An LLM extracting entities from prose will
    eventually emit a keyword bare ("she reached the top" -> entity:top). A
    parser that typed "top" by spelling alone would reject that write with a
    400 the client could not foresee. The probes are recalls of the same
    shapes, because recalls are read-only. They prove acceptance without
    writing anything.
    """
    status, body = query(text)

    assert status == 200, f"{text!r} should parse, got {status}: {body.get('error')!r}"


@pytest.mark.parametrize(
    "text",
    [
        "recall@0 top:3",  # keyword+':' is the top clause, and a recall still needs a seed
        "recall@0 shelf top",  # clause position: a bare keyword is a clause missing its ':'
        "remember@5 'x' entity:top:3",  # keyword+':' after the anchor's ':' is a field, not a value
        "remember@5 'x' entity:since:7d",  # the same for a time field
    ],
)
def test_query_keeps_keyword_colon_as_a_field(query, text):
    """A keyword immediately followed by ':' reads as a field.

    It never reads as a value that happens to precede a stray ':'. This
    boundary makes keywords safe to accept as values. Without it,
    `entity:top:3` would need a guess. A bare keyword in clause position that
    quietly became a search term would revive the silent-shift family. The
    separator tests above exist to prevent that family. An error an agent can
    correct from beats a 200 that answers a question nobody asked.
    """
    status, body = query(text)

    assert status == 400, f"{text!r} should be rejected, got {status}: {body}"
    assert body.get("error"), f"expected a parse error message for {text!r}"


@pytest.mark.parametrize(
    "text",
    [
        "Recall zebras",  # command position: commands are lower case
        "REMEMBER 'a shouted fact' topic:x",
        "recall zebras Since 7d",  # would read as a three-term search without the check
        "recall zebras Since 7d 30d",  # the shifted time-bound shape, through the casing door
        "recall zebras Depth 2",  # the same shape on a modifier
    ],
)
def test_query_rejects_miscased_keyword(query, text):
    """A mis-cased keyword is a 400 wherever it would read as syntax.

    Any upper case counts, because casing does not un-reserve a word. Keywords
    are lower-case syntax. Upper case is legal only where a token is plainly
    data: a term, a phrase or an anchor value. The last three shapes are the
    dangerous ones. Case folding of terms would read `recall zebras Since 7d`
    as a three-term search. That is a 200 scoped by nothing, with no signal to
    correct from. It would revive the silent-shift family above through the
    casing door.

    A keyword glued to a ':' is the exception and is not listed here. A word
    before a colon can be nothing else, so it runs as that clause with a
    warning (see test_miscased_clause_before_a_colon_is_that_clause).
    """
    status, body = query(text)

    assert status == 400, f"{text!r} should be rejected, got {status}: {body}"
    assert body.get("error"), f"expected a parse error message for {text!r}"


def test_miscased_keyword_error_names_the_casing(query):
    """The 400 for a mis-cased clause keyword names the casing and the fix.

    It tells the agent what is wrong and how to get the word instead:
    lower-case the keyword, or quote the word. Without the hint, the error for
    `Since` would blame a stray ':' or nothing at all. The error must let the
    agent correct itself.
    """
    status, body = query("recall zebras Since 7d")

    assert status == 400
    error = body.get("error", "")
    assert "lower case" in error, f"want the casing named, got {error!r}"
    assert "'Since'" in error, f"want the quoted escape shown, got {error!r}"


@pytest.mark.parametrize(
    "text",
    [
        "recall@0 zebras TOP:3",
        "recall@0 zebras Topic:food",
        "recall@0 zebras Depth:2",
        "recall@0 zebras Since:7d",
        "recall@0 zebras Entity:bob",
    ],
)
def test_miscased_clause_before_a_colon_is_that_clause(query, text):
    """A keyword glued to a ':' is that clause whatever its casing.

    This is the one place casing is forgiven. No production puts a bare word
    before a colon, so `TOP:3` has one reading. The cases are reads only. A
    mis-cased write would add a fact on graph 5, where other tests depend on
    exact counts.
    """
    status, body = query(text)

    assert status == 200, f"{text!r} should parse, got {status}: {body}"


@pytest.mark.parametrize(
    "text,keyword",
    [
        ("recall@0 zebras TOP:3", "TOP"),
        ("recall@0 zebras Topic:food", "Topic"),
        ("recall@0 zebras Since:7d", "Since"),
    ],
)
def test_a_miscased_clause_runs_but_warns(query, text, keyword):
    """A forgiven casing still warns: the clause runs and the response says so.

    The colon leaves one reading. Rejecting it would report the casing instead
    of whatever the casing hides. But this language is lower case, and a 200
    with nothing attached teaches the opposite. The warning names the spelling
    that ran, so a caller can correct the habit from the response alone.
    """
    status, body = query(text)

    assert status == 200, f"{text!r} should parse, got {status}: {body}"
    warnings = body.get("warnings") or []
    assert warnings, f"{text!r}: expected a casing warning, got none"
    joined = " ".join(str(w) for w in warnings)
    assert "lower case" in joined, joined
    assert f'"{keyword}"' in joined, joined


def test_a_miscased_anchor_value_is_data_and_stays_silent(query):
    """An anchor value is folded by design, so its casing draws no warning.

    `entity:Top` and `entity:top` are the same anchor, so an agent never has to
    remember how it capitalised something. A warning here would contradict
    that. So the casing warning covers the clause key alone.
    """
    status, body = query("recall@0 zebras entity:Top topic:Food")

    assert status == 200, body
    assert "warnings" not in body, body.get("warnings")


def test_a_miscased_repeat_is_still_a_duplicate(query):
    """`depth:2 DEPTH:5` is a duplicate, not a casing complaint.

    This is why the casing above is forgiven, not reported. Blaming the casing
    would name the shallower of the two mistakes. An agent that lower-cased the
    keyword as told would get a silently rescoped query back. The duplicate
    error also proves that the clause was recognised.
    """
    status, body = query("recall@0 zebras depth:2 DEPTH:5")

    assert status == 400, body
    assert "duplicate" in body.get("error", "").lower(), body.get("error")


@pytest.mark.parametrize(
    "text",
    [
        "recall@0 zebras",  # nothing keyword-shaped anywhere
        "recall@0 'since' 7d",  # quoting the term states the intent
        "recall@0 zebras entity:top",  # anchor values are unambiguous, never warned about
        "recall@0 zebras since:7d",  # an actual clause is what it says it is
        "recall@0 zebras topic:food depth:2",  # a depth beside an anchor selects a lane that runs
        "recall@0 zebras depth:0",  # the floor asks for no graph at all
        "recall@0 'top'",  # a quoted reserved word is a plain term
        "recall@0 'Top' 'since'",
        "recall@0 zebras 'topic' food",
        "recall@0 'the parrot'",  # a phrase never warns for its content
        "recall@0 'the'",
        "recall@0 topic:birds",  # anchor-seeded, no term to warn about
    ],
)
def test_unambiguous_query_carries_no_warnings_key(query, text):
    """A query with nothing to warn about has no warnings key at all.

    The key appears only when there is something to say. So the common case
    carries no extra key, and a client checking `"warnings" in body` gets a
    real signal, not a constant empty list. Quoting is how a caller searches
    for a reserved word or a stop word, so a quoted term must not warn.
    """
    status, body = query(text)

    assert status == 200, body.get("error")
    assert "warnings" not in body, (
        f"{text!r} should be warning-free, got {body['warnings']}"
    )


# depth selects the retrieval lane. depth:0 is the floor, with no anchor
# traversal. depth:1 and depth:2 run the one anchor round at different
# admission bars. Those are the only meaningful values, so the ceiling
# (db.max-depth, 2 by default) admits exactly 0-2. Malformed values are parse
# errors, and values over the ceiling are limit errors. recall_test.py pins
# the lane semantics. These tests pin the rejections.


@pytest.mark.parametrize(
    "text",
    [
        "recall x depth:",  # the clause with no value at all
        "recall x depth:-1",  # '-' is its own token, so a negative cannot be written
        "recall x depth:1.5",  # a lane selector is an integer, not a distance
        "recall x depth:99999999999999999999",  # overflows int
    ],
)
def test_query_rejects_malformed_depth(query, text):
    """A depth that is not a non-negative integer is a parse error.

    This complements the ceiling test below. This family never reaches the
    limit check, because there is no number to compare.
    """
    status, body = query(text)

    assert status == 400, f"{text!r} should be rejected, got {status}: {body}"
    assert body.get("error"), f"expected a parse error message for {text!r}"


@pytest.mark.parametrize("text", ["recall x depth:3", "recall x depth:99"])
def test_query_rejects_depth_past_the_ceiling(query, text):
    """A depth above 2 is refused, not silently treated as depth:2.

    The search runs one anchor round and no more, so a larger depth has no
    meaning. Answering it anyway would tell an agent its request was honoured
    when it was quietly downgraded. That is the silent-reinterpretation failure
    the missing-separator tests above exist to prevent. The message names the
    ceiling so the agent can correct the query.
    """
    status, body = query(text)

    assert status == 400, f"{text!r} should be rejected, got {status}: {body}"
    assert "out of range (0-2)" in body.get("error", ""), body.get("error")


@pytest.mark.parametrize(
    "text", ["recall x depth:0", "recall x depth:1", "recall x depth:2"]
)
def test_query_accepts_every_valid_depth(query, text):
    """0, 1 and 2 are the whole accepted range, and each parses.

    It sits beside the rejections to show the boundary in one place. Depth 2
    is accepted, and depth 3 is not.
    """
    status, body = query(text)

    assert status == 200, f"{text!r} should parse, got {status}: {body.get('error')!r}"


@pytest.mark.parametrize("text", ["recall@0 zebras depth:1", "recall@0 zebras depth:2"])
def test_depth_without_an_anchor_runs_but_warns(query, text):
    """A depth above the floor, with no anchor named, runs with a warning.

    The response says the clause had no effect. The graph is entered only
    through a topic:/entity: the recall names. With none, the recall is a text
    search whatever its depth. Honouring the clause in silence would tell an
    agent that transmission happened when it did not. So the warning names the
    clause and what would give it effect.
    """
    status, body = query(text)

    assert status == 200, f"{text!r} should parse, got {status}: {body}"
    warnings = body.get("warnings") or []
    assert len(warnings) == 1, f"{text!r}: want exactly one warning, got {warnings}"
    assert "has no effect" in warnings[0], warnings[0]
    assert "names none" in warnings[0], warnings[0]


@pytest.mark.parametrize(
    ("text", "blame"),
    [
        ("recall zebras topic food extra", "topic"),
        ("recall zebras entity alice extra", "entity"),
        ("recall zebras since 7d 30d", "since"),
        ("recall zebras top 5 depth:2", "top"),
        ("recall zebras depth 2 top:3", "depth"),
        ("recall zebras since:soon top:3", "soon"),
        ("recall zebras depth:abc top:3", "abc"),
        ("recall@abc zebras top:3", "abc"),
    ],
)
def test_parse_error_column_points_at_the_offending_token(query, text, blame):
    """The 400 must point at the last character of the token it quotes.

    The column is the only part of the response that says *where* the query
    went wrong, and a client draws its caret from it. The lexer's read cursor
    runs a token ahead of the parser. A column taken from it would point an
    error quoting `food` at the end of the next token. Every case here keeps a
    token to the right of the bad one, because only then do the two positions
    differ.
    """
    status, body = query(text)
    error = body.get("error", "")
    column = text.index(blame) + len(blame)

    assert status == 400, f"{text!r} should be rejected, got {status}: {body}"
    assert f"parse error at column {column}:" in error, (
        f"{text!r}: want column {column} (end of {blame!r}), got {error!r}"
    )
    assert f'"{blame}"' in error, (
        f"{text!r}: the message should quote the token it blames, got {error!r}"
    )


@pytest.mark.parametrize(
    ("text", "detail"),
    [
        ("recall x since:soon", 'invalid since value "soon"'),
        ("recall x until:later", 'invalid until value "later"'),
        ("recall x depth:abc", 'invalid depth value "abc"'),
        ("recall x top:abc", 'invalid top value "abc"'),
    ],
)
def test_query_parse_error_message_is_unmangled(query, text, detail):
    """The clause helpers' positioned errors must reach the client verbatim.

    A call site that re-wrapped them with a bad %e verb would turn a clean
    'invalid since value "soon"' into `&{%!e(string=...)}` in the 400 body.
    That garbles the message an agent needs to correct itself.
    """
    status, body = query(text)
    error = body.get("error", "")

    assert status == 400, f"{text!r} should be rejected, got {status}: {body}"
    assert detail in error, f"{text!r}: expected {detail!r} in the body, got {error!r}"
    assert "%!e" not in error, f"{text!r}: mangled error surfaced: {error!r}"
    assert "Error while parsing" not in error, (
        f"{text!r}: re-wrapped error surfaced: {error!r}"
    )
    assert "parse error at column" in error, (
        f"{text!r}: position lost from the error: {error!r}"
    )


# A write, a recall of an empty graph and a recall that matched nothing answer
# in three shapes: the write's acknowledgement, a 204, and a 200 with
# {"count":0,"hits":[]}. With one shared shape, a stored fact would look
# byte-identical to a failed search. A caller who never wrote to a graph would
# get the same answer as one whose query missed. These tests pin the three
# apart. The Python SDK and the MCP bridge both parse against them.


def test_an_accepted_write_is_acknowledged_not_answered_with_results(query):
    """A write comes back 200 with its own acknowledgement and no result set.

    The "status" key is part of the contract. Its presence tells an accepted
    write apart from a recall that matched nothing.
    """
    status, body = query("remember@1 'ackprobe is a loose remember'")

    assert status == 200, body.get("error")
    assert body == {"status": "ok"}


def test_a_recall_of_an_empty_graph_is_204_with_no_body(query):
    """Graph 8 is never written to, so a recall of it answers 204.

    A 204 must not carry a body, and none is needed. The whole answer is
    "nothing has been stored here". No result set could say that without also
    looking like an ordinary miss.
    """
    status, body = query("recall@8 zebras")

    assert status == 204, body.get("error")
    assert body == {}


def test_a_recall_that_matched_nothing_on_a_populated_graph_stays_200(query):
    """A populated graph that matched nothing keeps the empty result envelope.

    This is the distinction the 204 exists for. The test writes to the graph
    first instead of assuming it is populated. Files run in any order, and a
    graph that happened to be empty would answer 204 and fail for the wrong
    reason.
    """
    status, body = query("remember@1 'the barometer falls before the storm'")
    assert status == 200, body.get("error")

    status, body = query("recall@1 zzznomatchzzz")

    assert status == 200, body.get("error")
    assert body["results"]["count"] == 0
    assert body["results"]["hits"] == []
