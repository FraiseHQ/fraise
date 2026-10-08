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

"""Gates on pkg/server: retrieval quality on LoCoMo, and latency through the HTTP API."""

import pytest


@pytest.mark.bench(
    "./pkg/server",
    "BenchmarkRetrievalQuality",
    count=1,
    benchtime="1x",
    data={"FRAISE_LOCOMO": "data/locomo-conv-26.json"},
)
@pytest.mark.parametrize("unit", ["f1@10", "p@1", "recall@10"])
def test_retrieval_does_not_drop(comparison, unit):
    """No retrieval metric drops by more than 0.001, over all questions or in any category.

    The metrics are exact, the same server ranking the same facts the same
    way, so one run a side decides it and 0.001 is not a noise margin: it is
    the smallest drop worth a look.
    """
    dropped = [
        row for row in comparison if row.unit == unit and row.base - row.head > 0.001
    ]
    assert not dropped


@pytest.mark.nightly
@pytest.mark.bench("./pkg/server", "BenchmarkHTTP", count=5, benchtime="1x")
def test_http_p99_is_not_slower(comparison):
    """The p99 through the HTTP API does not grow by 25% or more, at p < 0.05 over five runs a side.

    Five is the fewest runs a side at which the U test can reach p < 0.05;
    each is a server filled and held under load for twenty seconds, which is
    why this gate runs nightly rather than on every pull request.
    """
    slower = [
        row
        for row in comparison
        if row.unit == "p99-sec" and row.significant and row.change >= 0.25
    ]
    assert not slower
