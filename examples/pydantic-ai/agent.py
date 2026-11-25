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

"""A Pydantic AI agent that uses Fraise for long-term memory.

The memory tools come from ``fraise_sdk.integrations.pydantic_ai`` and go into
the agent's ``tools=``, so the agent decides what to store and when to recall
it.

The two turns run as *separate* agent runs with no shared message history. So
the second turn can only answer by recalling what the first turn stored in
Fraise. The same two turns then run again on an identical agent without the
tools, as a control: that agent cannot answer, which shows the answer came
from Fraise and not from the context. The script exits non-zero when either
run turns out otherwise.

Environment:
    FRAISE_URL       base URL of the Fraise server (default http://localhost:9876)
    OPENAI_API_KEY   required by the OpenAI model the agent runs on

Run it with Docker (brings up Fraise too):
    OPENAI_API_KEY=sk-... docker compose run --rm agent
"""

import os
import sys

from fraise_sdk import FraiseClient
from fraise_sdk.integrations.pydantic_ai import memory_tools
from pydantic_ai import Agent

FACT = "My cat is called Tartine."
QUESTION = "What is my cat called?"
ANSWER = "tartine"


def ask(agent: Agent, prompt: str) -> str:
    """Run one turn as a fresh run, with no message history, and print it."""
    print(f"> {prompt}")
    answer = agent.run_sync(prompt).output
    print("assistant:", answer)
    return answer


def main() -> None:
    """Run the two turns with Fraise, then again without it."""
    fraise = FraiseClient(os.environ.get("FRAISE_URL", "http://localhost:9876"))

    instructions = (
        "You have a long-term memory. When the user shares a durable fact about "
        "themselves, store it with the remember tool. When answering a question, "
        "first recall relevant facts from memory."
    )
    # This example is keyword-only. To vectorise, pass an embedder to
    # memory_tools(fraise, embedder=...), such as OpenAIEmbedder from
    # fraise_sdk.providers.openai, which needs the openai extra.
    with_memory = Agent(
        "openai:gpt-5-nano", instructions=instructions, tools=memory_tools(fraise)
    )
    without_memory = Agent("openai:gpt-5-nano", instructions=instructions)

    print("with Fraise")
    ask(with_memory, FACT)
    recalled = ask(with_memory, QUESTION)

    print("\ncontrol, without Fraise")
    ask(without_memory, FACT)
    guessed = ask(without_memory, QUESTION)

    if ANSWER not in recalled.lower():
        sys.exit("the agent with Fraise did not recall the fact")
    if ANSWER in guessed.lower():
        sys.exit("the agent without Fraise answered anyway, so memory proves nothing")


if __name__ == "__main__":
    main()
