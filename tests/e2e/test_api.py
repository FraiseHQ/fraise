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
bodies, and query strings the parser must reject as client errors."""

import requests


def test_health_check(get):
    response = get("/")

    assert response.status_code == 200
    assert response.json() == {"status": "ok"}


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
    hang. Graph 9 is above the eight graphs the store allocates."""
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
    executing against another tenant's graph."""
    for q in ("remember@256 'secret plan' topic:x", "recall@300 anything"):
        status, body = query(q)

        assert status == 400, f"{q!r}: expected 400, got {status}"
        assert "out of range" in body.get("error", ""), (
            f"{q!r}: expected the parser's out-of-range error, got {body.get('error')!r}"
        )


def test_query_rejects_non_integer_selector(query):
    """A non-numeric selector is a parse error, not a silent fallback to a
    default graph."""
    status, body = query("recall@abc anything")

    assert status == 400
    assert body.get("error"), "expected a parse error message"


def test_query_rejects_valid_uint8_selector_above_num_graphs(query):
    """@255 fits the selector type, so the parser passes it — the handler must
    then reject it against the allocated graph count with its own distinct
    error. This is the layer boundary: type consistency in the parser,
    allocation policy in the handler."""
    status, body = query("recall@255 anything")

    assert status == 400
    assert "does not exist" in body.get("error", ""), (
        f"expected the handler's does-not-exist error, got {body.get('error')!r}"
    )


def test_wrapping_selector_write_does_not_leak_to_graph_zero(query):
    """Regression guard for the wrap itself: a rejected remember@256 must leave
    no trace on graph 0 (the graph @256 used to wrap to). Read-only on graph 0
    apart from the probe recall, so it does not disturb that graph's facts."""
    status, _ = query("remember@256 'wrapprobe should never land' topic:wrapprobe")
    assert status == 400

    status, body = query("recall@0 wrapprobe")
    assert status == 200
    assert body["results"]["count"] == 0, (
        "a rejected @256 write leaked onto graph 0 — uint8 wrap regression"
    )
