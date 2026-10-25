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
@pytest.mark.parametrize("unit", ["turn-f1@10", "turn-p@1", "turn-recall@10"])
def test_retrieval_does_not_drop(comparison, fail_regressed, unit):
    """No retrieval metric drops by more than 0.001, over all questions or in any category.

    The metrics count a hit only when it is an evidence turn, the fact the
    reader needs.

    The metrics are exact. The same server ranks the same facts the same way
    on any machine. So one run decides it, and 0.001 is not a noise margin. It
    is the smallest drop worth a look.
    """
    dropped = [
        row for row in comparison if row.unit == unit and row.base - row.head > 0.001
    ]
    fail_regressed(dropped)


@pytest.mark.nightly
@pytest.mark.bench("./pkg/server", "BenchmarkHTTP", count=5, benchtime="1x")
def test_http_slowdowns_are_reported(comparison, warn_slower):
    """A significant rise of 10% or more in a latency percentile is reported, without failing.

    The baseline was measured on another runner, so latency is shown for a
    reviewer to judge, not gated. Each run fills a server and holds it under
    load for twenty seconds, so this gate runs nightly only.
    """
    for row in comparison:
        if (
            row.unit in ("p50-sec", "p95-sec", "p99-sec")
            and row.significant
            and row.change >= 0.10
        ):
            warn_slower(row)
