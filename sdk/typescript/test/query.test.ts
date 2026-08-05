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

import { FraiseQueryError } from "../src/errors.ts";
import { buildRecall, buildRemember, VECTOR_PARAM } from "../src/query.ts";

describe("buildRemember", () => {
  it("builds a minimal query", () => {
    expect(buildRemember("the parrot is turquoise")).toBe("remember@0 'the parrot is turquoise'");
  });

  it("adds graph, topics and entities", () => {
    expect(
      buildRemember("anne loves the color orange", { graph: 3, topics: ["color"], entities: ["anne"] }),
    ).toBe("remember@3 'anne loves the color orange' topic:color entity:anne");
  });

  it("appends the vector placeholder", () => {
    expect(buildRemember("the parrot is turquoise", { graph: 6, withVector: true })).toBe(
      `remember@6 'the parrot is turquoise' vec:$${VECTOR_PARAM}`,
    );
  });

  it("rejects an apostrophe", () => {
    expect(() => buildRemember("it's turquoise")).toThrow(FraiseQueryError);
  });

  it("rejects an empty value", () => {
    expect(() => buildRemember("   ")).toThrow(FraiseQueryError);
  });
});

describe("buildRecall", () => {
  it("builds keywords with clauses", () => {
    expect(buildRecall({ keywords: ["anna", "bob"], graph: 2, top: 10, depth: 5 })).toBe(
      "recall@2 anna bob top:10 depth:5",
    );
  });

  it("accepts a vector-only seed", () => {
    expect(buildRecall({ graph: 6, withVector: true })).toBe(`recall@6 vec:$${VECTOR_PARAM}`);
  });

  it("accepts a topic-only seed", () => {
    expect(buildRecall({ topics: ["birds"] })).toBe("recall@0 topic:birds");
  });

  it("requires at least one seed", () => {
    expect(() => buildRecall({ graph: 1 })).toThrow(/at least one seed/);
  });

  it("rejects whitespace in a keyword", () => {
    expect(() => buildRecall({ keywords: ["two words"] })).toThrow(/whitespace/);
  });

  it.each([0, -1])("rejects non-positive top/depth (%i)", (bad) => {
    expect(() => buildRecall({ keywords: ["x"], top: bad })).toThrow(FraiseQueryError);
    expect(() => buildRecall({ keywords: ["x"], depth: bad })).toThrow(FraiseQueryError);
  });

  it("rejects a negative graph", () => {
    expect(() => buildRemember("x", { graph: -1 })).toThrow(/non-negative/);
  });
});
