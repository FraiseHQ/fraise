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
"""The ``fraise-mcp`` command: configure the server and serve it over stdio.

Every setting is a flag, and each flag falls back to an environment variable,
then to a default. A harness that launches the server as a subprocess can
configure it either way. Stdout belongs to the protocol once the server runs,
so nothing on this path prints to it.
"""

from __future__ import annotations

import argparse
import os
import warnings
from collections.abc import Sequence
from importlib.metadata import version

from fraise_sdk.client import FraiseClient
from fraise_sdk.constants import DEFAULT_BASE_URL
from fraise_sdk.errors import FraiseWarning
from fraise_sdk.providers.fastembed import FastEmbedEmbedder

from fraise_mcp.server import build_server

ADDR_ENV = "FRAISE_ADDR"
MODEL_ENV = "FRAISE_EMBEDDING_MODEL"
NO_VECTORS_ENV = "FRAISE_NO_VECTORS"


def parse_args(argv: Sequence[str] | None = None) -> argparse.Namespace:
    """Parse the command line, with each flag's environment variable as its default.

    Args:
        argv: the arguments after the program name, ``None`` for
            ``sys.argv[1:]``.

    Returns:
        The settings: ``addr``, ``model`` (``None`` for the embedder's own
        default) and ``no_vectors``.
    """
    parser = argparse.ArgumentParser(
        prog="fraise-mcp",
        description="MCP server for a running Fraise daemon, over stdio.",
    )
    parser.add_argument(
        "--addr",
        default=os.environ.get(ADDR_ENV, DEFAULT_BASE_URL),
        help=f"URL of the Fraise daemon (env {ADDR_ENV}, default {DEFAULT_BASE_URL})",
    )
    parser.add_argument(
        "--model",
        default=os.environ.get(MODEL_ENV) or None,
        help=(
            f"fastembed model the statements are embedded with (env {MODEL_ENV}, "
            "default BAAI/bge-small-en-v1.5)"
        ),
    )
    parser.add_argument(
        "--no-vectors",
        action="store_true",
        default=os.environ.get(NO_VECTORS_ENV, "").lower() in {"1", "true", "yes"},
        help=f"send every statement on text alone (env {NO_VECTORS_ENV}=1)",
    )
    return parser.parse_args(argv)


def main(argv: Sequence[str] | None = None) -> None:
    """Run the server over stdio until the client disconnects.

    Args:
        argv: the arguments after the program name, ``None`` for
            ``sys.argv[1:]``.
    """
    args = parse_args(argv)
    # The server renders the daemon's warnings into the tool result, where the
    # model reads them. The SDK's own copy on stderr would only repeat them.
    warnings.filterwarnings("ignore", category=FraiseWarning)
    embedder = None
    if not args.no_vectors:
        embedder = FastEmbedEmbedder(args.model) if args.model else FastEmbedEmbedder()
    with FraiseClient(args.addr) as client:
        build_server(client, embedder, version=version("fraise-mcp")).run("stdio")
