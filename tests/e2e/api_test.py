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

"""HTTP surface and request validation: the health check, malformed request
bodies, query strings the parser must reject as client errors, and the
grammar boundary that keeps reserved words usable as ordinary data.
"""

import pytest
import requests


def test_health_check(get):
    response = get("/")

    assert response.status_code == 200
    assert response.json()["status"] == "ok"
    assert response.json().get("version") is not None


def test_query_rejects_malformed_json(base_url, request_timeout):
    response = requests.post(
        f"{base_url}/api/v1/q",
        data="{not json",
        headers={"Content-Type": "application/json"},
        timeout=request_timeout,
    )

    assert response.status_code == 400


def test_query_rejects_unparsable_query(query):
    status, body = query("bogus nonsense")

    assert status == 400
    assert body.get("error"), "expected a parse error message"


def test_query_rejects_out_of_range_graph(query, num_graphs):
    """A selector past the allocated graph range is a fast client error, not a
    hang. The selector is taken from the server's own allocation so it stays
    out of range whatever the suite's config sets the count to.
    """
    status, body = query(f"recall@{num_graphs} zebras")

    assert status == 400
    assert body.get("error"), "expected an out-of-range error message"


# Graph selector validation is layered, and each layer has its own error:
# the parser rejects anything that does not faithfully fit the uint8 selector
# type (otherwise uint8 narrowing would wrap @256 to graph 0 — a tenant
# isolation hole), and the handler rejects selectors that fit the type but
# name a graph the store never allocated. The tests below pin each layer's
# error separately, by message, so a regression in one cannot hide behind the
# other still firing.


