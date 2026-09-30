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

"""Standard-library APIs the SDK uses, on every Python it supports.

The SDK is written against the newest spelling of an API, and this module
supplies that spelling on the older interpreters the package still supports.
Call sites read as they will once the floor moves, and a backport is deleted
rather than migrated when it does.
"""

from __future__ import annotations

import sys
import warnings


def _warn(
    message: str | Warning,
    category: type[Warning] | None = None,
    stacklevel: int = 1,
    source: object | None = None,
    *,
    skip_file_prefixes: tuple[str, ...] = (),
) -> None:
    """Warn as :func:`warnings.warn` does from Python 3.12, on Python 3.11.

    Frames whose file starts with one of ``skip_file_prefixes`` are not counted
    toward ``stacklevel``, so a warning lands on the first line outside those
    files however deep the call that raised it — a fixed ``stacklevel`` is right
    for one call path only. The walk is translated into the plain
    ``stacklevel`` 3.11 understands, by 3.12's rules: counting starts at the
    caller, and any prefix raises ``stacklevel`` to at least 2.

    Args:
        message: The warning text, or a warning instance.
        category: The warning class; :class:`UserWarning` when omitted.
        stacklevel: How many counted frames up the warning is attributed.
        source: The object that caused the warning, passed through.
        skip_file_prefixes: File-name prefixes whose frames are not counted.
    """
    if skip_file_prefixes:
        stacklevel = max(2, stacklevel)
    # From inside this function, level 2 is its caller. Each counted step moves
    # one frame up and then past every frame a prefix skips.
    level = 2
    frame = sys._getframe(1)
    for _ in range(stacklevel - 1):
        frame = frame.f_back
        level += 1
        while frame is not None and frame.f_code.co_filename.startswith(
            skip_file_prefixes
        ):
            frame = frame.f_back
            level += 1
        if frame is None:
            break
    warnings.warn(message, category, stacklevel=level, source=source)


if sys.version_info >= (3, 12):
    warn = warnings.warn
else:
    warn = _warn
