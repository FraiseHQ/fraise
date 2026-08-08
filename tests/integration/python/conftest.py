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

"""Fixtures for the Python SDK integration suite.

Exercises the real ``fraise_sdk`` client against a live server named by
FRAISE_URL — inside the docker compose network that is http://fraise:9876 —
waiting for its health check before any test runs.
"""

import os
import time

import pytest
from fraise_sdk.client import FraiseClient

FRAISE_URL = os.environ.get("FRAISE_URL", "http://localhost:9876")
WAIT_TIMEOUT_SECONDS = 30


@pytest.fixture(scope="session")
def client():
    """A FraiseClient pointed at a server confirmed to be up.

    Yields:
            FraiseClient: fraise client.
    """
    fraise = FraiseClient(FRAISE_URL)
    deadline = time.monotonic() + WAIT_TIMEOUT_SECONDS
    while time.monotonic() < deadline:
        if fraise.health():
            break
        time.sleep(0.5)
    else:
        fraise.close()
        pytest.fail(f"fraise server not reachable at {FRAISE_URL}")
    yield fraise
    fraise.close()
