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
runs them on the working tree and hands the test one row per benchmark and
unit of benchstat's comparison with the baseline, which the test asserts its
limits on. The baseline is the newest nightly run on main, whose outputs CI
downloads into ``--bench-baseline``; every run leaves its own in
``--bench-out`` under the same names, which is how a nightly run becomes the
next baseline. Without a baseline for a gate there is nothing to compare
against yet: the benchmarks still run and are reported, and the gate passes.
Gates sharing a marker share one measurement.

A pull request labelled perf-regression-accepted with a ``Perf:`` line in its
description marks every gate xfail with that line as the reason: its
regressions still show, as accepted. The session ends by writing report.md,
the verdict and benchstat's tables, which CI posts.
"""

import csv
import io
import json
import os
import re
import subprocess
import warnings
from dataclasses import dataclass
from pathlib import Path

import pytest

_HERE = Path(__file__).resolve().parent
_ROOT = _HERE.parents[1]
_BENCHSTAT = [
    "go",
    "run",
    "golang.org/x/perf/cmd/benchstat@v0.0.0-20260929162123-406019bb8b68",
]
_ACCEPT_LABEL = "perf-regression-accepted"
_RUNS = pytest.StashKey[dict]()
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


def _quantity(value, unit):
    """A value in the unit benchstat measured it in, scaled the way benchstat's tables print it."""
    scales = {
        "sec": [(1e-9, "ns"), (1e-6, "µs"), (1e-3, "ms"), (1, "s")],
        "B": [(1, "B"), (2**10, "KiB"), (2**20, "MiB"), (2**30, "GiB")],
    }
    base = next(
        (b for b in scales if unit == f"{b}/op" or unit.endswith(f"-{b}")), None
    )
    if base is None or value == 0:
        return f"{value:.4g}"
    factor, suffix = next(
        ((f, s) for f, s in reversed(scales[base]) if abs(value) >= f), scales[base][0]
    )
    return f"{value / factor:.4g} {suffix}"


def _single(text):
    rows, unit = [], ""
    for record in csv.reader(io.StringIO(text)):
        if len(record) != 3:
            continue
        name, value, ci = record
        if not name:
            unit = value if ci == "CI" else unit
        elif name != "geomean" and value:
            rows.append((re.sub(r"-\d+$", "", name), unit, float(value)))
    return rows


def _table(base, head):
    """benchstat's comparison of base with head as a markdown table, or head's own numbers when there is no base."""
    if base is None:
        lines = ["| benchmark | unit | this run |", "|---|---|---:|"]
        csv_text = _run([*_BENCHSTAT, "-format", "csv", str(head)])
        return lines + [
            f"| {name} | {unit} | {_quantity(value, unit)} |"
            for name, unit, value in _single(csv_text)
        ]
    lines = [
        "| benchmark | unit | baseline | this run | vs base | |",
        "|---|---|---:|---:|---:|---|",
    ]
    for row in _rows(
        _run([*_BENCHSTAT, "-format", "csv", f"base={base}", f"head={head}"])
    ):
        test = "exact" if row.p is None else f"p={row.p:.3f}"
        delta = f"**{row.delta}**" if row.significant else row.delta
        lines.append(
            f"| {row.benchmark} | {row.unit} | {_quantity(row.base, row.unit)} | {_quantity(row.head, row.unit)} | {delta} | {test} |"
        )
    return lines


def _measured(path):
    return path.is_file() and "\nBenchmark" in path.read_text()


def pytest_addoption(parser):
    group = parser.getgroup("benchmark gates")
    group.addoption(
        "--bench-baseline",
        help="a nightly run's outputs to compare against; a gate without one there only measures",
    )
    group.addoption(
        "--bench-out",
        default=str(_ROOT / "bin" / "perf"),
        help="where this run's outputs and report.md go",
    )


def pytest_configure(config):
    config.addinivalue_line(
        "markers",
        "bench(package, pattern, count=10, benchtime=None, data=None): a gate on the Go benchmarks "
        "matching pattern in package, run count times; data maps environment variables to files "
        "under tests/perf the benchmarks read",
    )
    config.addinivalue_line(
        "markers", "nightly: a gate too slow to run on every pull request"
    )
    config.stash[_RUNS] = {}
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
    runs = session.config.stash[_RUNS]
    if not runs:
        return
    out = Path(session.config.getoption("bench_out")).resolve()
    baseline = session.config.getoption("bench_baseline")
    against = "the nightly baseline" if baseline else "no baseline yet"
    verdict = "🔴 Benchmarks regressed" if exitstatus != 0 else "🟢 No regression"
    lines = [f"### {verdict} against {against}", ""]
    for kind in ("Failed", "Accepted"):
        listed = [
            f"- `{nodeid}`: {message}"
            for k, nodeid, message in session.config.stash[_OUTCOMES]
            if k == kind
        ]
        if listed:
            lines += [f"**{kind}**", "", *listed, ""]
    for name, (base, head) in runs.items():
        summary = name if base else f"{name} (no baseline yet)"
        lines += [
            f"<details><summary>{summary}</summary>",
            "",
            *_table(base, head),
            "",
            "</details>",
            "",
        ]
    (out / "report.md").write_text("\n".join(lines))


@pytest.fixture
def comparison(request, pytestconfig):
    """The marked benchmarks run on the working tree and compared with the baseline, one row per benchmark and unit.

    With no baseline for them yet the rows are empty: there is nothing to
    regress from, so every limit holds, and the report shows what was
    measured.
    """
    marker = request.node.get_closest_marker("bench")
    package, pattern = marker.args
    name = f"{package} {pattern}"
    runs = pytestconfig.stash[_RUNS]
    if name not in runs:
        slug = re.sub(r"\W+", "-", name).strip("-") + ".txt"
        out = Path(pytestconfig.getoption("bench_out")).resolve()
        out.mkdir(parents=True, exist_ok=True)
        cmd = [
            *["go", "test", package, "-run", "^$", "-bench", pattern],
            "-count",
            str(marker.kwargs.get("count", 10)),
        ]
        cmd += ["-cpu", "4", "-benchmem", "-timeout", "2h"]
        if marker.kwargs.get("benchtime"):
            cmd += ["-benchtime", marker.kwargs["benchtime"]]
        data = {
            var: str(_HERE / path)
            for var, path in (marker.kwargs.get("data") or {}).items()
        }
        (out / slug).write_text(_run(cmd, env={**os.environ, **data}))
        baseline = pytestconfig.getoption("bench_baseline")
        base = Path(baseline).resolve() / slug if baseline else None
        runs[name] = (base if base and _measured(base) else None, out / slug)
    base, head = runs[name]
    if base is None:
        return []
    return _rows(_run([*_BENCHSTAT, "-format", "csv", f"base={base}", f"head={head}"]))


@pytest.fixture
def warn_slower():
    """Reports a significant slowdown for a reviewer to judge, without failing the gate."""

    def warn(row):
        warnings.warn(str(row), UserWarning, stacklevel=2)

    return warn
