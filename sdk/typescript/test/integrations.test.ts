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

import { describe, expect, it } from "vitest";

import { FraiseClient, type FraiseClientOptions } from "../src/index.ts";
import { memoryTools, recallTool, rememberTool } from "../src/integrations/openai-agents.ts";

function harness(options: Omit<FraiseClientOptions, "fetch"> = {}) {
  let last: Record<string, unknown> | null = null;
  const fetchImpl = (async (_url: string, init?: RequestInit) => {
    last = init?.body ? JSON.parse(init.body as string) : null;
    return { ok: true, status: 200, json: async () => ({ results: { count: 0, hits: [] } }) } as Response;
  }) as unknown as typeof fetch;
  return { client: new FraiseClient({ ...options, fetch: fetchImpl }), body: () => last };
}

// The @openai/agents FunctionTool is invoked with (context, jsonArgs).
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const invoke = (tool: any, args: unknown) => tool.invoke({}, JSON.stringify(args));

const embed = (t: string): number[] => [Number((0.1 * t.length).toFixed(3))];

describe("memoryTools", () => {
  it("exposes both tools with stable names", () => {
    const { client } = harness();
    expect(memoryTools(client).map((t) => t.name)).toEqual(["recall_memory", "remember_fact"]);
  });
});

describe("remember tool", () => {
  it("vectorises when given an embedder", async () => {
    const h = harness();
    const tool = rememberTool(h.client, { graph: 6, embedder: embed });
    await invoke(tool, { fact: "the sky is blue", topics: null, entities: null });
    expect(h.body()).toEqual({
      query: "remember@6 'the sky is blue' vec:$v",
      parameters: { v: [1.5] }, // 0.1 * "the sky is blue".length (15)
    });
  });

  it("stores text only without an embedder", async () => {
    const h = harness();
    const tool = rememberTool(h.client, { graph: 6 });
    await invoke(tool, { fact: "the sky is blue", topics: ["weather"], entities: null });
    expect(h.body()).toEqual({ query: "remember@6 'the sky is blue' topic:weather" });
  });
});

describe("recall tool", () => {
  it("vectorises the keywords when given an embedder", async () => {
    const h = harness();
    const tool = recallTool(h.client, { graph: 6, embedder: embed });
    await invoke(tool, { keywords: ["sky", "blue"], top: null, depth: null });
    expect(h.body()).toEqual({
      query: "recall@6 sky blue top:5 depth:2 vec:$v",
      parameters: { v: [0.8] }, // 0.1 * "sky blue".length (8)
    });
  });

  it("is keyword-only without an embedder", async () => {
    const h = harness();
    const tool = recallTool(h.client, { graph: 6 });
    await invoke(tool, { keywords: ["sky"], top: 3, depth: null });
    expect(h.body()).toEqual({ query: "recall@6 sky top:3 depth:2" });
  });
});
