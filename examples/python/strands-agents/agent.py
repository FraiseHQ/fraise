# MIT License
#
# Copyright (c) 2026 René-Jean Corneille
#
# Permission is hereby granted, free of charge, to any person obtaining a copy
# of this software and associated documentation files (the "Software"), to deal
# in the Software without restriction, including without limitation the rights
# to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
# copies of the Software, and to permit persons to whom the Software is
# furnished to do so, subject to the following conditions:
#
# The above copyright notice and this permission notice shall be included in all
# copies or substantial portions of the Software.
#
# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
# FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
# AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
# LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
# OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
# SOFTWARE.

"""A Strands Agents agent that uses Fraise through its MCP bridge.

The two memory turns use separate Agent instances with no shared history. The
second turn can therefore answer only by calling Fraise's recall tool.

Environment:
    FRAISE_MCP_ADDR: daemon address used by ``fraise mcp`` (default:
        http://localhost:9876)
    CONTROL_RUN: set to 1 to run the no-memory control turn after the memory
        demo
    AWS_REGION and AWS credentials: used by Strands' default model
"""

from __future__ import annotations

import os

from mcp import StdioServerParameters, stdio_client
from strands import Agent
from strands.tools.mcp import MCPClient

SYSTEM_PROMPT = (
    "You have long-term memory through the Fraise tools. When the user shares "
    "a durable fact, store it with remember. Before answering a question that "
    "depends on an earlier turn, call recall. If you cannot confirm a fact "
    "from memory, say that you do not know it."
)


def make_client() -> MCPClient:
    """Create a Strands MCP client for the Fraise stdio bridge."""
    address = os.environ.get("FRAISE_MCP_ADDR", "http://localhost:9876")
    return MCPClient(
        lambda: stdio_client(
            StdioServerParameters(
                command="fraise",
                args=["mcp", "-addr", address],
            )
        )
    )


def ask(tools: list[object], prompt: str) -> None:
    """Run one fresh Strands Agent session and print its response."""
    agent = Agent(tools=tools, system_prompt=SYSTEM_PROMPT)
    print(f"user: {prompt}")
    print(f"assistant: {agent(prompt)}")


def main() -> None:
    """Run the two-turn memory demo and an optional no-memory control."""
    with make_client() as client:
        tools = list(client.list_tools_sync())
        print("registered Fraise tools:", ", ".join(tool.tool_name for tool in tools))

        # Fresh Agent objects keep the conversation history out of turn 2.
        ask(tools, "My favourite colour is orange. Please remember this.")
        ask(tools, "What is my favourite colour? Use memory before answering.")

    if os.environ.get("CONTROL_RUN") == "1":
        print("\ncontrol (Fraise disabled)")
        ask([], "What is my favourite colour? If you cannot verify it, say unknown.")


if __name__ == "__main__":
    main()
