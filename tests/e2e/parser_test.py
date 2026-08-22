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

"""Adversarial parser surface: 217 hostile query strings over raw HTTP.

This file is written as a specification, not a regression net — most of it
fails against the current parser on purpose. Each case pins the *error message
a caller needs* rather than the one the parser happens to emit, because the
caller is an agent: it can only repair a query the error tells it how to
repair. `Expected colon, but found ""` is a dead end; `quote it ('top') to
search for the word` is a repair instruction.

Three properties are pinned throughout:

1. A bad query is a 400 with a non-empty, *actionable* message — never a 500,
   never a hang, never a silent 200 answering a different question.
2. A query that only *looks* dangerous (specials inside quotes, reserved words
   in value position) is data and must succeed.
3. A query that is ambiguous between two valid readings runs and warns, rather
   than guessing silently.

Writes are avoided wherever a recall proves the same point, so this file adds
no facts to the graph map in conftest.py. The handful of remembers that must
succeed are pinned to graph 1 (loose remembers) and are idempotent — a fact is
keyed by its value.
"""

import pytest


def _reject(status, body, expected, query_text):
    """Assert a 400 whose message contains `expected` (case-insensitively)."""
    assert status == 400, f"{query_text!r}: expected 400, got {status} — body {body!r}"
    message = (body.get("error") or "").lower()
    assert message, f"{query_text!r}: 400 with an empty error message"
    assert expected.lower() in message, (
        f"{query_text!r}: error {body.get('error')!r} should mention {expected!r}"
    )


def _accept(status, body, query_text):
    """Assert a query the parser must treat as valid."""
    assert status == 200, (
        f"{query_text!r}: expected 200, got {status} — {body.get('error')!r}"
    )


# ---------------------------------------------------------------------------
# Duplicate single-valued clauses. Each is a bare assignment in the clause
# switch (r.depth = ...), so the last occurrence silently wins. Repeated
# anchors are a different case and stay legal — they are a list by design.
# ---------------------------------------------------------------------------


@pytest.mark.parametrize(
    "text",
    [
        "recall ferry depth:2 depth:5",
        "recall ferry depth:1 depth:2 depth:3",
        "recall ferry top:3 top:10",
        "recall ferry top:1 top:2 top:3",
        "recall ferry since:7d since:30d",
        "recall ferry until:1d until:2d",
        "recall ferry since:7d until:30d since:1d",
        "recall ferry vec:$a vec:$b",
        "recall ferry depth:2 top:3 depth:9",
        "recall ferry top:5 since:7d top:9",
        "recall@1 ferry top:1 top:2",
        "recall ferry topic:harbour depth:1 depth:4",
        "remember@1 'the ferry docks at dawn' vec:$a vec:$b",
        "recall ferry depth:2 DEPTH:5",
    ],
)
def test_duplicate_single_valued_clause_is_rejected(query, text):
    """A repeated modifier is an agent generation bug, and last-wins hides it.

    parseTimeValue's own comment argues the case: a query that silently answers
    a differently-scoped question is worse than an error the agent can correct
    from. The message must name the duplicated clause so the agent knows which
    one to drop.
    """
    status, body = query(text)
    _reject(status, body, "duplicate", text)


# ---------------------------------------------------------------------------
# Bounds. The graph selector is correctly clamped to the uint8 range; depth and
# top get strconv.Atoi and nothing else, so one string can request a
# million-hop traversal or a two-billion-entry heap.
# ---------------------------------------------------------------------------


@pytest.mark.parametrize(
    "text",
    [
        "recall ferry depth:65",
        "recall ferry depth:100",
        "recall ferry depth:1000",
        "recall ferry depth:1000000",
        "recall ferry depth:2147483647",
        "recall ferry depth:99999999999999999999",
        "recall ferry top:10001",
        "recall ferry top:100000",
        "recall ferry top:2000000000",
        "recall ferry top:2147483648",
        "recall ferry top:4294967296",
        "recall ferry top:99999999999999999999",
        "recall ferry depth:1000000 top:1000000",
        "recall@1 ferry depth:500",
    ],
)
def test_depth_and_top_are_bounded(query, text):
    """An unbounded traversal or result size is a denial of service from a
    single string, so both take a parse-time ceiling like the selector does.

    The message must say the value is out of range and what the range is —
    an agent that only learns "invalid" will retry with another huge number.
    """
    status, body = query(text)
    _reject(status, body, "out of range", text)


