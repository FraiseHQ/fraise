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

"""Fixtures and hooks of the benchmark gates.

A gate is a test marked ``bench(package, pattern, ...)``: the marker names the
Go benchmarks it judges and how to run them, and the ``comparison`` fixture
hands it one row per benchmark and unit of benchstat's comparison, base
against head, which the test asserts its limits on. Go's own tools do the
work: benchdiff runs the benchmarks at ``--bench-base`` and at the working
tree, and benchstat compares them (each side's median, a Mann-Whitney U test
for a timed unit, none for one declared exact). Gates sharing a marker share
one measurement. Without ``--bench-base`` every gate is skipped.

A pull request labelled perf-regression-accepted with a ``Perf:`` line in its
description marks every gate xfail with that line as the reason: its
regressions still show, as accepted. The session ends by writing report.md,
the verdict and benchstat's tables, which CI posts on the pull request.
"""

import csv
import io
import json
import os
import re
import shutil
import subprocess
import warnings
from dataclasses import dataclass
from pathlib import Path

import pytest

_HERE = Path(__file__).resolve().parent
_ROOT = _HERE.parents[1]
_BENCHDIFF = ["go", "run", "github.com/willabides/benchdiff/cmd/benchdiff@v0.9.1"]
_BENCHSTAT = [
    "go",
    "run",
    "golang.org/x/perf/cmd/benchstat@v0.0.0-20260929162123-406019bb8b68",
]
_ACCEPT_LABEL = "perf-regression-accepted"
_COMPARISONS = pytest.StashKey[dict]()
_OUTCOMES = pytest.StashKey[list]()


@dataclass(frozen=True)
class _Row:
    """One benchmark's unit as a row of benchstat's comparison.

    benchstat names units in base units (ns/op is sec/op) and drops the
    Benchmark prefix; the GOMAXPROCS suffix is dropped here. delta is its
    verdict, ``~`` for noise; p is None for an exact unit.
    """

    benchmark: str
    unit: str
    base: float
    head: float
    delta: str
    p: float | None

    @property
    def change(self):
        """The head's change relative to the base; growing from zero is infinite."""
        if self.base == 0:
            return 0.0 if self.head == 0 else float("inf")
        return (self.head - self.base) / abs(self.base)

    @property
    def significant(self):
        """Whether benchstat counts the difference as a change rather than noise."""
        return self.delta != "~" and self.head != self.base

    def __str__(self):
        test = "exact" if self.p is None else f"p={self.p:.3f}"
        return f"{self.benchmark} {self.unit}: {self.base:.4g} -> {self.head:.4g} ({self.delta}, {test})"


def _run(cmd, env=None):
    done = subprocess.run(
        cmd, cwd=_ROOT, env=env, capture_output=True, text=True, check=False
    )
    if done.returncode != 0:
        # A failing benchmark says why on stdout, a failing build on stderr.
        pytest.fail(
            f"{' '.join(cmd)} exited {done.returncode}:\n{done.stdout[-4000:]}{done.stderr[-4000:]}",
            pytrace=False,
        )
    return done.stdout


def _rows(text):
    rows, unit = [], ""
    for record in csv.reader(io.StringIO(text)):
        if len(record) < 7:
            continue
        name, base, _, head, _, delta, p = record[:7]
        if not name:
            unit = base if delta == "vs base" else unit
        elif name != "geomean" and base and head:
            match = re.search(r"p=([\d.]+)", p)
            rows.append(
                _Row(
                    re.sub(r"-\d+$", "", name),
                    unit,
                    float(base),
                    float(head),
                    delta,
                    float(match[1]) if match else None,
                )
            )
    return rows


def pytest_addoption(parser):
    group = parser.getgroup("benchmark gates")
    group.addoption(
        "--bench-base",
        help="the git ref the gates compare the working tree against; without one they are skipped",
    )
    group.addoption(
        "--bench-out",
        default=str(_ROOT / "bin" / "perf"),
        help="where both sides' output and report.md go",
    )


def pytest_configure(config):
    config.addinivalue_line(
        "markers",
        "bench(package, pattern, count=10, benchtime=None, data=None): a gate on the Go benchmarks "
        "matching pattern in package, run count times a side; data maps environment variables to "
        "files under tests/perf the benchmarks read",
    )
    config.addinivalue_line(
        "markers", "nightly: a gate too slow to run on every pull request"
    )
    config.stash[_COMPARISONS] = {}
    config.stash[_OUTCOMES] = []


