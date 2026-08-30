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

"""Fixtures for the server + MCP bridge integration suite.

This suite exercises the one binary in both of its roles at once: the daemon
serving the HTTP query API, and ``fraise mcp`` bridging it over stdio. Both
processes are the real, freshly built binary — nothing is mocked — so what
passes here is exactly what `claude mcp add fraise -- fraise mcp` runs. Like
``tests/e2e``, this suite mirrors what it drives (``pkg/mcp``), not the SDK.

The daemon is started fresh on a free port for the session, so graph contents
are deterministic without the e2e suite's graph-allocation map; tests keep
their facts distinguishable by writing unique keywords.

Everything a test needs is a fixture here, and fixtures are the only way a
test gets it: a test module never imports from this file.
"""

import json
import os
import queue
import socket
import subprocess
import threading
import time
from pathlib import Path

import pytest
import requests

_REPO_ROOT = Path(__file__).resolve().parents[2]

_DAEMON_WAIT_SECONDS = 30
_READ_TIMEOUT_SECONDS = 15

# The protocol revision this client speaks; the server negotiates from it.
_PROTOCOL_VERSION = "2025-06-18"


def pytest_configure(config):
    # Register the marker so `-m integration` / `-m "not integration"` work
    # and pytest doesn't warn about an unknown mark.
    config.addinivalue_line(
        "markers",
        "integration: drives the built fraise binary end to end; needs Go or FRAISE_BIN",
    )


def pytest_collection_modifyitems(items):
    # Every test under this directory drives real processes, so the mark is
    # applied here, once, instead of as a line to forget in each new module.
    for item in items:
        item.add_marker(pytest.mark.integration)


def _free_port() -> int:
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


class _MCPClient:
    """A newline-delimited JSON-RPC client over a ``fraise mcp`` subprocess.

    A real client for a real server — the stdio twin of the e2e suite's HTTP
    ``query`` fixture, not a mock. Reads happen on a pumping thread feeding a
    queue, so a response that arrives buffered together with another message
    can never deadlock a timeout-guarded read.
    """

    def __init__(self, binary: str, port: int):
        self._proc = subprocess.Popen(  # noqa: S603 — the binary under test
            [binary, "mcp", "-port", str(port)],
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
        )
        self._id = 0
        self._lines: queue.Queue[str] = queue.Queue()
        self._reader = threading.Thread(target=self._pump, daemon=True)
        self._reader.start()
        # The server's initialize result, kept for assertions on identity:
        # populated by handshake(), which every fixture-provided client has run.
        self.handshake_result: dict | None = None

    def _pump(self):
        for line in self._proc.stdout:
            self._lines.put(line)

    def _send(self, message: dict):
        self._proc.stdin.write(json.dumps(message) + "\n")
        self._proc.stdin.flush()

    def _read(self) -> dict:
        try:
            return json.loads(self._lines.get(timeout=_READ_TIMEOUT_SECONDS))
        except queue.Empty:
            stderr = self._proc.stderr.read() if self._proc.poll() is not None else ""
            raise TimeoutError(
                f"no MCP message within {_READ_TIMEOUT_SECONDS}s; "
                f"bridge exit={self._proc.poll()} stderr={stderr!r}"
            ) from None

    def request(self, method: str, params: dict | None = None) -> dict:
        """Send one request and return its response.

        Interleaved messages with other ids (notifications) are skipped: a
        request's answer is matched by id, not by arrival order.

        Args:
            method: the JSON-RPC method name.
            params: the request params; an empty object when omitted.

        Returns:
            The response's ``result`` payload.

        Raises:
            AssertionError: on a protocol-level error response — a tool-level
                failure travels inside the result as ``isError``, so an error
                here means the conversation itself broke.
        """
        self._id += 1
        self._send(
            {"jsonrpc": "2.0", "id": self._id, "method": method, "params": params or {}}
        )
        while True:
            message = self._read()
            if message.get("id") != self._id:
                continue
            if "error" in message:
                raise AssertionError(
                    f"protocol error from {method}: {message['error']}"
                )
            return message["result"]

    def notify(self, method: str, params: dict | None = None):
        self._send({"jsonrpc": "2.0", "method": method, "params": params or {}})

    def handshake(self) -> dict:
        """Run initialize + initialized and return the server's initialize
        result — every conversation's mandatory opening."""
        result = self.request(
            "initialize",
            {
                "protocolVersion": _PROTOCOL_VERSION,
                "capabilities": {},
                "clientInfo": {"name": "pytest", "version": "0"},
            },
        )
        self.notify("notifications/initialized")
        self.handshake_result = result
        return result

    def call(self, name: str, arguments: dict) -> dict:
        """Call one tool and return the tools/call result payload."""
        return self.request("tools/call", {"name": name, "arguments": arguments})

    def close(self):
        try:
            self._proc.stdin.close()
            self._proc.wait(timeout=5)
        except (OSError, subprocess.TimeoutExpired):
            self._proc.kill()


@pytest.fixture(scope="session")
def fraise_binary(tmp_path_factory):
    """Path to the fraise binary: FRAISE_BIN when set (CI builds it once via
    make build-go), else a fresh `go build` into a session temp dir."""
    prebuilt = os.environ.get("FRAISE_BIN")
    if prebuilt:
        return prebuilt
    out = tmp_path_factory.mktemp("bin") / "fraise"
    subprocess.run(  # noqa: S603 — building the code under test
        ["go", "build", "-o", str(out), "./cmd/server"],
        check=True,
        cwd=_REPO_ROOT,
    )
    return str(out)


@pytest.fixture(scope="session")
def daemon_port(fraise_binary, tmp_path_factory):
    """A fresh daemon on a free port for the whole session.

    Health-checked before the first test and torn down after the last; the
    log rides in a temp file that surfaces on startup failure instead of
    scrolling by.

    Yields:
        The port the daemon listens on.

    Raises:
        RuntimeError: when the daemon exits early or never answers its
            health check, carrying the tail of its log.
    """
    port = _free_port()
    log_path = tmp_path_factory.mktemp("daemon") / "daemon.log"
    with open(log_path, "w") as log:
        proc = subprocess.Popen(  # noqa: S603 — the binary under test
            [fraise_binary, "-port", str(port)],
            stdout=log,
            stderr=subprocess.STDOUT,
        )
        deadline = time.monotonic() + _DAEMON_WAIT_SECONDS
        while True:
            try:
                if (
                    requests.get(f"http://127.0.0.1:{port}/", timeout=1).status_code
                    == 200
                ):
                    break
            except requests.RequestException:
                pass
            if time.monotonic() > deadline or proc.poll() is not None:
                proc.kill()
                raise RuntimeError(
                    f"daemon did not become healthy: {log_path.read_text()[-2000:]}"
                )
            time.sleep(0.2)
        yield port
        proc.terminate()
        proc.wait(timeout=10)


@pytest.fixture(scope="session")
def mcp(fraise_binary, daemon_port):
    """A handshaken MCP client over a bridge aimed at the live daemon.

    Yields:
        The client, initialize/initialized already exchanged.
    """
    client = _MCPClient(fraise_binary, daemon_port)
    client.handshake()
    yield client
    client.close()


@pytest.fixture
def mcp_without_daemon(fraise_binary):
    """A handshaken MCP client whose daemon port has nothing listening.

    The bridge must still converse; only tool calls may fail, in band.

    Yields:
        The client, handshaken against the daemonless bridge.
    """
    client = _MCPClient(fraise_binary, _free_port())
    client.handshake()
    yield client
    client.close()