@pytest.mark.parametrize(
    "text",
    [
        "recall ferry depth:0",
        "recall ferry depth:1",
        "recall ferry depth:2",
        "recall ferry top:1",
        "recall ferry top:10",
        "recall ferry top:100",
        "recall ferry depth:2 top:10",
        "recall@1 ferry depth:1 top:5",
    ],
)
def test_ordinary_depth_and_top_still_parse(query, text):
    """The ceiling must not eat the ordinary range — depth 0 included, which
    the Go suite already pins as meaningful (an explicit no-traversal recall).
    """
    status, body = query(text)
    _accept(status, body, text)


# ---------------------------------------------------------------------------
# Empty data. A quoted empty string currently sails through as a real fact,
# term or anchor identity.
# ---------------------------------------------------------------------------


@pytest.mark.parametrize(
    "text",
    [
        "remember@1 ''",
        "remember@1 '' topic:harbour",
        "remember@1 '   '",
        "recall ''",
        "recall '   '",
        "recall ferry topic:''",
        "recall ferry entity:''",
        "recall ferry topic:'' entity:''",
    ],
)
def test_empty_data_is_rejected(query, text):
    """An empty fact is unretrievable and an empty anchor is an identity no
    caller can name again, so both are storage-corrupting no-ops.

    Whitespace-only is the same case: it survives folding and produces an
    anchor nobody can type twice.
    """
    status, body = query(text)
    _reject(status, body, "empty", text)


# ---------------------------------------------------------------------------
# Reserved words by position. Leading position warns (both readings named),
# value position is data, trailing position errors — and the trailing error is
# where the quality collapses: the mis-cased branch produces an excellent
# message while the lower-case one produces "Expected colon, but found """.
# ---------------------------------------------------------------------------


@pytest.mark.parametrize(
    "text",
    [
        "recall top",
        "recall since",
        "recall until",
        "recall depth",
        "recall vec",
        "recall topic",
        "recall entity",
        "recall forget",
        "recall update",
        "recall remember",
        "recall recall",
    ],
)
def test_leading_reserved_word_runs_and_warns(query, text):
    """A leading keyword is legal data one ':' away from being a clause, so it
    runs as a term search and carries a warning naming both readings.

    Pinned here because the warning is the only signal that separates "I meant
    the word" from "I forgot the colon" — losing it makes a whole class of
    mistyped queries silent.
    """
    status, body = query(text)
    _accept(status, body, text)
    warnings = body.get("warnings") or []
    assert warnings, f"{text!r}: expected a keyword-ambiguity warning, got none"
    joined = " ".join(str(w) for w in warnings).lower()
    assert "keyword" in joined, (
        f"{text!r}: warning {warnings!r} should name the keyword ambiguity"
    )


@pytest.mark.parametrize(
    "text",
    [
        "recall ferry top",
        "recall ferry since",
        "recall ferry until",
        "recall ferry depth",
        "recall ferry topic",
        "recall ferry entity",
        "recall ferry vec",
        "recall ferry bridge top",
        "recall ferry bridge since",
        "recall@1 ferry top",
        "recall ferry topic:harbour top",
    ],
)
def test_trailing_reserved_word_error_is_actionable(query, text):
    """The same mistake gets two wildly different messages today.

    "recall ferry Top" produces "mis-cased keyword ... quote it ('Top') to
    search for the word" — a repair instruction. "recall ferry top" produces
    "Expected colon, but found \"\"", which reports end-of-input as an empty
    literal and offers nothing. Both should route through the first message.
    """
    status, body = query(text)
    assert status == 400, f"{text!r}: expected 400, got {status} — {body!r}"
    message = body.get("error") or ""
    assert 'found ""' not in message, (
        f"{text!r}: error {message!r} reports end-of-input as an empty literal; "
        "say 'end of input'"
    )
    assert "quote it" in message.lower(), (
        f"{text!r}: error {message!r} should tell the caller to quote the "
        "keyword, the way the mis-cased branch does"
    )


@pytest.mark.parametrize(
    "text",
    [
        "recall ferry topic:top",
        "recall ferry topic:since",
        "recall ferry topic:depth",
        "recall ferry topic:recall",
        "recall ferry topic:remember",
        "recall ferry entity:top",
        "recall ferry entity:since",
        "recall ferry entity:vec",
        "recall ferry entity:forget",
        "recall ferry topic:top entity:since",
        "recall@1 ferry topic:top",
    ],
)
def test_reserved_word_in_value_position_is_data(query, text):
    """Spelling alone must not make a word syntax: a stored anchor that happens
    to be called "top" has to be nameable without quoting.

    This is the boundary the Go suite calls KeywordAsValue; it is pinned again
    here because it is what a stricter duplicate/bounds rule is most likely to
    break by accident.
    """
    status, body = query(text)
    _accept(status, body, text)


