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
"""Constants used in this project."""

import os

DEFAULT_BASE_URL = "http://localhost:9876"
DEFAULT_TIMEOUT_SECONDS = 30.0

# The routes a query is posted to. The explain route runs a recall through the
# same pipeline as the query route and answers with each hit's contribution
# breakdown, so a read sent to either is the same query string.
QUERY_PATH = "/api/v1/q"
EXPLAIN_PATH = "/api/v1/explain"

# 204 No Content is how the server answers a recall of a graph that holds
# nothing. It is a success, not an error, and it has no body: the distinction
# between "nothing is stored here" and "nothing matched" is the status itself.
NO_CONTENT = 204

# Server versions this SDK is verified against. Keep in sync with COMPATIBILITY.md
# and bump when a release starts relying on newer server behaviour.
SUPPORTED_SERVER = ">=0.2.0,<0.3.0"
SERVER_MIN = (0, 2, 0)
SERVER_MAX_EXCLUSIVE = (0, 3, 0)

# Name bound to the out-of-band vector in the request parameters.
VECTOR_PARAM = "v"

# The highest graph a query can name: the selector is a uint8, so the builders
# refuse a larger one before anything is sent. Which graphs up to it exist
# depends on the server's num-graphs setting, and only the server checks that.
MAX_GRAPH = 255

# The grammar's reserved words, mirroring the server's keyword table. A bare term
# spelling one of these is syntax wherever a term stands, so the builder has to
# know them to quote around them.
KEYWORDS = frozenset(
    {
        "recall",
        "remember",
        "forget",
        "update",
        "topic",
        "entity",
        "since",
        "until",
        "top",
        "depth",
        "vec",
    }
)

# The Claude integration's server and tool names. Claude sees a tool as
# ``mcp__<server>__<tool>``, so the mcp_servers key and the allowed_tools
# entries must name the same server; `allowed_tools` builds the entries from
# these names.
DEFAULT_SERVER_NAME = "fraise_memory"
RECALL_TOOL = "recall_memory"
REMEMBER_TOOL = "remember_fact"

# The recall tool's default top, so the model need not choose one. There is no
# default depth: an omitted clause takes the lane the server is configured
# with, and the tool names no topic or entity, so a lane above 0 would only
# draw a warning.
DEFAULT_TOP = 5

# The retrieval lanes are 0, 1 and 2: a search runs at most one
# anchor-mediated round, so the server rejects a larger depth at parse time.
# The tool's schema states the bound and the tool checks it before calling, so
# the model gets a correction it can act on rather than a failed round trip.
# An operator can only lower the ceiling (max-depth), and the server's own
# rejection still surfaces as a tool error.
MAX_DEPTH = 2

# The SDK's own files. A warning is attributed to the first frame outside them,
# the caller's line, however deep in the SDK it was raised; a fixed stacklevel
# is right for one call path only.
SDK_FILES = (os.path.dirname(os.path.abspath(__file__)) + os.sep,)
