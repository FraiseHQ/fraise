/*
MIT License

Copyright (c) 2026 René-Jean Corneille

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

/**
 * An OpenAI Agents (TypeScript) agent that uses Fraise for long-term memory.
 *
 * Two turns run as separate agent runs with no shared history, so the second
 * turn can only answer by recalling what the first turn stored in Fraise.
 *
 * Environment:
 *   FRAISE_URL       base URL of the Fraise server (default http://localhost:9876)
 *   OPENAI_API_KEY   required by the OpenAI Agents SDK
 *
 * Run it with Docker (brings up Fraise too):
 *   OPENAI_API_KEY=sk-... docker compose run --rm agent
 */

import { Agent, run } from "@openai/agents";
import { FraiseClient } from "fraise-sdk";
import { memoryTools } from "fraise-sdk/integrations/openai-agents";
import { OpenAIEmbedder } from "fraise-sdk/providers";

async function main(): Promise<void> {
  const fraise = new FraiseClient({
    baseUrl: process.env["FRAISE_URL"] ?? "http://localhost:9876",
  });

  // Passing an embedder makes the memory tools vectorise implicitly: remember
  // stores each fact with its embedding and recall searches by vector too.
  // Drop the `embedder` option for plain keyword memory.
  const embedder = new OpenAIEmbedder({ dimensions: 256 });

  const agent = new Agent({
    name: "Assistant",
    instructions:
      "You have a long-term memory. When the user shares a durable fact about themselves, " +
      "store it with the remember tool. When answering a question, first recall relevant " +
      "facts from memory.",
    model: "gpt-5-nano",
    tools: memoryTools(fraise, { embedder }),
  });

  console.log("turn 1 > My favourite colour is orange.");
  const first = await run(agent, "My favourite colour is orange.");
  console.log("assistant:", first.finalOutput);

  // Fresh run — no chat history carried over — so memory is the only source.
  console.log("\nturn 2 > What is my favourite colour?");
  const second = await run(agent, "What is my favourite colour?");
  console.log("assistant:", second.finalOutput);
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
