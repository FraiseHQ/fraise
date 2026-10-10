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

"""Gates on internal/hash: hashing a key with each supported function."""

import pytest


@pytest.mark.bench("./internal/hash", "Benchmark")
def test_slowdowns_are_reported(comparison, warn_slower):
    """A significant slowdown of 10% or more is reported, without failing.

    The baseline was timed on another runner, and two hosted runners differ by
    more than the slowdowns worth catching, so time is shown for a reviewer to
    judge rather than gated; allocations, which are counted, are.
    """
    for row in comparison:
        if row.unit == "sec/op" and row.significant and row.change >= 0.10:
            warn_slower(row)


@pytest.mark.bench("./internal/hash", "Benchmark")
def test_allocations_do_not_grow(comparison):
    """No benchmark allocates 5% more per operation, or allocates at all where it did not.

    Allocations are counted, not timed, so they do not vary with the runner
    and need no statistics.
    """
    grown = [
        row for row in comparison if row.unit == "allocs/op" and row.change >= 0.05
    ]
    assert not grown
