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


def test_query_rejects_out_of_range_graph(query):
    """A selector past the allocated graph range is a fast client error, not a
    hang. Graph 9 is above the eight graphs the store allocates.
    """
    status, body = query("recall@9 anything")

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
    for q in ("remember@256 'secret plan' topic:x", "recall@300 anything"):
        status, body = query(q)

        assert status == 400, f"{q!r}: expected 400, got {status}"
        assert "out of range" in body.get("error", ""), (
            f"{q!r}: expected the parser's out-of-range error, got {body.get('error')!r}"
        )


def test_query_rejects_non_integer_selector(query):
    """A non-numeric selector is a parse error, not a silent fallback to a
    default graph.
    """
    status, body = query("recall@abc anything")

    assert status == 400
    assert body.get("error"), "expected a parse error message"


def test_query_rejects_valid_uint8_selector_above_num_graphs(query):
    """@255 fits the selector type, so the parser passes it — the handler must
    then reject it against the allocated graph count with its own distinct
    error. This is the layer boundary: type consistency in the parser,
    allocation policy in the handler.
    """
    status, body = query("recall@255 anything")

    assert status == 400
    assert "does not exist" in body.get("error", ""), (
        f"expected the handler's does-not-exist error, got {body.get('error')!r}"
    )


def test_wrapping_selector_write_does_not_leak_to_graph_zero(query):
    """Regression guard for the wrap itself: a rejected remember@256 must leave
    no trace on graph 0 (the graph @256 used to wrap to). Read-only on graph 0
    apart from the probe recall, so it does not disturb that graph's facts.
    """
    status, _ = query("remember@256 'wrapprobe should never land' topic:wrapprobe")
    assert status == 400

    status, body = query("recall@0 wrapprobe")
    assert status == 200
    assert body["results"]["count"] == 0, (
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


# A keyed field is `key:value`. The parser used to advance past the separator
# without checking it was a ':', so a missing colon did not fail — it shifted
# every following token one role to the left. "recall x since 7d 30d" answered
# with a 30d bound and no error at all, and "recall x topic food extra" filtered
# by nothing. The tests below cover the whole keyed-field family together,
# because the bug was per-helper: parseTop and parseDepth checked the separator,
# parseAnchorField and parseTimeValue did not, and nothing pinned the contract
# across all of them.


@pytest.mark.parametrize(
    "text",
    [
        "recall zebras topic food",
        "recall zebras topic food extra",  # the ticket repro
        "recall zebras entity alice",
        "recall zebras since 7d",
        "recall zebras until 2026-01-15",
        "recall zebras top 5",
        "recall zebras depth 2",
        "recall zebras topic:food entity alice",  # one good field, one bad
        "remember@5 'ulysse moved to quimper' topic relocation",
        "remember@5 'ulysse moved to quimper' entity ulysse",
    ],
)
def test_query_rejects_missing_field_separator(query, text):
    """A keyed field written without its ':' is a 400 naming the separator.

    Anything else is the silent-parse failure mode: the query runs, returns
    200, and answers a question nobody asked.
    """
    status, body = query(text)

    assert status == 400, f"{text!r} should be rejected, got {status}: {body}"
    assert "colon" in body.get("error", "").lower(), (
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
    """The shapes that used to parse clean, and are the reason this is a bug and
    not a typo.

    With the separator skipped, `since 7d 30d` consumed `7d` as the separator
    and took `30d` as the bound: a 200 carrying results scoped four times wider
    than asked for. There is no signal an agent could use to notice that, which
    is why these must be rejected rather than best-effort interpreted.
    """
    status, body = query(text)

    assert status == 400, (
        f"{text!r} parsed instead of failing — the token shift is back, and the "
        f"result is scoped by the wrong bound: {body}"
    )


def test_missing_separator_write_does_not_land(query):
    """A rejected remember must not have written anything.

    The 400 covers the parse; this covers execution. A missing colon on a write
    used to mis-assign the anchors rather than fail, so the fact was committed —
    under the wrong topic, where no later recall would find it.
    """
    status, _ = query("remember@5 'colonprobe should never land' topic colonprobe")
    assert status == 400

    status, body = query("recall@5 colonprobe")
    assert status == 200, body.get("error")
    assert body["results"]["count"] == 0, (
        "a rejected write landed on graph 5 — the parse error did not stop execution"
    )


# A reserved word is only syntax where a clause can start. In value position —
# the right-hand side of a field's ':', or the leading term a recall must
# begin with — it reads as an ordinary word, so an entity that happens to be
# called "top" needs no quoting. Two rules hold that line: a keyword
# immediately followed by ':' is always a field, and a keyword is lower-case
# only — written with any upper case where a clause could start, it is an
# error naming the casing, never a term. One ambiguity survives — a leading
# term that spells a keyword is legal data but one ':' from a clause — and it
# is answered with a warning beside the results rather than a guess. The tests
# below pin every side: the shapes that now parse, the keyword-colon shapes
# that must stay rejected, the mis-cased shapes that must never be silently
# swallowed as data, and the warning that covers the ambiguity no rule can
# close.


@pytest.mark.parametrize(
    "text",
    [
        "recall@0 top",  # keyword as the leading term: a search for the word "top"
        "recall@0 Top",  # upper case is legal in data position, and folds to the same word
        "recall@0 top top:3",  # same spelling as term and clause, told apart by the ':'
        "recall@0 shelf entity:top",  # the bug-report repro, on the read side
        "recall@0 shelf topic:top",
        "recall@0 shelf entity:Top",  # an anchor value may carry any casing, keyword spelling or not
        "recall@0 shelf entity:since topic:recall",  # every keyword, not just "top"
    ],
)
def test_query_accepts_keyword_in_value_position(query, text):
    """A keyword on the right of a field's ':', or as the leading recall term,
    is data — the query parses and runs.

    `remember 'x' entity:top` used to die with `parse error ... expected a
    word or quoted phrase, but found "top"`, because the parser typed "top"
    by spelling alone. An LLM extracting entities from prose will eventually
    emit exactly that word bare ("she reached the top"), so one unlucky
    extraction killed a whole ingestion run with a 400 the client could not
    anticipate. These probes are recalls of the same shapes, chosen because
    they are read-only: acceptance is proven without writing anything.
    """
    status, body = query(text)

    assert status == 200, f"{text!r} should parse, got {status}: {body.get('error')!r}"


@pytest.mark.parametrize(
    "text",
    [
        "recall@0 top:3",  # keyword+':' is the top clause — and a recall still needs a term first
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
        "recall zebras Since 7d",  # parsed clean as a three-term search before the check
        "recall zebras Since 7d 30d",  # the shifted time-bound shape, through the casing door
        "recall zebras TOP:3",  # mis-cased clause: rejected for its casing, not its stray ':'
        "recall zebras Topic:food",
        "remember@5 'a caseprobe fact' Entity:bob",  # mis-cased field on the write side
    ],
)
def test_query_rejects_miscased_keyword(query, text):
    """A keyword written with any upper case is a 400 wherever it would read
    as syntax — casing does not un-reserve a word.

    Keywords are lower-case syntax; upper case is only legal where a token is
    unambiguously data (a term, a phrase, an anchor value). The dangerous
    shapes are the middle two: case folding of terms would happily read
    `recall zebras Since 7d` as a three-term search — a 200 scoped by nothing,
    with no signal to correct from — reviving the silent-shift family above
    through the casing door.
    """
    status, body = query(text)

    assert status == 400, f"{text!r} should be rejected, got {status}: {body}"
    assert body.get("error"), f"expected a parse error message for {text!r}"


def test_miscased_keyword_error_names_the_casing(query):
    """The 400 for a mis-cased clause keyword tells the agent what is wrong
    and how to get the word instead: lower-case the keyword, or quote the
    word. Without the hint, `Since` blamed a stray ':' or nothing at all,
    and the one thing the error must enable is self-correction.
    """
    status, body = query("recall zebras Since 7d")

    assert status == 400
    error = body.get("error", "")
    assert "lower case" in error, f"want the casing named, got {error!r}"
    assert "'Since'" in error, f"want the quoted escape shown, got {error!r}"


def test_leading_keyword_term_warns_but_runs(query):
    """`recall since 7d` runs as a two-term search and the response carries a
    warning naming the clause it nearly is.

    The leading term is the one position where a bare keyword legally reads
    as a word — which makes it the one position where a mistyped clause can
    slip through as data: `recall since 7d` is one ':' away from
    `recall since:7d`, and the two answer differently-scoped questions. The
    server cannot know which was meant, so it answers the query as written
    and says what else it could have meant, naming the clause syntax and the
    quoting escape so an agent can resolve the ambiguity from the response
    alone. Neither erroring (which would reject legitimate one-word searches
    like `recall top`) nor staying silent (a typo with no signal) is
    acceptable here.
    """
    status, body = query("recall@0 since 7d")

    assert status == 200, body.get("error")
    warnings = body.get("warnings", [])
    assert len(warnings) == 1, f"want exactly one warning, got {warnings}"
    assert "since:<value>" in warnings[0], warnings[0]
    assert "('since')" in warnings[0], warnings[0]


@pytest.mark.parametrize(
    "text",
    [
        "recall@0 zebras",  # nothing keyword-shaped anywhere
        "recall@0 'since' 7d",  # quoting the term states the intent
        "recall@0 zebras entity:top",  # anchor values are unambiguous, never warned about
        "recall@0 zebras since:7d",  # an actual clause is what it says it is
    ],
)
def test_unambiguous_query_carries_no_warnings_key(query, text):
    """A query with nothing to warn about has no warnings key at all.

    The key appears only when there is something to say, so the response
    shape for the common case is unchanged and a client checking
    `"warnings" in body` gets a real signal, not a constant empty list.
    Quoting is the documented way to silence the leading-term warning, so
    the quoted shape must genuinely be silent.
    """
    status, body = query(text)

    assert status == 200, body.get("error")
    assert "warnings" not in body, (
        f"{text!r} should be warning-free, got {body['warnings']}"
    )


@pytest.mark.parametrize(
    ("text", "blame"),
    [
        ("recall zebras topic food extra", "food"),
        ("recall zebras entity alice extra", "alice"),
        ("recall zebras since 7d 30d", "7d"),
        ("recall zebras top 5 depth:2", "5"),
        ("recall zebras depth 2 top:3", "2"),
        ("remember@5 'a colonprobe fact' topic food entity:x", "food"),
        ("recall zebras since:soon top:3", "soon"),
        ("recall zebras depth:abc top:3", "abc"),
        ("recall@abc zebras top:3", "abc"),
    ],
)
def test_parse_error_column_points_at_the_offending_token(query, text, blame):
    """The 400 must point at the last character of the token it quotes.

    The column is the only thing in the response that says *where* the query
    went wrong, and a client's caret is drawn from it. It used to be taken from
    the lexer's read cursor, which runs a token ahead of the parser, so an
    error quoting `food` pointed at the end of the token after it — every case
    here keeps a token to the right of the bad one, because that is the only
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
    The parser's call sites used to re-wrap them with a bad %e verb, turning a
    clean 'invalid since value "soon"' into `&{%!e(string=...)}` in the 400
    body — garbling, at the last step, exactly the message an agent needs to
    self-correct.
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