def test_query_rejects_selector_that_would_wrap(query):
    """The parser must reject a selector above the uint8 range before it is
    narrowed: @256 would wrap to graph 0 and @300 to graph 44, silently
    executing against another tenant's graph.
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
    """@255 fits the selector type, so the parser passes it — the handler must
    then reject it against the allocated graph count with its own distinct
    error. This is the layer boundary: type consistency in the parser,
    allocation policy in the handler.
    """
    status, body = query("recall@255 zebras")

    assert status == 400
    assert "does not exist" in body.get("error", ""), (
        f"expected the handler's does-not-exist error, got {body.get('error')!r}"
    )


def test_wrapping_selector_write_does_not_leak_to_graph_zero(query):
    """Regression guard for the wrap itself: a rejected remember@256 must leave
    no trace on graph 0 (the graph @256 would wrap to). Read-only on graph 0
    apart from the probe recall, so it does not disturb that graph's facts.
    """
    status, _ = query("remember@256 'wrapprobe should never land' topic:wrapprobe")
    assert status == 400

    status, body = query("recall@0 wrapprobe")
    # What this asserts is the probe's absence, which both successful shapes
    # attest to: 200 with no hits, or the 204 an empty graph answers with. The
    # suite primes every claimed graph so the first is what actually arrives,
    # but pinning it would tie a wrap regression test to that arrangement.
    assert status in (200, 204), body.get("error")
    assert body.get("results", {}).get("count", 0) == 0, (
        "a rejected @256 write leaked onto graph 0 — uint8 wrap regression"
    )


@pytest.mark.parametrize(
    "text",
    [
        "recall x depth:abc",  # non-numeric depth
        "recall x top:abc",  # non-numeric top
        "recall x top:99999999999999999999",  # top overflows int
        "recall x since:yesterday",  # unparseable time bound
        "recall x until:soon",  # unparseable time bound
        "recall@abc x",  # non-numeric graph selector
        "recall@999 x",  # selector overflows uint8 (would wrap to @231)
    ],
)
def test_query_rejects_invalid_modifier_value(query, text):
    """An invalid depth/top/since/until/selector value is a 400 with a message,
    not a silent fall back to the default or to no constraint at all. Agents
    self-correct from the error; a differently-scoped result with no error is
    the worst failure mode for this product.
    """
    status, body = query(text)

    assert status == 400, f"{text!r} should be rejected, got {status}: {body}"
    assert body.get("error"), f"expected a parse error message for {text!r}"


# A keyed field is `key:value`, and the ':' is checked, never skipped. Skipping
# it would shift every following token one role to the left: "recall x since
# 7d 30d" would answer with a 30d bound and no error at all, and "recall x
# topic food extra" would filter by nothing. The tests below cover the whole
# keyed-field family together, because each clause is parsed by its own helper
# (parseIntField, parseAnchorField, parseTimeValue) and the contract has to
# hold across all of them.


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

    The message writes the clause the caller meant, with the word they wrote
    after the keyword as its value (topic:food), and falls back to <value> when
    nothing follows. Anything else is the silent-parse failure mode: the query
    runs, returns 200, and answers a question nobody asked.
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
    """The shapes that would parse clean if the separator were skipped, and
    the reason a missing ':' is an error rather than a typo to tolerate.

    With the separator skipped, `since 7d 30d` would consume `7d` as the
    separator and take `30d` as the bound: a 200 carrying results scoped four
    times wider than asked for. There is no signal an agent could use to
    notice that, which is why these must be rejected rather than best-effort
    interpreted.
    """
    status, body = query(text)

    assert status == 400, (
        f"{text!r} parsed instead of failing — the token shift is back, and the "
        f"result is scoped by the wrong bound: {body}"
    )


def test_missing_separator_write_does_not_land(query):
    """A rejected remember must not have written anything.

    The 400 covers the parse; this covers execution. A missing colon that
    mis-assigned the anchors rather than failing would commit the fact under
    the wrong topic, where no later recall would find it.
    """
    status, _ = query("remember@5 'colonprobe should never land' topic colonprobe")
    assert status == 400

    status, body = query("recall@5 colonprobe")
    # As above: either successful shape attests that the probe is absent,
    # which is the whole claim here.
    assert status in (200, 204), body.get("error")
    assert body.get("results", {}).get("count", 0) == 0, (
        "a rejected write landed on graph 5 — the parse error did not stop execution"
    )


# A reserved word is only syntax where a clause can start. In value position,
# the right-hand side of a field's ':', it reads as an ordinary word, so an
# entity that happens to be called "top" needs no quoting; as a search term it
# is quoted. Two rules hold that line: a keyword immediately followed by ':'
# is always a field, and a keyword is lower-case only. Written with any upper
# case where a clause could start, it is an error naming the casing, never a
# term; glued to a ':', it runs as that clause with a warning naming the
# casing. The tests below pin every side: the shapes that parse, the
# keyword-colon shapes that must stay rejected, the mis-cased shapes that must
# never be silently swallowed as data, and the casing warnings.


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
    """A keyword on the right of a field's ':', or quoted as a recall term, is
    data: the query parses and runs.

    An LLM extracting entities from prose will eventually emit a keyword bare
    ("she reached the top" -> entity:top), and a parser that typed "top" by
    spelling alone would fail that write with a 400 the client could not
    anticipate. These probes are recalls of the same shapes, chosen because
    they are read-only: acceptance is proven without writing anything.
    """
    status, body = query(text)

    assert status == 200, f"{text!r} should parse, got {status}: {body.get('error')!r}"


@pytest.mark.parametrize(
    "text",
    [
        "recall@0 top:3",  # keyword+':' is the top clause, and a recall still needs a seed
        "recall@0 shelf top",  # clause position: a bare keyword is a clause missing its ':'
        "remember@5 'x' entity:top:3",  # keyword+':' after the anchor's ':' is a field, not a value
        "remember@5 'x' entity:since:7d",  # ditto for a time field
    ],
)
def test_query_keeps_keyword_colon_as_a_field(query, text):
    """A keyword immediately followed by ':' still reads as a field — never as
    a value that happens to precede a stray ':'.

    This boundary is what makes accepting keywords as values safe: without
    it, `entity:top:3` would need a guess, and a bare keyword in clause
    position quietly becoming a search term would revive the silent-shift
    family the separator tests above exist to prevent. An error an agent can
    correct from beats a 200 answering a question nobody asked.
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
    """A keyword written with any upper case is a 400 wherever it would read
    as syntax — casing does not un-reserve a word.

    Keywords are lower-case syntax; upper case is only legal where a token is
    unambiguously data (a term, a phrase, an anchor value). The dangerous
    shapes are the last three: case folding of terms would read
    `recall zebras Since 7d` as a three-term search, a 200 scoped by nothing
    with no signal to correct from, reviving the silent-shift family above
    through the casing door.

    A keyword glued to a ':' is the exception and is not listed here: a word
    before a colon can be nothing else, so it runs as that clause with a
    warning rather than failing (see
    test_miscased_clause_before_a_colon_is_that_clause).
    """
    status, body = query(text)

    assert status == 400, f"{text!r} should be rejected, got {status}: {body}"
    assert body.get("error"), f"expected a parse error message for {text!r}"


