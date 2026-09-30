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

"""Tests for the standard-library backports, run against the stdlib they copy.

Setup lives in conftest.py; this file is assertions.
"""

import sys
import warnings
from unittest.mock import patch

import pytest
from fraise_sdk import FraiseClient, FraiseWarning
from fraise_sdk.compat import _warn


def test_the_backport_names_the_callers_line(session):
    """On Python 3.11 a FraiseWarning still names the caller's line.

    CI runs 3.12 and newer, where ``warn`` is the standard library's, so the
    backport is swapped into the SDK's deepest warning path —
    ``recall``, ``build_recall``, ``_term`` — to prove it on any interpreter.
    A warning pointing into the SDK leaves the caller hunting for the line.
    """
    with (
        patch("fraise_sdk.query.warn", _warn),
        pytest.warns(FraiseWarning, match="is a reserved word") as record,
    ):
        FraiseClient().recall("since", "7d")
    assert record[0].filename == __file__


@pytest.mark.skipif(
    sys.version_info < (3, 12), reason="compares against 3.12's warnings.warn"
)
def test_the_backport_skips_the_prefixes_the_standard_library_skips(session):
    """The backport attributes a warning exactly where 3.12's ``warn`` does.

    Both run the same SDK call from the same line, so a difference in file or
    line can only be a difference in how they walk the stack past the SDK.
    """
    seen = []
    for implementation in (_warn, warnings.warn):
        with (
            patch("fraise_sdk.query.warn", implementation),
            pytest.warns(FraiseWarning) as record,
        ):
            FraiseClient().recall("since", "7d")
        seen.append((record[0].filename, record[0].lineno))
    assert seen[0] == seen[1]


@pytest.mark.parametrize("stacklevel", [1, 2, 3])
def test_the_backport_counts_a_plain_stacklevel_as_the_standard_library_does(
    stacklevel,
):
    """Without prefixes the backport is ``warnings.warn``, frame for frame.

    Its own frame must never be counted: an off-by-one here would move every
    warning the SDK raises one line away from where it was aimed.
    """
    seen = []
    for implementation in (_warn, warnings.warn):
        with pytest.warns(UserWarning, match="probe") as record:
            implementation("probe", stacklevel=stacklevel)
        seen.append((record[0].filename, record[0].lineno))
    assert seen[0] == seen[1]