@pytest.mark.parametrize(
    "text,expected",
    [
        ("recall ferry since:top", "invalid since value"),
        ("recall ferry since:entity", "invalid since value"),
        ("recall ferry until:depth", "invalid until value"),
        ("recall ferry until:recall", "invalid until value"),
        ("recall ferry depth:top", "invalid depth value"),
        ("recall ferry depth:since", "invalid depth value"),
        ("recall ferry top:top", "invalid top value"),
        ("recall ferry top:vec", "invalid top value"),
    ],
)
def test_reserved_word_where_a_value_is_required_names_the_clause(
    query, text, expected
):
    """A keyword in a numeric or temporal slot is a value error, not a grammar
    error, so the message names the clause and the value it could not read.

    "Expected literal, but found \"top\"" is wrong twice: "top" *is* a literal
    to the caller, and the clause that rejected it goes unnamed.
    """
    status, body = query(text)
    _reject(status, body, expected, text)


# ---------------------------------------------------------------------------
# The vec: clause. parseVecField produces precise positioned errors and both
# call sites throw them away for a generic wrap — the exact mangling
# TestClauseErrorsSurfaceUnmangled forbids for every other clause, and the one
# clause that test does not cover.
# ---------------------------------------------------------------------------


@pytest.mark.parametrize(
    "text,expected",
    [
        ("recall ferry vec:v", "param field operator $"),
        ("recall ferry vec:query", "param field operator $"),
        ("recall ferry vec:$", "expected literal"),
        ("recall ferry vec$:v", "expected colon"),
        ("recall ferry vec:$$", "expected literal"),
        ("recall ferry vec:$v extra", "unexpected"),
        ("remember@1 'the ferry docks at dawn' vec:v", "param field operator $"),
        ("remember@1 'the ferry docks at dawn' vec:$", "expected literal"),
    ],
)
def test_vec_clause_errors_surface_unmangled(query, text, expected):
    """vec: is the one clause whose inner error is discarded by its caller.

    The Go suite already forbids the string "Error while parsing" in any parse
    error — it just never exercises vec. Both call sites should `return nil,
    err` like every other clause does.
    """
    status, body = query(text)
    message = body.get("error") or ""
    assert "error while parsing" not in message.lower(), (
        f"{text!r}: error {message!r} is the generic wrap; return the inner "
        "positioned error unchanged"
    )
    _reject(status, body, expected, text)


# ---------------------------------------------------------------------------
# Specials inside quotes. Everything between '...' is data; only a doubled
# quote is an escape. These must all succeed — a memory system that cannot
# store a fact containing a colon or a plus sign cannot store real sentences.
# ---------------------------------------------------------------------------


@pytest.mark.parametrize(
    "text",
    [
        "recall 'topic:billing'",
        "recall 'entity:acme'",
        "recall 'since:7d until:30d'",
        "recall 'star * asterisk'",
        "recall '@3'",
        "recall '$param'",
        "recall '(parenthesised)'",
        "recall ')unbalanced('",
        "recall 'colon: and more'",
        "recall 'double::colon'",
        "recall 'it''s escaped'",
        "recall ''''",  # a single escaped quote is a one-character term
        "recall 'recall remember forget update'",
        "recall 'depth:2 top:5 vec:$v'",
        "recall 'semi;colon pipe|bar brace{}'",
        "recall 'quote\"double'",
        "recall 'back`tick'",
        "recall 'emoji and accents'",
        "recall 'trailing space '",
        "recall ' leading space'",
        "recall 'MiXeD CaSe'",
        "recall 'hyphen-ated under_scored'",
    ],
)
def test_specials_inside_quotes_are_data(query, text):
    """A quoted phrase is opaque: reserved words and symbols inside it carry no
    meaning, so none of these is a grammar error.

    This is the property that lets real sentences be stored verbatim, and the
    one most at risk from a stricter prefix or duplicate rule that forgets to
    stop at the quote.
    """
    status, body = query(text)
    _accept(status, body, text)


@pytest.mark.parametrize(
    "text",
    [
        "remember@1 'the ferry docks at dawn'",
        "remember@1 'billing: the invoice was paid' topic:harbour",
        "remember@1 'acme & globex signed 50/50' topic:harbour",
        "remember@1 'it''s a quoted fact' topic:harbour",
        "remember@1 'a fact with depth:2 inside' topic:harbour",
        "remember@1 'a fact naming topic:harbour inside' entity:acme",
    ],
)
def test_specials_inside_a_remembered_fact_are_stored(query, text):
    """The write path has to be as opaque as the read path.

    Pinned to graph 1 (loose remembers) and idempotent — a fact is keyed by its
    value, so reruns against a long-lived server change nothing.
    """
    status, body = query(text)
    _accept(status, body, text)