def test_miscased_keyword_error_names_the_casing(query):
    """The 400 for a mis-cased clause keyword tells the agent what is wrong
    and how to get the word instead: lower-case the keyword, or quote the
    word. Without the hint, the error for `Since` would blame a stray ':' or
    nothing at all, and the one thing the error must enable is
    self-correction.
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

    This is the one place casing is forgiven: no production puts a bare word
    in front of a colon, so `TOP:3` has one reading. Reads only: a mis-cased
    write would add a fact the graph-5 counts elsewhere depend on.
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
    """Forgiven is not unremarked: the clause runs and the response says so.

    The colon leaves one reading, so rejecting it would report the casing
    instead of whatever the casing is hiding — but this language is lower case,
    and a 200 with nothing attached teaches the opposite. The warning names the
    spelling that ran so a caller can correct the habit from the response alone.
    """
    status, body = query(text)

    assert status == 200, f"{text!r} should parse, got {status}: {body}"
    warnings = body.get("warnings") or []
    assert warnings, f"{text!r}: expected a casing warning, got none"
    joined = " ".join(str(w) for w in warnings)
    assert "lower case" in joined, joined
    assert f'"{keyword}"' in joined, joined


def test_a_miscased_anchor_value_is_data_and_stays_silent(query):
    """An anchor value is folded by design, so its casing is not remarked on.

    `entity:Top` and `entity:top` are the same anchor — the point being that an
    agent never has to remember how it capitalised something. Warning here would
    contradict that, so the casing warning is scoped to the clause key alone.
    """
    status, body = query("recall@0 zebras entity:Top topic:Food")

    assert status == 200, body
    assert "warnings" not in body, body.get("warnings")


def test_a_miscased_repeat_is_still_a_duplicate(query):
    """`depth:2 DEPTH:5` is a duplicate, not a casing complaint.

    This is why the casing is forgiven above rather than reported: blaming the
    casing would name the shallower of the two mistakes, and an agent that
    dutifully lower-cased it would get a silently rescoped query back. The
    duplicate error is also the proof the clause was recognised at all.
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

    The key appears only when there is something to say, so the response
    shape for the common case is unchanged and a client checking
    `"warnings" in body` gets a real signal, not a constant empty list.
    Quoting is how a reserved word or a stop word is searched for, so a
    quoted term must be silent.
    """
    status, body = query(text)

    assert status == 200, body.get("error")
    assert "warnings" not in body, (
        f"{text!r} should be warning-free, got {body['warnings']}"
    )


# depth selects the retrieval lane: 0 is the floor (no anchor traversal), and 1
# and 2 run the one anchor-mediated round at different admission bars. Those
# are the only meaningful values, so the ceiling (db.max-depth, 2 by default)
# admits exactly 0-2 and everything else is rejected: malformed values as
# parse errors, over-ceiling values as limit errors. The lane *semantics* live
# in recall_test.py; these are the rejections.


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

    Complements the ceiling test below: this family never reaches the limit
    check, because there is no number to compare.
    """
    status, body = query(text)

    assert status == 400, f"{text!r} should be rejected, got {status}: {body}"
    assert body.get("error"), f"expected a parse error message for {text!r}"