def pytest_collection_modifyitems(config, items):
    path = os.environ.get("GITHUB_EVENT_PATH")
    pr = json.loads(Path(path).read_text()).get("pull_request") if path else None
    if not pr or _ACCEPT_LABEL not in {label["name"] for label in pr.get("labels", [])}:
        return
    reason = next(
        (
            line.strip()
            for line in (pr.get("body") or "").splitlines()
            if line.startswith("Perf:")
        ),
        None,
    )
    if reason:
        for item in items:
            if item.get_closest_marker("bench"):
                item.add_marker(pytest.mark.xfail(reason=reason, strict=False))


@pytest.hookimpl(wrapper=True)
def pytest_runtest_makereport(item, call):
    rep = yield
    # One outcome per gate: its call, or the setup that never got that far.
    if item.get_closest_marker("bench") and (
        rep.when == "call" or (rep.when == "setup" and rep.failed)
    ):
        if hasattr(rep, "wasxfail") and rep.skipped:
            item.config.stash[_OUTCOMES].append(("Accepted", item.nodeid, rep.wasxfail))
        elif rep.failed:
            item.config.stash[_OUTCOMES].append(
                (
                    "Failed",
                    item.nodeid,
                    call.excinfo.exconly()[:1000] if call.excinfo else "",
                )
            )
    return rep


def pytest_sessionfinish(session, exitstatus):
    base = session.config.getoption("bench_base")
    if not base:
        return
    out = Path(session.config.getoption("bench_out")).resolve()
    out.mkdir(parents=True, exist_ok=True)
    outcomes = session.config.stash[_OUTCOMES]
    regressed = exitstatus != 0
    lines = [
        f"### {'🔴 Benchmarks regressed' if regressed else '🟢 No regression'} against `{base}`",
        "",
    ]
    for kind in ("Failed", "Accepted"):
        listed = [
            f"- `{nodeid}`: {message}" for k, nodeid, message in outcomes if k == kind
        ]
        if listed:
            lines += [f"**{kind}**", "", *listed, ""]
    for name, (base_out, head_out) in session.config.stash[_COMPARISONS].items():
        table = subprocess.run(
            [*_BENCHSTAT, f"base={base_out}", f"head={head_out}"],
            cwd=_ROOT,
            capture_output=True,
            text=True,
            check=False,
        )
        lines += [
            f"<details><summary>{name}</summary>",
            "",
            "```",
            table.stdout.rstrip(),
            "```",
            "",
            "</details>",
            "",
        ]
    (out / "report.md").write_text("\n".join(lines))


@pytest.fixture
def comparison(request, pytestconfig):
    """The marked benchmarks compared, base against head, one row per benchmark and unit.

    A base that predates the benchmarks has nothing to compare against, and
    the gate is skipped rather than passed.
    """
    base = pytestconfig.getoption("bench_base")
    if not base:
        pytest.skip("no --bench-base to compare against")
    marker = request.node.get_closest_marker("bench")
    package, pattern = marker.args
    name = f"{package} {pattern}"
    comparisons = pytestconfig.stash[_COMPARISONS]
    if name not in comparisons:
        out = Path(pytestconfig.getoption("bench_out")).resolve()
        slug = re.sub(r"\W+", "-", name).strip("-")
        cache = out / "benchdiff" / slug
        shutil.rmtree(cache, ignore_errors=True)
        count = marker.kwargs.get("count", 10)
        cmd = [
            *_BENCHDIFF,
            f"--base-ref={base}",
            f"--packages={package}",
            f"--bench={pattern}",
            f"--count={count}",
        ]
        cmd += ["--cpu=4", "--benchmem", f"--cache-dir={cache}", "--force-base"]
        if marker.kwargs.get("benchtime"):
            cmd.append(f"--benchtime={marker.kwargs['benchtime']}")
        if count > 1:
            cmd.append("--warmup-count=1")
        data = {
            var: str(_HERE / path)
            for var, path in (marker.kwargs.get("data") or {}).items()
        }
        _run(cmd, env={**os.environ, **data})
        head_out, base_out = out / f"head-{slug}.txt", out / f"base-{slug}.txt"
        shutil.copy(cache / "benchdiff-worktree.out", head_out)
        shutil.copy(
            next(
                f
                for f in cache.glob("benchdiff-*.out")
                if f != cache / "benchdiff-worktree.out"
            ),
            base_out,
        )
        comparisons[name] = (base_out, head_out)
    base_out, head_out = comparisons[name]
    if "\nBenchmark" not in base_out.read_text():
        pytest.skip(f"{base} has no results for {name} to compare against")
    return _rows(
        _run([*_BENCHSTAT, "-format", "csv", f"base={base_out}", f"head={head_out}"])
    )


@pytest.fixture
def warn_slower():
    """Reports a significant slowdown below the limit that fails a gate, for a reviewer to judge."""

    def warn(row):
        warnings.warn(str(row), UserWarning, stacklevel=2)

    return warn
