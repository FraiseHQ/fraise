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

"""A Claude Agent SDK agent that uses Fraise for long-term memory.

Fraise's memory tools are exposed as an in-process MCP server. The two turns run
as *separate* client sessions with no shared history, so the second turn can only
answer by recalling what the first turn stored in Fraise.

Environment:
    FRAISE_URL          base URL of the Fraise server (default http://localhost:9876)
    ANTHROPIC_API_KEY   required by the Claude Agent SDK

The Claude Agent SDK drives Claude through the Claude Code CLI, so the `claude`
binary must be on PATH (the Docker image installs it). Run with Docker:
    ANTHROPIC_API_KEY=sk-ant-... docker compose run --rm agent
"""

import asyncio
import os

from claude_agent_sdk import (
    AssistantMessage,
    ClaudeAgentOptions,
    ClaudeSDKClient,
    TextBlock,
)

from fraise_sdk import FraiseClient
from fraise_sdk.integrations.claude_agents import allowed_tools, memory_server


async def ask(options: ClaudeAgentOptions, prompt: str) -> None:
    """Run a single-turn Claude session and print its text output."""
    async with ClaudeSDKClient(options=options) as client:
        await client.query(prompt)
        async for message in client.receive_response():
            if isinstance(message, AssistantMessage):
                for block in message.content:
                    if isinstance(block, TextBlock):
                        print("assistant:", block.text)


async def main() -> None:
    fraise = FraiseClient(os.environ.get("FRAISE_URL", "http://localhost:9876"))

    # This example is keyword-only. To vectorise, pass an embedder to
    # memory_server(fraise, embedder=...). Anthropic has no embeddings API, so
    # you'd use OpenAIEmbedder (from fraise_sdk.providers) with an OPENAI_API_KEY
    # set alongside ANTHROPIC_API_KEY.
    options = ClaudeAgentOptions(
        system_prompt=(
            "You have a long-term memory. When the user shares a durable fact "
            "about themselves, store it with the remember tool. When answering a "
            "question, first recall relevant facts from memory."
        ),
        mcp_servers={"fraise_memory": memory_server(fraise)},
        allowed_tools=allowed_tools(),
    )

    print("turn 1 > My favourite colour is orange.")
    await ask(options, "My favourite colour is orange.")

    # Fresh session — no history carried over — so memory is the only source.
    print("\nturn 2 > What is my favourite colour?")
    await ask(options, "What is my favourite colour?")


if __name__ == "__main__":
    asyncio.run(main())
