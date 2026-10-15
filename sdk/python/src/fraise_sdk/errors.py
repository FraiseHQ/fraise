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

"""Exceptions raised by the Fraise SDK, and the warning category it emits."""

from __future__ import annotations


class FraiseError(Exception):
    """Base class for every error raised by the SDK."""


class FraiseWarning(UserWarning):
    """A warning about a query that ran, from the server or from the SDK.

    The query ran and its results are valid, but something in it may not be
    what the caller meant. The server attaches one to a successful response,
    for example for a stop-word term that can never match. The typed helpers
    emit one before sending. ``recall("since", "7d")`` warns, because it
    searches the word "since", one ``:`` away from a ``since:7d`` bound. A
    failed extraction warns too, because the fact is stored without the
    extracted anchors.

    It is emitted through :mod:`warnings`, so it is visible by default and can
    be silenced by category::

        warnings.filterwarnings("ignore", category=FraiseWarning)

    In every case it names the caller's own line, never a line inside the SDK.
    """


class FraiseQueryError(FraiseError):
    """A query could not be built from the given arguments.

    It is raised before any request leaves the client. Examples are an empty
    fact value or anchor, or a graph id outside 0–255. The server's query
    grammar can represent none of these.
    """


class FraiseAPIError(FraiseError):
    """The server rejected a request or failed to execute it.

    Carries the HTTP status code and the server-supplied error message (the
    ``error`` field of the JSON body, when present).
    """

    def __init__(self, status_code: int, message: str) -> None:
        self.status_code = status_code
        self.message = message
        super().__init__(f"fraise request failed [{status_code}]: {message}")