@pytest.mark.parametrize(
    "text",
    [
        "recall 'unterminated",
        "recall 'unterminated with topic:x",
        "remember@1 'unterminated",
        "remember@1 'escaped then end''",
        "recall '",
        "remember@1 '",
        "recall ferry topic:'unterminated",
        "recall ferry 'one' 'two",
    ],
)
def test_unterminated_phrase_is_reported_as_such(query, text):
    """An unterminated phrase is the one quote error there is, and it already
    reports well — pinned so a stricter phrase rule cannot regress it into a
    generic token error.
    """
    status, body = query(text)
    _reject(status, body, "unterminated", text)


# ---------------------------------------------------------------------------
# Temporal values. The message already names both accepted forms; these pin
# that it keeps doing so across the plausible ways an agent gets it wrong.
# ---------------------------------------------------------------------------


@pytest.mark.parametrize(
    "text,expected",
    [
        ("recall ferry since:soon", "invalid since value"),
        ("recall ferry since:yesterday", "invalid since value"),
        ("recall ferry since:7", "invalid since value"),
        ("recall ferry since:d", "invalid since value"),
        ("recall ferry since:7dd", "invalid since value"),
        ("recall ferry since:2026-13-45", "invalid since value"),
        ("recall ferry since:2026-02-30", "invalid since value"),
        ("recall ferry since:15-01-2026", "invalid since value"),
        ("recall ferry since:2026/01/15", "invalid since value"),
        ("recall ferry until:later", "invalid until value"),
        ("recall ferry until:tomorrow", "invalid until value"),
        ("recall ferry until:0x10", "invalid until value"),
        ("recall ferry until:7 d", "invalid until value"),
        ("recall ferry until:--7d", "invalid until value"),
        ("recall ferry since:99999999999999999999d", "invalid since value"),
        ("recall ferry since:", "expected"),
    ],
)
def test_invalid_temporal_values_name_the_accepted_forms(query, text, expected):
    """A temporal error has to teach the grammar, since duration-vs-date is
    exactly what an agent guesses wrong.

    The existing message ("expected a duration like 7d or a date like
    2026-01-15") is the model for every other value error in this file.
    """
    status, body = query(text)
    _reject(status, body, expected, text)


@pytest.mark.parametrize(
    "text",
    [
        "recall ferry since:7d",
        "recall ferry since:0d",
        "recall ferry until:30d",
        "recall ferry since:2026-01-15",
        "recall ferry until:2026-12-31",
        "recall ferry since:7d until:30d",
        "recall ferry since:2026-01-15 until:2026-12-31",
        "recall@1 ferry since:7d top:5",
    ],
)
def test_valid_temporal_values_parse(query, text):
    """Both accepted forms, at both bounds, including the zero duration — the
    guard rail for any stricter temporal validation.
    """
    status, body = query(text)
    _accept(status, body, text)


# ---------------------------------------------------------------------------
# Graph selector. The best-messaged part of the parser; pinned so it stays that
# way, and extended to the cases that currently fall back to generic errors.
# ---------------------------------------------------------------------------


@pytest.mark.parametrize(
    "text,expected",
    [
        ("recall@256 ferry", "out of range"),
        ("recall@300 ferry", "out of range"),
        ("recall@99999 ferry", "out of range"),
        ("remember@256 'the ferry docks at dawn'", "out of range"),
        ("recall@3.5 ferry", "whole number"),
        ("recall@abc ferry", "whole number"),
        ("recall@ ferry", "whole number"),
        ("recall@topic ferry", "whole number"),
        ("recall@3@5 ferry", "unexpected"),
        ("recall@-1 ferry", "whole number"),
    ],
)
def test_graph_selector_errors_stay_specific(query, text, expected):
    """Selector validation is layered — parser rejects what cannot fit uint8,
    handler rejects what fits but names no allocated graph — and both layers
    must keep their own message so a regression in one cannot hide behind the
    other.
    """
    status, body = query(text)
    _reject(status, body, expected, text)


# ---------------------------------------------------------------------------
# Structure: grouping, multiple commands, newlines. All currently land in the
# generic default branch, which tells an agent nothing about which rule it hit.
# ---------------------------------------------------------------------------


