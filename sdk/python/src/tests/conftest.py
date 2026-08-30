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

"""Fixtures for the Fraise Python SDK unit suite.

Every fixture the suite uses lives here, including the ones a single test file
asks for: a test module is assertions, and a fixture defined among them hides
setup where nobody looks for it. Test modules never import from this file —
the only channel out is a fixture, injected through a test's arguments — so
the values behind them are private by convention and by the leading
underscore.

Nothing here reaches the network. The client's own `requests.Session` is
patched at its import site, so the whole suite runs with no server and no
daemon.
"""

import json
from unittest.mock import MagicMock, patch

import pytest
from fraise_sdk.client import DEFAULT_BASE_URL

_QUERY_URL = f"{DEFAULT_BASE_URL}/api/v1/q"
_NO_HITS = {"results": {"count": 0, "hits": []}}

# The shape the server sends for the grammar's one surviving ambiguity: a
# leading recall term that spells a keyword ran as a term search, and the
# warning names the clause it nearly is.
_SERVER_WARNING = (
    'parse warning at column 15: term "since" is also a keyword: write '
    "since:<value> if a clause was meant, or quote it ('since') to search "
    "for the word"
)


# -- addresses and payloads --------------------------------------------------


@pytest.fixture(scope="session")
def query_url():
    """The URL every query the client sends must be posted to."""
    return _QUERY_URL


@pytest.fixture(scope="session")
def no_hits():
    """A response body carrying an empty recall result."""
    return dict(_NO_HITS)


@pytest.fixture(scope="session")
def server_warning():
    """A parse warning exactly as the server words it."""
    return _SERVER_WARNING


# -- the patched session -----------------------------------------------------


def _arm(session, body: dict, status_code: int = 200) -> MagicMock:
    """Arm ``session`` to answer the next POST with ``body``.

    Private: tests reach this through the ``respond`` fixture. The ``session``
    fixture needs it before any fixture argument could be injected, which is
    why it exists as a function at all.
    """
    response = MagicMock(
        status_code=status_code,
        ok=200 <= status_code < 300,
        text=json.dumps(body),
    )
    response.json.return_value = body
    session.post.return_value = response
    return response


@pytest.fixture
def session():
    """The session the client builds for itself, patched at its import site.

    Patching rather than injecting keeps the client's own construction path —
    the one every caller takes — under test.

    Yields:
        The mock session every FraiseClient built in the test will use, armed
        to answer with an empty recall result.
    """
    with patch("fraise_sdk.client.requests.Session") as session_class:
        patched = session_class.return_value
        _arm(patched, _NO_HITS)
        yield patched


@pytest.fixture
def respond():
    """Callable arming a session to answer the next POST with a given body.

    Returns:
        ``callable(session, body, status_code=200) -> MagicMock`` — the mock
        response, for tests that want to assert on it directly.
    """
    return _arm


def _arm_get(session, body, status_code: int = 200) -> MagicMock:
    """Arm ``session`` to answer the next GET with ``body``.

    Private: tests reach this through the ``respond_get`` fixture. Mirrors
    ``_arm``, which arms POST — the health and version probes are the
    client's only GETs.
    """
    response = MagicMock(
        status_code=status_code,
        ok=200 <= status_code < 300,
        text=json.dumps(body),
    )
    response.json.return_value = body
    session.get.return_value = response
    return response


@pytest.fixture
def respond_get():
    """Callable arming a session to answer the next GET with a given body.

    The ``session`` fixture arms POST only; health/version/compatibility
    tests arm the GET side through this.

    Returns:
        ``callable(session, body, status_code=200) -> MagicMock`` — the mock
        response, for tests that want to alter it directly.
    """
    return _arm_get


@pytest.fixture
def sent():
    """Callable returning the JSON payload of the single POST a session made.

    Returns:
        ``callable(session) -> dict``, which also asserts exactly one POST
        happened.
    """

    def _sent(session) -> dict:
        session.post.assert_called_once()
        return session.post.call_args.kwargs["json"]

    return _sent


# -- embedders ---------------------------------------------------------------


@pytest.fixture(scope="session")
def encode():
    """The bare ``callable(text) -> vector`` embedder shape: len(text), 4 times.

    Deterministic, so a test can predict the vector the client will send.
    """

    def _encode(text: str) -> list[float]:
        return [float(len(text))] * 4

    return _encode


@pytest.fixture
def callable_embedder(encode):
    """Callable building a mock of the bare ``callable(text) -> vector`` shape.

    Returns:
        ``callable() -> MagicMock``. A fresh mock per call, so a test using two
        embedders can tell their call records apart.
    """

    def _callable_embedder() -> MagicMock:
        embedder = MagicMock(side_effect=encode)
        # A plain callable has no .embed — deleting it is what sends
        # resolve_embedder down the callable branch instead of the Embedder one.
        del embedder.embed
        return embedder

    return _callable_embedder
