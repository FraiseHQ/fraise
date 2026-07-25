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

"""Shared fixtures for the Fraise end-to-end suite.

The suite targets the server named by FRAISE_URL — inside the docker compose
network that is http://fraise:9876 — and waits for its health check before
any test runs.
"""

import os
import time

import pytest
import requests

BASE_URL = os.environ.get("FRAISE_URL", "http://localhost:9876").rstrip("/")

WAIT_TIMEOUT_SECONDS = 30
REQUEST_TIMEOUT_SECONDS = 10


@pytest.fixture(scope="session")
def base_url():
    """Base URL of a Fraise server that is confirmed to be up."""
    deadline = time.monotonic() + WAIT_TIMEOUT_SECONDS
    last_error = None
    while time.monotonic() < deadline:
        try:
            response = requests.get(f"{BASE_URL}/", timeout=2)
            if response.status_code == 200:
                return BASE_URL
            last_error = f"health check returned {response.status_code}"
        except requests.RequestException as exc:
            last_error = str(exc)
        time.sleep(0.5)
    pytest.fail(f"fraise server not reachable at {BASE_URL}: {last_error}")


@pytest.fixture(scope="session")
def query(base_url):
    """Callable posting a raw query string to /api/v1/q.

    Returns (status_code, decoded JSON body).
    """

    def _query(text: str):
        response = requests.post(
            f"{base_url}/api/v1/q",
            json={"query": text},
            timeout=REQUEST_TIMEOUT_SECONDS,
        )
        return response.status_code, response.json()

    return _query