@pytest.mark.parametrize(
    "text",
    [
        "recall (ferry)",
        "recall ferry (bridge)",
        "recall (ferry or bridge)",
        "recall ferry topic:(harbour)",
        "recall ((ferry))",
        "recall ferry )bridge(",
    ],
)
def test_grouping_is_rejected_as_unsupported(query, text):
    """Parentheses lex to LPAREN/RPAREN and no rule accepts them, so the error
    should say grouping is unsupported rather than name the character.

    If grouping is never coming, the tokens should stop being emitted; either
    way "Encountered unexpected token: \"(\"" is the wrong answer.
    """
    status, body = query(text)
    assert status == 400, f"{text!r}: expected 400, got {status} — {body!r}"
    message = (body.get("error") or "").lower()
    assert "group" in message or "not supported" in message, (
        f"{text!r}: error {body.get('error')!r} should say grouping is "
        "unsupported, not blame the character"
    )


@pytest.mark.parametrize(
    "text",
    [
        "recall ferry recall bridge",
        "recall ferry remember 'x'",
        "remember@1 'a' remember@1 'b'",
        "recall ferry; recall bridge",
        "recall ferry\nrecall bridge",
        "recall ferry\nbridge",
    ],
)
def test_second_command_is_rejected_as_one_per_instruction(query, text):
    """FQL is one command per instruction, and the newline cases are the
    dangerous ones: isBlank() swallows \\n, so "recall ferry\\nbridge" silently
    becomes a two-term recall instead of an error.

    The NEWLINE token exists in token.go and is never emitted — that is the gap
    this pins.
    """
    status, body = query(text)
    assert status == 400, (
        f"{text!r}: expected 400, got {status} — a second command must not be "
        f"folded into the first ({body!r})"
    )
    message = (body.get("error") or "").lower()
    assert "one command" in message or "end of query" in message, (
        f"{text!r}: error {body.get('error')!r} should say one command per instruction"
    )


# ---------------------------------------------------------------------------
# Anchor-seeded recall. Anchors are seeds, not filters, so "everything about
# billing" is a natural query — and it is currently unreachable, because
# parseRecall demands a term before any clause.
# ---------------------------------------------------------------------------


@pytest.mark.parametrize(
    "text",
    [
        "recall topic:harbour",
        "recall entity:acme",
        "recall topic:harbour entity:acme",
        "recall@1 topic:harbour",
        "recall topic:harbour top:5",
        "recall topic:harbour since:7d",
        "recall topic:harbour depth:2 top:3",
        "recall entity:acme until:30d",
    ],
)
def test_anchor_only_recall_is_reachable(query, text):
    """An anchor is a seed, so a recall with no text term is a well-formed
    question: expand from this anchor and rank what you reach.

    If a text term is genuinely required, this test should be replaced by one
    pinning a message that *says* so — "expected a word or quoted phrase, but
    found \"topic\"" reads like the anchor itself was malformed.
    """
    status, body = query(text)
    _accept(status, body, text)


# ---------------------------------------------------------------------------
# Degenerate and pathological input. The floor: never a 500, never an empty
# message, never a hang.
# ---------------------------------------------------------------------------


@pytest.mark.parametrize(
    "text",
    [
        "",
        " ",
        "\t",
        "\n",
        "   \t \r\n  ",
        ":",
        "::",
        ":::",
        "@",
        "@@@",
        "$",
        "$$",
        "(",
        ")",
        "()",
        "bogus nonsense",
        "recall",
        "remember",
        "recall@",
    ],
)
def test_degenerate_input_is_a_clean_client_error(query, text):
    """Every unparsable string is a 400 carrying a non-empty message.

    The empty query is called out separately below because "expected a command
    (recall, remember), found \"\"" describes an empty *token*, not an empty
    query.
    """
    status, body = query(text)
    assert status == 400, f"{text!r}: expected 400, got {status} — {body!r}"
    assert body.get("error"), f"{text!r}: 400 with no error message"


@pytest.mark.parametrize(
    "text",
    [
        "recall " + "a" * 10000,
        "recall '" + "b" * 10000 + "'",
        "recall " + "ferry " * 2000,
        "remember@1 '" + "c" * 10000 + "'",
        "recall ferry" + " depth:1" * 500,
        "recall " + "topic:harbour " * 500,
    ],
)
def test_pathological_length_is_bounded_not_fatal(query, text):
    """A very long query is answered or rejected, never a 500 and never a hang.

    A parse-time length ceiling is a reasonable answer here — this pins only
    that the server stays a well-behaved HTTP service either way.
    """
    status, body = query(text)
    assert status in (200, 400, 413), (
        f"{text[:40]!r}...: expected 200/400/413, got {status} — {body!r}"
    )
    if status != 200:
        assert body.get("error"), "a rejection must carry a message"
