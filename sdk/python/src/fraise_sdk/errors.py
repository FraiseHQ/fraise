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
    """A warning the server attached to a successful response.

    The query ran and its results are valid; the server is flagging a reading
    the caller may not have meant — e.g. a leading recall term that spells a
    grammar keyword, where ``recall since 7d`` is one ``:`` away from
    ``recall since:7d``. Emitted through :mod:`warnings` so it is visible by
    default and silenceable by category::

        warnings.filterwarnings("ignore", category=FraiseWarning)
    """


class FraiseQueryError(FraiseError):
    """A query could not be built from the given arguments.

    Raised before any request leaves the client — e.g. an empty fact value, or
    a keyword with embedded whitespace, which the server's query grammar cannot
    represent.
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
