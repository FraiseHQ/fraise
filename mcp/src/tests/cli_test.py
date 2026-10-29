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
"""The fraise-mcp command: its settings, its wiring, and the server it runs.

The integration half drives the installed command against a live daemon, one
subprocess per call, so each call restarts the server.
"""

from unittest.mock import patch

import pytest
from fraise_mcp.cli import main, parse_args


@pytest.mark.parametrize(
    "env, argv, want",
    [
        ({}, [], ("http://localhost:9876", None, False)),
        (
            {
                "FRAISE_ADDR": "http://10.0.0.5:9876",
                "FRAISE_EMBEDDING_MODEL": "BAAI/bge-base-en-v1.5",
                "FRAISE_NO_VECTORS": "1",
            },
            [],
            ("http://10.0.0.5:9876", "BAAI/bge-base-en-v1.5", True),
        ),
        (
            {"FRAISE_ADDR": "http://10.0.0.5:9876", "FRAISE_EMBEDDING_MODEL": "a/b"},
            ["--addr", "http://fraise:9876", "--model", "c/d", "--no-vectors"],
            ("http://fraise:9876", "c/d", True),
        ),
        ({"FRAISE_NO_VECTORS": "0"}, [], ("http://localhost:9876", None, False)),
    ],
)
def test_a_flag_wins_over_its_environment_variable_and_both_over_the_default(
    env, argv, want
):
    """A harness can configure the server through its command line or its environment."""
    with patch.dict("os.environ", env, clear=True):
        args = parse_args(argv)
    assert (args.addr, args.model, args.no_vectors) == want


@pytest.mark.parametrize(
    "argv, model",
    [([], None), (["--model", "BAAI/bge-base-en-v1.5"], "BAAI/bge-base-en-v1.5")],
)
def test_main_serves_over_stdio_with_the_configured_embedder(argv, model):
    """The embedder is built with the chosen model, or the provider's own default."""
    with (
        patch.dict("os.environ", {}, clear=True),
        patch("fraise_mcp.cli.FraiseClient") as client,
        patch("fraise_mcp.cli.FastEmbedEmbedder") as embedder,
        patch("fraise_mcp.cli.build_server") as build,
    ):
        main(["--addr", "http://fraise:9876", *argv])
    client.assert_called_once_with("http://fraise:9876")
    embedder.assert_called_once_with(*([model] if model else []))
    build.assert_called_once()
    assert build.call_args.args == (
        client.return_value.__enter__.return_value,
        embedder.return_value,
    )
    build.return_value.run.assert_called_once_with("stdio")


def test_main_without_vectors_builds_no_embedder():
    """--no-vectors leaves the model unloaded and every statement as written."""
    with (
        patch("fraise_mcp.cli.FraiseClient"),
        patch("fraise_mcp.cli.FastEmbedEmbedder") as embedder,
        patch("fraise_mcp.cli.build_server") as build,
    ):
        main(["--no-vectors"])
    embedder.assert_not_called()
    assert build.call_args.args[1] is None


@pytest.mark.integration
@pytest.mark.parametrize(
    "flags, graph, want",
    [
        ((), "vector_graph", "- my favourite colour is teal (relevance"),
        (("--no-vectors",), "control_graph", "No stored facts matched."),
    ],
)
def test_a_fact_remembered_before_a_restart_is_recalled_by_meaning_after_it(
    call_command, spelling_pair, request, flags, graph, want
):
    """Remember, restart the server, recall: the vector finds a spelling the text misses.

    The fact says "favourite colour" and the question "favorite color", which
    share no stem, so the text index alone cannot match them. With vectors the
    fact comes back. The control run, on a graph that never took a vector,
    finds nothing. So the hit is the local embedding's doing, and it survives
    a restart because the daemon, not the server, holds the memory.
    """
    fact, question = spelling_pair
    graph_id = request.getfixturevalue(graph)
    remembered = call_command("remember", f"remember@{graph_id} '{fact}'", *flags)
    assert remembered.content[0].text == "Remembered."
    got = call_command("recall", f"recall@{graph_id} {question}", *flags)
    assert not got.is_error
    assert got.content[0].text.startswith(want)
