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

import { FraiseApiError, FraiseClient, FraiseError, type FraiseClientOptions } from "../src/index.ts";
import { Embedder } from "../src/providers/embedder.ts";

interface Harness {
  client: FraiseClient;
  body(): Record<string, unknown> | null;
}

/** A FraiseClient wired to a fetch stub that records the last posted body. */
function harness(
  responseBody: unknown,
  status = 200,
  options: Omit<FraiseClientOptions, "fetch"> = {},
): Harness {
  let last: Record<string, unknown> | null = null;
  const fetchImpl = (async (_url: string, init?: RequestInit) => {
    last = init?.body ? JSON.parse(init.body as string) : null;
    return {
      ok: status >= 200 && status < 300,
      status,
      json: async () => responseBody,
      text: async () => JSON.stringify(responseBody),
    } as Response;
  }) as unknown as typeof fetch;
  return { client: new FraiseClient({ ...options, fetch: fetchImpl }), body: () => last };
}

/** A deterministic embedder: returns a fixed-length vector keyed off text length. */
function fixedEmbedder(dim = 4) {
  const calls: string[] = [];
  const embed = (text: string): number[] => {
    calls.push(text);
    return Array<number>(dim).fill(text.length);
  };
  return { embed, calls };
}

const okEmpty = { results: { count: 0, hits: [] } };

describe("FraiseClient basics", () => {
  it("posts the expected remember query", async () => {
    const h = harness(okEmpty);
    await h.client.remember("the parrot is turquoise", { graph: 3, topics: ["color"] });
    expect(h.body()).toEqual({ query: "remember@3 'the parrot is turquoise' topic:color" });
  });

  it("sends an explicit vector as parameters", async () => {
    const h = harness(okEmpty);
    await h.client.remember("kingfisher is blue", { graph: 6, vector: [0.5, 0.5] });
    expect(h.body()).toEqual({
      query: "remember@6 'kingfisher is blue' vec:$v",
      parameters: { v: [0.5, 0.5] },
    });
  });

  it("parses recall hits", async () => {
    const body = {
      results: {
        count: 2,
        hits: [
          { value: "mars is the red planet", score: 1.0, timestamp: "2026-01-01T00:00:00Z" },
          { value: "venus is hot", score: 0.42, timestamp: "2026-01-01T00:00:00Z" },
        ],
      },
    };
    const h = harness(body);
    const result = await h.client.recall(["mars", "venus"], { graph: 7, top: 10 });
    expect(h.body()).toEqual({ query: "recall@7 mars venus top:10" });
    expect(result.count).toBe(2);
    expect(result.hits.map((x) => x.value)).toEqual(["mars is the red planet", "venus is hot"]);
    expect(result.hits[0]!.score).toBe(1.0);
  });

  it("returns an empty result set", async () => {
    const h = harness(okEmpty);
    const result = await h.client.recall(["nothingindexed"]);
    expect(result.count).toBe(0);
    expect(result.hits).toEqual([]);
  });

  it("surfaces the server error message", async () => {
    const h = harness({ error: "could not parse query" }, 400);
    await expect(h.client.recall(["bogus"])).rejects.toThrowError(FraiseApiError);
    await expect(h.client.recall(["bogus"])).rejects.toMatchObject({ statusCode: 400 });
  });
});

describe("FraiseClient embedding", () => {
  it("encodes the remember value", async () => {
    const { embed, calls } = fixedEmbedder();
    const h = harness(okEmpty, 200, { embedder: embed });
    await h.client.remember("the parrot is turquoise", { graph: 6 });
    expect(h.body()).toEqual({
      query: "remember@6 'the parrot is turquoise' vec:$v",
      parameters: { v: [23, 23, 23, 23] }, // "the parrot is turquoise".length === 23
    });
    expect(calls).toEqual(["the parrot is turquoise"]);
  });

  it("encodes the recall keywords by default", async () => {
    const { embed, calls } = fixedEmbedder();
    const h = harness(okEmpty, 200, { embedder: embed });
    await h.client.recall(["kingfisher", "blue"], { graph: 6 });
    expect(h.body()).toMatchObject({ query: "recall@6 kingfisher blue vec:$v" });
    expect(calls).toEqual(["kingfisher blue"]);
  });

  it("lets a query phrase override the keywords for embedding", async () => {
    const { embed, calls } = fixedEmbedder();
    const h = harness(okEmpty, 200, { embedder: embed });
    await h.client.recall(["zzznomatch"], { graph: 6, query: "a sleepy kitten in the sun" });
    expect(calls).toEqual(["a sleepy kitten in the sun"]);
  });

  it("prefers an explicit vector over the embedder", async () => {
    const { embed, calls } = fixedEmbedder();
    const h = harness(okEmpty, 200, { embedder: embed });
    await h.client.remember("x is y", { graph: 6, vector: [0.1, 0.2] });
    expect(h.body()).toMatchObject({ parameters: { v: [0.1, 0.2] } });
    expect(calls).toEqual([]);
  });

  it("skips a configured embedder with embed: false", async () => {
    const { embed, calls } = fixedEmbedder();
    const h = harness(okEmpty, 200, { embedder: embed });
    await h.client.remember("x is y", { graph: 6, embed: false });
    expect(h.body()).not.toHaveProperty("parameters");
    expect(calls).toEqual([]);
  });

  it("throws when embed: true but no embedder is configured", async () => {
    const h = harness(okEmpty);
    await expect(h.client.remember("x is y", { embed: true })).rejects.toThrowError(FraiseError);
  });

  it("sends no vector without an embedder", async () => {
    const h = harness(okEmpty);
    await h.client.remember("x is y", { graph: 6 });
    expect(h.body()).not.toHaveProperty("parameters");
  });

  it("accepts an Embedder subclass instance", async () => {
    class Const extends Embedder {
      embed(): number[] {
        return [1, 2, 3];
      }
    }
    const h = harness(okEmpty, 200, { embedder: new Const() });
    await h.client.remember("hello world", { graph: 6 });
    expect(h.body()).toMatchObject({ parameters: { v: [1, 2, 3] } });
  });
});