@pytest.mark.parametrize("text", ["recall x depth:3", "recall x depth:99"])
def test_query_rejects_depth_past_the_ceiling(query, text):
    """A depth above 2 is refused rather than silently treated as depth:2.

    The search does not iterate past one anchor-mediated round, so a larger
    depth has no meaning. Answering it anyway would tell an agent its request
    was honoured when it was quietly downgraded — the same silent-reinterpretation
    failure the missing-separator tests above exist to prevent. The message
    names the ceiling so the agent can correct.
    """
    status, body = query(text)

    assert status == 400, f"{text!r} should be rejected, got {status}: {body}"
    assert "out of range (0-2)" in body.get("error", ""), body.get("error")


@pytest.mark.parametrize(
    "text", ["recall x depth:0", "recall x depth:1", "recall x depth:2"]
)
def test_query_accepts_every_valid_depth(query, text):
    """0, 1 and 2 are the whole accepted range, and each parses.

    Pinned alongside the rejections so the boundary is visible in one place:
    2 is accepted, 3 is not.
    """
    status, body = query(text)

    assert status == 200, f"{text!r} should parse, got {status}: {body.get('error')!r}"


@pytest.mark.parametrize("text", ["recall@0 zebras depth:1", "recall@0 zebras depth:2"])
def test_depth_without_an_anchor_runs_but_warns(query, text):
    """A depth above the floor on a recall naming no anchor runs, and the
    response says the clause had no effect.

    The graph is entered only through a topic:/entity: the recall names, so
    with none it is a text search whatever its depth. Honouring the clause in
    silence would tell an agent transmission happened when it did not, so the
    warning names the clause and what would give it effect.
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

    The column is the only thing in the response that says *where* the query
    went wrong, and a client's caret is drawn from it. Taken from the lexer's
    read cursor, which runs a token ahead of the parser, it would point an
    error quoting `food` at the end of the token after it. Every case here
    keeps a token to the right of the bad one, because that is the only
    arrangement in which the two positions differ.
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
    A call site re-wrapping them with a bad %e verb would turn a clean
    'invalid since value "soon"' into `&{%!e(string=...)}` in the 400 body,
    garbling the message an agent needs to self-correct.
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
# {"count":0,"hits":[]}. One shared shape would make a stored fact
# byte-identical to a failed search, and give a caller who had never written
# to a graph the same answer as one whose query missed. These pin the three
# apart; the Python SDK and the MCP bridge both parse against them.


def test_an_accepted_write_is_acknowledged_not_answered_with_results(query):
    """A write comes back 200 with its own acknowledgement and no result set.

    The "status" key is part of the contract: its presence is what
    distinguishes an accepted write from a recall that matched nothing.
    """
    status, body = query("remember@1 'ackprobe is a loose remember'")

    assert status == 200, body.get("error")
    assert body == {"status": "ok"}


def test_a_recall_of_an_empty_graph_is_204_with_no_body(query):
    """Graph 8 is never written to, so a recall of it answers 204.

    No body, because 204 must not carry one — and none is needed: the whole
    answer is "nothing has been stored here", which no result set could say
    that an ordinary miss would not also say.
    """
    status, body = query("recall@8 zebras")

    assert status == 204, body.get("error")
    assert body == {}


def test_a_recall_that_matched_nothing_on_a_populated_graph_stays_200(query):
    """A populated graph that matched nothing keeps the empty result envelope.

    This is the distinction the 204 exists for. The graph is seeded here
    rather than assumed populated, because files run in any order and a graph
    that happened to be empty would answer 204 and pass the wrong assertion.
    """
    status, body = query("remember@1 'the barometer falls before the storm'")
    assert status == 200, body.get("error")

    status, body = query("recall@1 zzznomatchzzz")

    assert status == 200, body.get("error")
    assert body["results"]["count"] == 0
    assert body["results"]["hits"] == []
