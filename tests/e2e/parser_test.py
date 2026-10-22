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

"""Adversarial parser surface: hostile query strings over raw HTTP.

Each case pins the *error message a caller needs*, not whatever the parser
happens to emit. The caller is an agent, and it can only repair a query the
error tells it how to repair. `Expected colon, but found ""` is a dead end.
`quote it ('top') to search for the word` is a repair instruction.

Three properties are pinned throughout:

1. A bad query is a 400 with a non-empty, *actionable* message. It is never
   a 500, never a hang, and never a silent 200 answering a different
   question.
2. A query that only *looks* dangerous is data and must succeed: specials
   inside quotes, or reserved words in value position.
3. A query that runs despite something it should not carry, such as a stop
   word among its terms, says so in a warning.

Writes are avoided wherever a recall proves the same point. The few remembers
that must succeed are pinned to graph 1 (loose remembers). They are idempotent,
since a fact is keyed by its value.
"""

import pytest

# Duplicate single-valued clauses. The clause switch assigns each clause
# (r.depth = ...). A bare assignment would let the last occurrence win
# silently, so a repeat is rejected. Repeated anchors are a different case and
# stay legal, because anchors are a list by design.


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
def test_duplicate_single_valued_clause_is_rejected(query, text, assert_rejected):
    """A repeated modifier is an agent generation bug, and last-wins hides it.

    A query that silently answers a differently-scoped question is worse than
    an error the agent can correct from. The message must name the duplicated
    clause so the agent knows which one to drop.
    """
    status, body = query(text)
    assert_rejected(status, body, "duplicate", text)


# Bounds. The graph selector must fit the uint8 range. depth and top each take
# a ceiling (db.max-depth, db.max-top). No lane exists past depth 2. Without a
# ceiling on top, a single string could request a two-billion-entry heap.


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
def test_depth_and_top_are_bounded(query, text, assert_rejected):
    """depth and top take a parse-time ceiling, like the selector does.

    An unbounded result size is a denial of service from a single string, and
    no lane exists past depth 2. The message must say the value is out of range
    and what the range is. An agent that only learns "invalid" will retry with
    another huge number.
    """
    status, body = query(text)
    assert_rejected(status, body, "out of range", text)


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
def test_ordinary_depth_and_top_still_parse(query, text, assert_accepted):
    """The ceiling must not eat the ordinary range, depth 0 included.

    The Go suite pins depth 0 as meaningful: an explicit recall with no
    traversal.
    """
    status, body = query(text)
    assert_accepted(status, body, text)


# Empty data. A quoted empty string is rejected as a fact, term or anchor
# identity.


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
def test_empty_data_is_rejected(query, text, assert_rejected):
    """Empty facts and anchors are rejected: they would corrupt storage.

    An empty fact cannot be retrieved. An empty anchor is an identity no caller
    can name again. Whitespace-only is the same case. It survives folding and
    makes an anchor nobody can type twice.
    """
    status, body = query(text)
    assert_rejected(status, body, "empty", text)


# Reserved words by position. A reserved word is syntax wherever a value cannot
# stand. As a bare term it is a 400 naming both fixes: the clause, and the
# quote that searches for the word. After a clause has started, it is a clause
# missing its ':'. After a field's ':', it is data.


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
    ],
)
def test_leading_reserved_word_is_rejected(query, text, assert_rejected):
    """A leading keyword is a 400, like one anywhere else among the terms.

    Terms and phrases may come in any order, so no position makes a bare
    keyword data. Data goes in quotes.
    """
    status, body = query(text)
    assert_rejected(
        status, body, f'term "{text.split(" ")[1]}" is also a keyword:', text
    )


