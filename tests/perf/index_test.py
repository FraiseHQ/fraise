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

"""Gates on internal/index: the text and vector index searches."""

import pytest


@pytest.mark.bench("./internal/index", "Benchmark")
def test_time_does_not_regress(comparison, warn_slower):
    """No benchmark slows by 25% or more, at p < 0.05 over ten runs a side; 10% warns.

    A hosted runner's noise is a few percent run to run, so only a large,
    significant slowdown fails; a smaller significant one is reported for a
    reviewer to judge.
    """
    slower = [
        row
        for row in comparison
        if row.unit == "sec/op" and row.significant and row.change >= 0.10
    ]
    for row in slower:
        warn_slower(row)
    assert not [row for row in slower if row.change >= 0.25]


@pytest.mark.bench("./internal/index", "Benchmark")
def test_allocations_do_not_grow(comparison):
    """No benchmark allocates 5% more per operation, or allocates at all where it did not.

    Allocations are counted, not timed, so they do not vary with the runner
    and need no statistics.
    """
    grown = [
        row for row in comparison if row.unit == "allocs/op" and row.change >= 0.05
    ]
    assert not grown
