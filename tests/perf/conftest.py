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
A baseline measured on another machine is one benchstat does not compare
with: the gate could not measure, and the report names both machines and
shows this run's numbers.
Gates sharing a marker share one measurement.

A pull request labelled perf-regression-accepted marks every gate xfail: its
regressions still show, as accepted. A gate fails through ``fail_regressed``
with the rows that broke its limit, so the report shows them as a table. The
session ends by writing report.md, the verdict, those tables and benchstat's,
which CI posts.
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
_REGRESSED = pytest.StashKey[list]()


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
        # A count is shown whole: 19,640 allocations, not 1.964e+04.
        return f"{value:,.0f}" if value.is_integer() else f"{value:.4g}"
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


def _compared(rows):
    """Rows of benchstat's comparison as a markdown table, a significant change in bold."""
    lines = [
        "| benchmark | unit | baseline | this run | vs base | |",
        "|---|---|---:|---:|---:|---|",
    ]
    for row in rows:
        test = "exact" if row.p is None else f"p={row.p:.3f}"
        delta = f"**{row.delta}**" if row.significant else row.delta
        lines.append(
            f"| {row.benchmark} | {row.unit} | {_quantity(row.base, row.unit)} | {_quantity(row.head, row.unit)} | {delta} | {test} |"
        )
    return lines


def _table(base, head):
    """benchstat's comparison of base with head as a markdown table, or head's own numbers when nothing of it compares."""
    rows = (
        _rows(_run([*_BENCHSTAT, "-format", "csv", f"base={base}", f"head={head}"]))
        if base
        else []
    )
    if not rows:
        lines = ["| benchmark | unit | this run |", "|---|---|---:|"]
        csv_text = _run([*_BENCHSTAT, "-format", "csv", str(head)])
        return lines + [
            f"| {name} | {unit} | {_quantity(value, unit)} |"
            for name, unit, value in _single(csv_text)
        ]
    return _compared(rows)


def _measured(path):
    return path.is_file() and "\nBenchmark" in path.read_text()