@pytest.mark.parametrize(
    "text",
    [
        "recall forget",
        "recall update",
        "recall remember",
        "recall recall",
    ],
)
def test_leading_command_word_is_rejected_with_the_quote(query, text, assert_rejected):
    """A leading command word is a 400 naming the quote.

    A command word has no clause reading, because recall:<value> is itself an
    error. A message offering it would send the caller from one rejection to
    the next. So the message offers only what works, quoting the word. It never
    suggests a clause spelled with a command.
    """
    word = text.split()[1]
    status, body = query(text)
    assert_rejected(
        status, body, f"term \"{word}\" is also a command: quote it ('{word}')", text
    )
    assert f"{word}:<value>" not in body["error"], (
        f"{text!r}: error {body['error']!r} offers a clause no command word has"
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
    """A trailing keyword gets a repair instruction, like a mis-cased one.

    "recall ferry Top" is answered with "mis-cased keyword ... quote it ('Top')
    to search for the word". "recall ferry top" must offer the same quote. It
    must not say "Expected colon, but found \"\"", which reports end-of-input
    as an empty literal and offers nothing.
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
    "text,column,fixes",
    [
        ("recall top", 10, ["write top:<value>", "quote it ('top')"]),
        ("recall Top", 10, ["write top:<value>", "quote it ('Top')"]),
        ("recall since 7d", 12, ["write since:7d", "quote it ('since')"]),
        ("recall zebras topic food", 19, ["write topic:food", "quote it ('topic')"]),
        (
            "recall zebras entity alice extra",
            20,
            ["write entity:alice", "quote it ('entity')"],
        ),
    ],
)
def test_reserved_word_as_a_term_names_both_fixes(query, text, column, fixes):
    """A reserved word as a bare term is a 400 at the keyword, in any casing.

    The message offers both readings. The clause fix takes the word after the
    keyword as its value, since that is the query the caller meant. The quote
    is how to search for the word itself. Either fix alone would leave half the
    callers guessing.
    """
    status, body = query(text)
    assert status == 400, f"{text!r}: expected 400, got {status} — {body!r}"
    error = body.get("error") or ""
    assert f"parse error at column {column}:" in error, (
        f"{text!r}: error {error!r} should point at the keyword, column {column}"
    )
    for fix in fixes:
        assert fix in error, f"{text!r}: error {error!r} should offer {fix!r}"


@pytest.mark.parametrize(
    "text,fix",
    [
        ("recall zebras since:7d topic food", "write topic:food"),
        ("recall zebras top:5 depth 2", "write depth:2"),
        ("remember@1 'zebras eat grass' topic food entity:x", "write topic:food"),
    ],
)
def test_keyword_after_a_clause_is_a_missing_colon(query, text, fix, assert_rejected):
    """After a clause has started, a keyword is a clause missing its ':'.

    Terms come first, so it cannot be a word to quote. The message names only
    the clause the caller meant.
    """
    status, body = query(text)
    assert_rejected(status, body, fix, text)
    assert "quote it" not in body["error"], (
        f"{text!r}: error {body['error']!r} suggests a quote, but no term can "
        "stand after a clause"
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
        "recall ferry topic:'since'",
    ],
)
def test_reserved_word_in_value_position_is_data(query, text, assert_accepted):
    """Spelling alone must not make a word syntax.

    A stored anchor called "top" must be nameable without quoting. The Go suite
    pins this boundary as KeywordAsValue. It is pinned again here, because a
    stricter duplicate or bounds rule is the most likely to break it by
    accident.
    """
    status, body = query(text)
    assert_accepted(status, body, text)


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
    query, text, expected, assert_rejected
):
    """A keyword where a number or time belongs is a value error.

    It is not a grammar error, so the message names the clause and the value it
    could not read. A message like "Expected literal, but found \"top\"" would
    mislead. "top" *is* a literal to the caller, and the clause that rejected
    it would go unnamed.
    """
    status, body = query(text)
    assert_rejected(status, body, expected, text)


@pytest.mark.parametrize(
    "text,clause",
    [
        ("remember@1 'the ferry docks at dawn' since:7d", "since"),
        ("remember@1 'the ferry docks at dawn' until:2026-01-15", "until"),
        ("remember@1 'the ferry docks at dawn' top:3", "top"),
        ("remember@1 'the ferry docks at dawn' depth:2", "depth"),
        ("remember@1 'the ferry docks at dawn' topic:harbour since:7d", "since"),
        ("remember@1 'the ferry docks at dawn' Since:7d", "since"),
    ],
)
def test_recall_clause_on_a_remember_names_the_command(
    query, text, clause, assert_rejected
):
    """A well-formed recall clause on a remember is a 400 naming the command.

    The error says the clause is a recall clause and lists the clauses a
    remember takes. The keyword repair ("write since:<value> if a clause was
    meant") would tell the caller to write exactly what they wrote. An agent
    following it would send the same query back. The mistake is the command the
    clause was given to, whatever its casing and wherever it sits among the
    anchors.
    """
    status, body = query(text)
    assert_rejected(
        status,
        body,
        f"{clause}: is a recall clause: a remember takes only topic:, entity: and vec:",
        text,
    )
    assert f"write {clause}:<value>" not in body["error"], (
        f"{text!r}: error {body['error']!r} repeats the clause back as the fix"
    )


# The vec: clause. parseVecField's positioned errors reach the client as it
# produced them, never behind a generic wrap. Every other clause's errors do
# the same (TestClauseErrorsSurfaceUnmangled pins this in Go).


@pytest.mark.parametrize(
    "text,expected",
    [
        ("recall ferry vec:v", "param field operator $"),
        ("recall ferry vec:query", "param field operator $"),
        ("recall ferry vec:$", "expected literal"),
        # without its ':', vec is a keyword among the terms, not a clause
        ("recall ferry vec$:v", 'term "vec" is also a keyword'),
        ("recall ferry vec:$$", "expected literal"),
        ("recall ferry vec:$v extra", "unexpected"),
        ("remember@1 'the ferry docks at dawn' vec:v", "param field operator $"),
        ("remember@1 'the ferry docks at dawn' vec:$", "expected literal"),
    ],
)
def test_vec_clause_errors_surface_unmangled(query, text, expected, assert_rejected):
    """vec:'s inner, positioned error reaches the client unchanged.

    Both call sites return parseVecField's error as it is, like every other
    clause, not behind a generic "Error while parsing" wrap.
    """
    status, body = query(text)
    message = body.get("error") or ""
    assert "error while parsing" not in message.lower(), (
        f"{text!r}: error {message!r} is the generic wrap; return the inner "
        "positioned error unchanged"
    )
    assert_rejected(status, body, expected, text)


# Specials inside quotes. Everything between '...' is data, and only a doubled
# quote is an escape. These must all succeed. A memory system that cannot store
# a fact containing a colon or a plus sign cannot store real sentences.


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
        "recall ferry topic:'US elections'",  # quoted anchor values follow the same rule
        "recall ferry topic:'My Project'",
        "recall ferry entity:'O''Brien'",
    ],
)
def test_specials_inside_quotes_are_data(query, text, assert_accepted):
    """A quoted phrase is opaque, so none of these is a grammar error.

    Reserved words and symbols inside it carry no meaning. This property lets
    real sentences be stored verbatim. It is also the one most at risk from a
    stricter prefix or duplicate rule that forgets to stop at the quote.
    """
    status, body = query(text)
    assert_accepted(status, body, text)


@pytest.mark.parametrize(
    "text,char",
    [
        ("recall ferry; recall bridge", ";"),
        ("recall e-mail", "-"),
        ("recall ferry v1.2", "."),
        ("recall ferry 🍊", "🍊"),
        ("recall ferry topic:machine-learning", "-"),
        ("recall ferry vec:$my_vec", "_"),
        ("remember@1 'the ferry docks at dawn' topic:a.b", "."),
    ],
)
def test_special_character_outside_quotes_is_rejected(
    query, text, char, assert_rejected
):
    """Outside quotes, any character but a letter or digit is a 400 naming it.

    A special character absorbed into a word would change the query without an
    error. "recall ferry; recall bridge" would read "ferry;" as a term, and the
    second recall as a search for the word "recall". Quoting is the escape. The
    test above pins that such characters are data inside quotes.
    """
    status, body = query(text)
    assert_rejected(
        status, body, f'"{char}" is only allowed inside a quoted phrase', text
    )


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
def test_specials_inside_a_remembered_fact_are_stored(query, text, assert_accepted):
    """The write path has to be as opaque as the read path.

    The writes go to graph 1 (loose remembers). A fact is keyed by its value,
    so reruns against a long-lived server change nothing.
    """
    status, body = query(text)
    assert_accepted(status, body, text)


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
def test_unterminated_phrase_is_reported_as_such(query, text, assert_rejected):
    """An unterminated phrase is the only quote error, and it reports well.

    This test pins it, so a stricter phrase rule cannot turn it into a generic
    token error.
    """
    status, body = query(text)
    assert_rejected(status, body, "unterminated", text)


# Temporal values. The message names both accepted forms. These tests pin that
# it keeps doing so for the likely ways an agent gets it wrong.


@pytest.mark.parametrize(
    "text,expected",
    [
        ("recall ferry since:soon", "invalid since value"),
        ("recall ferry since:yesterday", "invalid since value"),
        ("recall ferry since:7", "invalid since value"),
        ("recall ferry since:d", "invalid since value"),
        ("recall ferry since:7dd", "invalid since value"),
        ("recall ferry since:'2026-13-45'", "invalid since value"),
        ("recall ferry since:'2026-02-30'", "invalid since value"),
        ("recall ferry since:'15-01-2026'", "invalid since value"),
        ("recall ferry since:'2026/01/15'", "invalid since value"),
        ("recall ferry until:later", "invalid until value"),
        ("recall ferry until:tomorrow", "invalid until value"),
        ("recall ferry until:0x10", "invalid until value"),
        ("recall ferry until:7 d", "invalid until value"),
        ("recall ferry until:--7d", "invalid until value"),
        ("recall ferry since:", "expected"),
        # an unquoted date: the message shows the quoted form that parses
        ("recall ferry since:2026-01-15", "a quoted date like '2026-01-15'"),
    ],
)
def test_invalid_temporal_values_name_the_accepted_forms(
    query, text, expected, assert_rejected
):
    """A temporal error has to teach the grammar.

    Duration versus date is exactly what an agent guesses wrong. The message
    ("expected a duration like 7d or a quoted date like '2026-01-15'") is the
    model for every other value error in this file.
    """
    status, body = query(text)
    assert_rejected(status, body, expected, text)


@pytest.mark.parametrize(
    "text,expected",
    [
        (
            "recall ferry since:106752d",
            'invalid since value "106752d": out of range (at most 106751d)',
        ),
        (
            "recall ferry until:15251w",
            'invalid until value "15251w": out of range (at most 15250w)',
        ),
        ("recall ferry since:2562048h", "out of range (at most 2562047h)"),
        ("recall ferry since:153722868m", "out of range (at most 153722867m)"),
        ("recall ferry since:9223372037s", "out of range (at most 9223372036s)"),
        ("recall ferry since:99999999999999999999d", "out of range (at most 106751d)"),
    ],
)
def test_durations_longer_than_a_duration_holds_are_rejected(
    query, text, expected, assert_rejected
):
    """A duration past about 292 years is out of range.

    The message says how far each unit goes. Wrapped negative, since:106752d
    would resolve to a bound in the future. It would run as a window opening
    after now: a 200 with nothing in it, answering a question nobody asked. The
    limit is per unit, so the message names the largest count the caller's own
    unit allows.
    """
    status, body = query(text)
    assert_rejected(status, body, expected, text)


@pytest.mark.parametrize(
    "text",
    [
        "recall ferry since:7d",
        "recall ferry since:0d",
        "recall ferry until:30d",
        "recall ferry since:'2026-01-15'",
        "recall ferry until:'2026-12-31'",
        "recall ferry since:7d until:30d",
        "recall ferry since:'2026-01-15' until:'2026-12-31'",
        "recall ferry since:'2026-01-15T10:00:00Z'",  # quoted, a time of day fits
        "recall@1 ferry since:7d top:5",
        "recall zebras since:7d until:'2026-01-15' depth:2 top:5",
    ],
)
def test_valid_temporal_values_parse(query, text, assert_accepted):
    """Both accepted forms parse at both bounds, the zero duration included.

    This is the guard rail for any stricter temporal validation.
    """
    status, body = query(text)
    assert_accepted(status, body, text)


# Graph selector. Every malformed selector gets a message of its own, not a
# generic token error. These tests pin each one.


@pytest.mark.parametrize(
    "text,expected",
    [
        ("recall@256 ferry", "out of range"),
        ("recall@300 ferry", "out of range"),
        ("recall@99999 ferry", "out of range"),
        ("remember@256 'the ferry docks at dawn'", "out of range"),
        ("recall@3.5 ferry", "expected a space after the command"),
        ("recall@abc ferry", "whole number"),
        ("recall@ ferry", "whole number"),
        ("recall@topic ferry", "whole number"),
        ("recall@3@5 ferry", "one selector"),
        ("recall@-1 ferry", "whole number"),
    ],
)
def test_graph_selector_errors_stay_specific(query, text, expected, assert_rejected):
    """Each selector validation layer keeps its own message.

    The parser rejects what cannot fit uint8. The handler rejects what fits but
    names no allocated graph. Separate messages stop a regression in one layer
    from hiding behind the other.
    """
    status, body = query(text)
    assert_rejected(status, body, expected, text)


# Whitespace and adjacency. Any run of space, tab or carriage return separates
# words, and a trailing newline ends the instruction. The parts of a command or
# a clause are glued: the selector to its verb, a value to its ':' and a
# parameter name to its '$'. A space inside one is rejected with a message
# saying where it is not allowed.


@pytest.mark.parametrize(
    "text",
    [
        "recall  zebras",
        "recall\tzebras",
        "recall zebras ",
        "recall zebras \n  \n\t",  # trailing blank lines are not a second instruction
        "recall 'zebras\x00food'",  # a NUL inside quotes is data
    ],
)
def test_whitespace_separates_words(query, text, assert_accepted):
    """Spacing is not meaning: each of these is a one-term recall and parses.

    A client that pretty-prints, tab-separates or ends its payload with a
    newline must not get a 400 for it.
    """
    status, body = query(text)
    assert_accepted(status, body, text)


def test_nul_outside_quotes_is_rejected_by_name(query, assert_rejected):
    """A NUL outside a phrase is rejected by name.

    Read as the end of input, it would silently drop everything after it.
    """
    text = "recall zebras\x00food"
    status, body = query(text)
    assert_rejected(
        status, body, "a NUL character is only allowed inside a quoted phrase", text
    )


@pytest.mark.parametrize(
    "text,expected",
    [
        ("remember @1 'x'", "no space allowed between remember and @"),
        ("recall @1 zebras", "no space allowed between recall and @"),
        ("remember@ 1 'x'", "no space allowed between @ and the selector"),
        ("recall zebras topic :food", "no space allowed before :"),
        ("recall zebras topic: food", "no space allowed after :"),
        ("recall zebras since :7d", "no space allowed before :"),
        ("recall zebras vec: $v", "no space allowed after :"),
        ("recall zebras vec:$ v", "no space allowed after $"),
        ("recall zebras : topic:food", "stray"),
    ],
)
def test_no_space_inside_a_command_or_clause(query, text, expected, assert_rejected):
    """A space inside a command or clause is a 400 saying where it cannot go.

    "unexpected" or a blamed '@' leaves the caller to work out that the only
    fix is deleting a space. The message should say so. Rejected writes are
    pinned to graph 1, so a regression that accepts one lands among the loose
    remembers.
    """
    status, body = query(text)
    assert_rejected(status, body, expected, text)


# Structure: grouping, multiple commands, newlines. Each is rejected with the
# rule it broke. The generic "unexpected" fallback would tell an agent nothing
# about which rule it hit.


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
    """Parentheses lex to LPAREN/RPAREN, and no rule accepts them.

    So the error says grouping is unsupported, not only which character it
    found. "Encountered unexpected token: \"(\"" would leave the caller
    guessing.
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
        "remember@1 'a' remember@1 'b'",
        "recall ferry\nrecall bridge",
        "recall ferry\nbridge",
    ],
)
def test_second_command_is_rejected_as_one_per_instruction(query, text):
    """FQL is one command per instruction.

    The newline cases are the dangerous ones. A lexer that skipped \\n as a
    blank would silently turn "recall ferry\\nbridge" into a two-term recall.
    The lexer emits a NEWLINE token, and the parser rejects a second
    instruction after it.
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


@pytest.mark.parametrize(
    "text,word",
    [
        ("recall ferry recall bridge", "recall"),
        ("recall ferry remember 'x'", "remember"),
    ],
)
def test_command_word_among_the_terms_is_a_word_to_quote(
    query, text, word, assert_rejected
):
    """A command word among the terms is a word the caller forgot to quote.

    It is not a second command, so the error names the quote.
    """
    status, body = query(text)
    assert_rejected(
        status, body, f"term \"{word}\" is also a command: quote it ('{word}')", text
    )


# Stop words. The index drops English stop words from stored facts, so a bare
# stop word can never match. It runs with a warning at the term. A recall whose
# only search terms are stop words is a 400, not an empty result that looks
# like a miss. A phrase never warns for its content.


@pytest.mark.parametrize(
    "text,warnings",
    [
        ("recall the parrot", [(10, "the")]),
        ("recall parrot and the zebra", [(17, "and"), (21, "the")]),
        ("recall the 'parrot'", [(10, "the")]),  # a phrase is a real seed
    ],
)
def test_stop_word_term_runs_and_warns(query, text, warnings, assert_accepted):
    """Each bare stop word gets one warning, positioned at the term.

    The warning says why the word cannot match.
    """
    status, body = query(text)
    assert_accepted(status, body, text)
    assert body.get("warnings") == [
        f'parse warning at column {column}: term "{term}" is a stop word: '
        "stored facts never contain it, so it cannot match"
        for column, term in warnings
    ], f"{text!r}: warnings {body.get('warnings')!r}"


def test_stop_word_beside_a_vector_warns(query, vector, planets_graph, assert_accepted):
    """A vector is a seed, so a stop word beside it only warns.

    The planet graph never gets a vector, so a vector of any width is answered
    there, not rejected for its dimension.
    """
    text = f"recall@{planets_graph} the vec:$v"
    status, body = query(text, parameters={"v": vector(3)})
    assert_accepted(status, body, text)
    column = len(f"recall@{planets_graph} the")
    assert body.get("warnings") == [
        f'parse warning at column {column}: term "the" is a stop word: '
        "stored facts never contain it, so it cannot match"
    ], f"{text!r}: warnings {body.get('warnings')!r}"


@pytest.mark.parametrize(
    "text",
    [
        "recall the",
        "recall the and",
        "recall the topic:birds",  # anchors do not rescue it
    ],
)
def test_stop_word_only_recall_is_rejected(query, text, assert_rejected):
    """A recall whose bare terms are all stop words is a 400 naming the fix.

    With no phrase and no vector, it can match nothing. An empty result would
    look like a miss. Anchors do not change that. `recall topic:birds` is the
    query that was meant, and it is accepted on its own.
    """
    status, body = query(text)
    assert_rejected(
        status,
        body,
        "so nothing can match; give a term that is not a stop word, a phrase, "
        "or a vector",
        text,
    )
    assert 'term "the" is a stop word' in body["error"], body["error"]


# Anchor-seeded recall. Anchors are seeds, so "everything about billing" is a
# natural query: with no term or vector beside them the anchors seed the
# recall with everything filed under them. The results themselves are pinned
# in recall_test.py; these pin that the shape parses with every modifier a
# recall takes.


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
def test_anchor_only_recall_is_reachable(query, text, assert_accepted):
    """A recall with anchors and no text term is a well-formed question.

    An anchor is a seed, so everything filed under it seeds the recall. The
    parser accepts each shape whatever the graph holds.
    """
    status, body = query(text)
    assert_accepted(status, body, text)


# Degenerate and pathological input. The floor: never a 500, never an empty
# message, never a hang.


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
    """Every unparsable string is a 400 carrying a non-empty message."""
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

    A parse-time length ceiling would be a fair answer. This test pins only
    that the server stays a well-behaved HTTP service either way.
    """
    status, body = query(text)
    assert status in (200, 400, 413), (
        f"{text[:40]!r}...: expected 200/400/413, got {status} — {body!r}"
    )
    if status != 200:
        assert body.get("error"), "a rejection must carry a message"


@pytest.mark.parametrize(
    "text, err",
    [
        (
            "remember 'a fact'x",
            'parse error at column 18: expected a whitespace, found "x"',
        ),
        (
            "remember 'a fact'zzz topic:a",
            'parse error at column 20: expected a whitespace, found "zzz"',
        ),
    ],
)
def test_glued_token_in_remember_commands(query, text, err):
    """A token glued to a remember's quoted fact is a 400 at that token."""
    status, body = query(text)

    assert status == 400
    assert body.get("error") == err
