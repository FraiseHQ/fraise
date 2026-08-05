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

import { Embedder, resolveEmbedder } from "../src/providers/embedder.ts";
import { OpenAIEmbedder } from "../src/providers/openai.ts";

describe("resolveEmbedder", () => {
  it("returns undefined for undefined", () => {
    expect(resolveEmbedder(undefined)).toBeUndefined();
  });

  it("passes through a plain function", () => {
    const fn = (t: string): number[] => [t.length];
    const resolved = resolveEmbedder(fn);
    expect(resolved?.("abc")).toEqual([3]);
  });

  it("binds the embed method of an Embedder", async () => {
    class E extends Embedder {
      embed(text: string): number[] {
        return [text.length];
      }
    }
    const resolved = resolveEmbedder(new E());
    expect(await resolved?.("abcd")).toEqual([4]);
  });

  it("rejects a non-embedder value", () => {
    expect(() => resolveEmbedder({} as never)).toThrow(TypeError);
  });
});

// A minimal stand-in for the OpenAI client (no network).
function fakeOpenAI() {
  const record: Record<string, unknown> = {};
  return {
    record,
    embeddings: {
      create: async (args: Record<string, unknown>) => {
        Object.assign(record, args);
        return { data: [{ embedding: [0.1, 0.2, 0.3] }] };
      },
    },
  };
}

describe("OpenAIEmbedder", () => {
  it("calls the client and returns the vector", async () => {
    const fake = fakeOpenAI();
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const embedder = new OpenAIEmbedder({ client: fake as any, model: "text-embedding-3-small", dimensions: 3 });
    await expect(embedder.embed("hello")).resolves.toEqual([0.1, 0.2, 0.3]);
    expect(fake.record).toEqual({ model: "text-embedding-3-small", input: "hello", dimensions: 3 });
  });

  it("omits dimensions when unset", async () => {
    const fake = fakeOpenAI();
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const embedder = new OpenAIEmbedder({ client: fake as any });
    await embedder.embed("world");
    expect(fake.record).not.toHaveProperty("dimensions");
    expect(fake.record["input"]).toBe("world");
  });

  it("is an Embedder", () => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    expect(new OpenAIEmbedder({ client: fakeOpenAI() as any })).toBeInstanceOf(Embedder);
  });
});
