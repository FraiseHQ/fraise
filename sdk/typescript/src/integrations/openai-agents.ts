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
 * Built-in Fraise memory tools for the OpenAI Agents SDK (`@openai/agents`).
 *
 *   import { Agent } from "@openai/agents";
 *   import { FraiseClient } from "fraise-sdk";
 *   import { memoryTools } from "fraise-sdk/integrations/openai-agents";
 *
 *   const fraise = new FraiseClient({ baseUrl: "http://localhost:9876" });
 *   const agent = new Agent({ name: "Assistant", tools: memoryTools(fraise) });
 *
 * The tools bind to one memory graph (default 0), so the agent decides *what* to
 * store and recall, never *where*. Pass an `embedder` to vectorise implicitly:
 * recall and remember then encode their text through it and carry the vector.
 */

import { tool } from "@openai/agents";
import { z } from "zod";

import type { FraiseClient, RecallOptions, RememberOptions } from "../client.ts";
import { FraiseError } from "../errors.js";
import { resolveEmbedder, type Embedder, type EmbedderLike } from "../providers/embedder.ts";

// Tool-call budgets: sane ceilings so the model need not reason about scale.
const DEFAULT_TOP = 5;
const DEFAULT_DEPTH = 2;

export interface MemoryToolOptions {
  graph?: number;
  /** Provide to vectorise implicitly; omit for keyword-only memory. */
  embedder?: Embedder | EmbedderLike;
}

/** A tool that searches long-term memory and returns matching facts. */
export function recallTool(client: FraiseClient, options: MemoryToolOptions = {}) {
  const graph = options.graph ?? 0;
  const encode = resolveEmbedder(options.embedder);

  return tool({
    name: "recall_memory",
    description:
      "Search the assistant's long-term memory for facts related to some keywords " +
      "and return them ranked by relevance. Call this before answering when the user " +
      "refers to something they told you earlier.",
    parameters: z.object({
      keywords: z
        .array(z.string())
        .describe("Salient words to search for — names, topics, or nouns from the question."),
      top: z
        .number()
        .int()
        .positive()
        .nullable()
        .describe("Maximum number of facts to return, most relevant first."),
      depth: z
        .number()
        .int()
        .positive()
        .nullable()
        .describe("How far to follow links between related facts (1 = direct matches only)."),
    }),
    async execute({ keywords, top, depth }): Promise<string> {
      const vector = encode && keywords.length > 0 ? await encode(keywords.join(" ")) : undefined;
      const opts: RecallOptions = {
        graph,
        top: top ?? DEFAULT_TOP,
        depth: depth ?? DEFAULT_DEPTH,
        embed: false,
        ...(vector ? { vector } : {}),
      };
      try {
        const result = await client.recall(keywords, opts);
        if (result.hits.length === 0) return "No stored facts matched those keywords.";
        return result.hits
          .map((h) => `- ${h.value} (relevance ${h.score.toFixed(3)})`)
          .join("\n");
      } catch (error) {
        if (error instanceof FraiseError) return `memory lookup failed: ${error.message}`;
        throw error;
      }
    },
  });
}

/** A tool that stores a single fact in long-term memory. */
export function rememberTool(client: FraiseClient, options: MemoryToolOptions = {}) {
  const graph = options.graph ?? 0;
  const encode = resolveEmbedder(options.embedder);

  return tool({
    name: "remember_fact",
    description:
      "Store a single self-contained fact in the assistant's long-term memory for later " +
      "recall. Use it when the user shares something durable — a preference, a name, a decision.",
    parameters: z.object({
      fact: z
        .string()
        .describe("A single, self-contained statement to remember. Must not contain an apostrophe (')."),
      topics: z
        .array(z.string())
        .nullable()
        .describe("Optional subject tags grouping related facts (e.g. 'color', 'travel')."),
      entities: z
        .array(z.string())
        .nullable()
        .describe("Optional named things the fact is about — a person, place, or object."),
    }),
    async execute({ fact, topics, entities }): Promise<string> {
      const vector = encode ? await encode(fact) : undefined;
      const opts: RememberOptions = {
        graph,
        embed: false,
        ...(topics ? { topics } : {}),
        ...(entities ? { entities } : {}),
        ...(vector ? { vector } : {}),
      };
      try {
        await client.remember(fact, opts);
        return `Stored: ${fact}`;
      } catch (error) {
        if (error instanceof FraiseError) return `could not store the fact: ${error.message}`;
        throw error;
      }
    },
  });
}

/**
 * Return both memory tools (recall + remember) bound to one graph. Pass an
 * `embedder` to make the tools vectorise implicitly.
 */
export function memoryTools(client: FraiseClient, options: MemoryToolOptions = {}) {
  return [recallTool(client, options), rememberTool(client, options)];
}
