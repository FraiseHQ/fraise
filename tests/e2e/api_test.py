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
bodies, and query strings the parser must reject as client errors.
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
