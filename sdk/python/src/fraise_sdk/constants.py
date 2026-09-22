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

DEFAULT_BASE_URL = "http://localhost:9876"
DEFAULT_TIMEOUT_SECONDS = 30.0

# 204 No Content is how the server answers a recall of a graph that holds
# nothing. It is a success, not an error, and it has no body: the distinction
# between "nothing is stored here" and "nothing matched" is the status itself.
NO_CONTENT = 204

# Server versions this SDK is verified against. Keep in sync with COMPATIBILITY.md
# and bump when a release starts relying on newer server behaviour.
SUPPORTED_SERVER = ">=0.1.0,<0.2.0"
SERVER_MIN = (0, 1, 0)
SERVER_MAX_EXCLUSIVE = (0, 2, 0)

# Name bound to the out-of-band vector in the request parameters.
VECTOR_PARAM = "v"

# The highest graph a query can name. A selector travels as a uint8, so 256 does
# not fail — it wraps to graph 0 unless the server catches it, which is why this
# is a hard edge rather than a hint. Which selectors below it exist is the
# server's business, and only it can answer that.
MAX_GRAPH = 255

# The grammar's reserved words, mirroring the server's keyword table. A bare term
# spelling one of these reads as the start of a clause everywhere except the
# first term of a recall, so the builder has to know them to quote around them.
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

# The name the memory server registers under. Tool identifiers Claude sees are
# namespaced as ``mcp__<server>__<tool>``, so this drives both the mcp_servers
# key and the allowed_tools entries — keep them in sync via `allowed_tools`.
DEFAULT_SERVER_NAME = "fraise_memory"
RECALL_TOOL = "recall_memory"
REMEMBER_TOOL = "remember_fact"

# Tool-call budget: a sane ceiling so the model need not reason about scale.
# Depth has no default here: an omitted clause takes the lane the server is
# configured with, and this tool names no topic or entity, so any explicit
# lane above the floor would only draw a warning.
DEFAULT_TOP = 5

# The retrieval lanes are 0, 1 and 2 by design — the scorer runs at most one
# anchor-mediated round — so a larger depth is not a deeper search but a
# request the server rejects at parse time. The bound is stated in the schema
# and enforced before the call, so the model gets a correction it can act on
# rather than a round trip that fails. An operator can only lower the ceiling
# (max-depth), and the server's own rejection still surfaces as a tool error.
MAX_DEPTH = 2