def _machine(path):
    """The machine a benchmark output was measured on, as its goos, goarch and cpu lines name it.

    benchstat compares only outputs whose lines agree, so a baseline measured
    on another machine has nothing in common with this run.
    """
    lines = dict(re.findall(r"^(goos|goarch|cpu): *(.*?) *$", path.read_text(), re.M))
    machine = f"{lines.get('goos')}/{lines.get('goarch')}"
    return f"{machine} ({lines['cpu']})" if lines.get("cpu") else machine


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
    group.addoption(
        "--bench-changed-since",
        help="a git ref: run only the gates whose packages changed since it; without one every gate runs",
    )
    group.addoption(
        "--bench-always",
        action="append",
        default=[],
        help="a path, or a directory ending in /, whose change since --bench-changed-since runs every gate; repeatable",
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


def _benchmarked(path):
    """Whether a changed .go file is code a benchmark runs: package source, or a test file that defines benchmarks."""
    if not path.endswith("_test.go"):
        return True
    file = _ROOT / path
    return file.is_file() and "\nfunc Benchmark" in file.read_text()


def _unchanged(items, since, always):
    """The gates none of whose benchmarks' packages changed since the ref, with why.

    A gate's packages are its package and every package of this module it
    builds on, its tests' imports included (``go list -deps -test``): a
    benchmark measures all the code it runs, so a change anywhere in that set
    is a change it can see, and a change outside it is one it cannot. A test
    file counts only when it defines benchmarks, and the gates and what runs
    them not at all: neither changes a number a benchmark measures, and the
    release gate runs every gate whatever changed. What a benchmark runs or
    reads that no package's source shows is in always, which the gate
    workflow names: a change to one of those runs every gate.
    """
    changed = _run(["git", "diff", "--name-only", since, "HEAD"]).split()
    if any(path.startswith(tuple(always)) for path in changed):
        return {}
    dirs = {
        str((_ROOT / path).parent.resolve())
        for path in changed
        if path.endswith(".go") and _benchmarked(path)
    }
    packages, unchanged = {}, {}
    for item in items:
        marker = item.get_closest_marker("bench")
        if not marker:
            continue
        package = marker.args[0]
        if package not in packages:
            listing = _run(
                [
                    "go",
                    "list",
                    "-deps",
                    "-test",
                    "-f",
                    "{{if .Module}}{{if .Module.Main}}{{.Dir}}{{end}}{{end}}",
                    package,
                ]
            )
            packages[package] = {str(Path(d).resolve()) for d in listing.split()}
        if not packages[package] & dirs:
            unchanged[item] = (
                f"no change since {since} in {package} or the packages it builds on"
            )
    return unchanged


def pytest_collection_modifyitems(config, items):
    since = config.getoption("bench_changed_since")
    if since:
        always = config.getoption("bench_always")
        for item, reason in _unchanged(items, since, always).items():
            item.add_marker(pytest.mark.skip(reason=reason))

    path = os.environ.get("GITHUB_EVENT_PATH")
    pr = json.loads(Path(path).read_text()).get("pull_request") if path else None
    if not pr or _ACCEPT_LABEL not in {label["name"] for label in pr.get("labels", [])}:
        return
    for item in items:
        if item.get_closest_marker("bench"):
            item.add_marker(pytest.mark.xfail(reason=_ACCEPT_LABEL, strict=False))


@pytest.hookimpl(wrapper=True)
def pytest_runtest_makereport(item, call):
    rep = yield
    # One outcome per gate: its call, or the setup that never got that far.
    if item.get_closest_marker("bench") and (
        rep.when == "call" or (rep.when == "setup" and not rep.passed)
    ):
        outcomes = item.config.stash[_OUTCOMES]
        regressed = item.stash.get(_REGRESSED, [])
        if hasattr(rep, "wasxfail") and rep.skipped:
            outcomes.append(("Accepted", item, rep.wasxfail, regressed))
        elif rep.skipped and isinstance(rep.longrepr, tuple):
            reason = rep.longrepr[2].removeprefix("Skipped: ")
            outcomes.append(("Not run", item, reason, []))
        elif rep.failed:
            # Failing at its call is a limit broken; failing at setup is a
            # run that never got its numbers (a build, a benchmark that
            # crashed), which says nothing about a regression.
            kind = "Failed" if rep.when == "call" else "Could not measure"
            message = call.excinfo.exconly()[:1000] if call.excinfo else ""
            outcomes.append((kind, item, message, regressed))
    return rep


def _set(name):
    """A benchmark set as the report names it: its package and what of it runs."""
    package, pattern = name.split(" ", 1)
    what = "all benchmarks" if pattern == "Benchmark" else f"<code>{pattern}</code>"
    return f"<b>{package.removeprefix('./')}</b> · {what}"


def _gate(item):
    """A gate as the report names it: its package, and its test read as a sentence."""
    words = item.originalname.removeprefix("test_").replace("_", " ")
    param = f" ({item.callspec.id})" if hasattr(item, "callspec") else ""
    return f"**{item.get_closest_marker('bench').args[0].removeprefix('./')}** · {words}{param}"


def _gates(outcomes):
    """Each gate's outcome as a paragraph naming it, then the rows that broke its limit as a table.

    A gate with no rows to show, one that failed on something other than its limit, gives
    its message instead. A slowdown is reported row by row, and a gate's rows
    share one table under its package alone: the heading already says what
    the gate checks.
    """
    grouped = {}
    for kind, item, message, rows in outcomes:
        _, message_, rows_ = grouped.get(item, (kind, message, []))
        grouped[item] = (kind, message_, rows_ + rows)
    lines = []
    for item, (kind, message, rows) in grouped.items():
        if rows:
            package = item.get_closest_marker("bench").args[0].removeprefix("./")
            name = f"**{package}**" if kind == "Slower" else _gate(item)
            lines += [name, "", *_compared(rows), ""]
        else:
            lines += [f"- {_gate(item)}: {message}", ""]
    return lines


def pytest_sessionfinish(session, exitstatus):
    runs = session.config.stash[_RUNS]
    outcomes = session.config.stash[_OUTCOMES]
    # A session that selected no gate has nothing to report; one whose gates
    # were all skipped still reports, so the comment on the pull request says
    # why nothing was measured rather than keeping an older run's numbers.
    if not runs and not outcomes:
        return
    out = Path(session.config.getoption("bench_out")).resolve()
    out.mkdir(parents=True, exist_ok=True)

    elsewhere = {
        name: (_machine(base), _machine(head))
        for name, (base, head) in runs.items()
        if base and _machine(base) != _machine(head)
    }
    compared = [
        name for name, (base, _) in runs.items() if base and name not in elsewhere
    ]
    kinds = {kind for kind, _, _, _ in outcomes}
    # The headline is what the outcomes add up to, worst first: a broken
    # limit, then a gate that could not measure, then a regression accepted.
    if "Failed" in kinds:
        headline = "### 🔴 Regression"
    elif "Could not measure" in kinds:
        headline = "### ⚠️ Could not measure"
    elif not runs:
        headline = "### ⚪ Nothing to measure"
    elif "Accepted" in kinds:
        headline = "### 🟡 Regression accepted"
    else:
        headline = "### 🟢 No regression"
    lines = [headline, ""]
    if not runs and "Could not measure" not in kinds:
        lines += ["No package a gate benchmarks changed.", ""]
    elif elsewhere:
        before, after = next(iter(elsewhere.values()))
        lines += [
            f"The newest nightly run of `main` ran on {before} and this run on {after}, and benchstat compares only runs on one machine, so these are this run's numbers.",
            "",
        ]
    elif len(compared) == len(runs) and runs:
        lines += ["Compared with the newest nightly run of `main`.", ""]
    elif compared:
        lines += [
            "Compared with the newest nightly run of `main`; sets marked *new* have no baseline yet and show this run's numbers.",
            "",
        ]
    elif runs:
        lines += [
            "No nightly baseline yet, so these are this run's numbers. Comparisons start once a nightly run has measured `main`.",
            "",
        ]

    for kind in ("Failed", "Could not measure", "Accepted", "Slower"):
        # A gate whose baseline ran on another machine could not measure for
        # the reason the sentence above gives once for all of them.
        listed = [
            outcome
            for outcome in outcomes
            if outcome[0] == kind
            and not (
                kind == "Could not measure"
                and " ".join(outcome[1].get_closest_marker("bench").args) in elsewhere
            )
        ]
        if not listed:
            continue
        lines += [f"#### {kind}", ""]
        if kind == "Slower":
            lines += [
                "By 10% or more and significant. Not a failure: the baseline was timed on another runner.",
                "",
            ]
        lines += _gates(listed)
        # A broken limit says how to accept it, so the way out is on the
        # pull request that needs it rather than only in the docs.
        if kind == "Failed":
            lines += [
                "> [!TIP]",
                f"> To accept a regression worth what it costs, label the pull request `{_ACCEPT_LABEL}`. See [Accepting a regression](https://docs.getfraise.dev/docs/operations/benchmarks#accepting-a-regression).",
                "",
            ]
    skipped = sorted(
        {
            item.get_closest_marker("bench").args[0].removeprefix("./")
            for k, item, _, _ in outcomes
            if k == "Not run"
        }
    )
    if skipped:
        lines += [
            f"Not run, nothing they build on changed: {', '.join(f'`{p}`' for p in skipped)}.",
            "",
        ]

    for name, (base, head) in runs.items():
        new = " · <i>new</i>" if not base and compared else ""
        lines += [
            f"<details><summary>{_set(name)}{new}</summary>",
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
    measured. With a baseline the rows are never empty: a comparison that
    matched nothing would hold every limit without checking one, so a
    baseline measured on another machine, which benchstat does not compare
    with this run, fails the gate as a run that could not measure, and so
    does one that has none of this run's benchmarks.
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
    if _machine(base) != _machine(head):
        pytest.fail(
            f"the baseline ran on {_machine(base)}, this run on {_machine(head)}",
            pytrace=False,
        )
    rows = _rows(_run([*_BENCHSTAT, "-format", "csv", f"base={base}", f"head={head}"]))
    if not rows:
        pytest.fail("none of this run's benchmarks is in the baseline", pytrace=False)
    return rows


@pytest.fixture
def fail_regressed(request):
    """Fails the gate on the rows that broke its limit; with none, the gate passes.

    The rows go into the report as a table under Failed, or under Accepted
    when the pull request accepts its regressions, so a reviewer reads each
    benchmark's baseline and change rather than an assertion's repr.
    """

    def fail(rows):
        if rows:
            request.node.stash[_REGRESSED] = list(rows)
            pytest.fail("\n".join(map(str, rows)), pytrace=False)

    return fail


@pytest.fixture
def warn_slower(request, pytestconfig):
    """Reports a significant slowdown for a reviewer to judge, without failing the gate.

    The slowdown goes into the report under Slower, where the pull request and
    Discord see it, as well as into pytest's warnings summary.
    """

    def warn(row):
        warnings.warn(str(row), UserWarning, stacklevel=2)
        pytestconfig.stash[_OUTCOMES].append(("Slower", request.node, "", [row]))

    return warn
