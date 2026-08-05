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
 * The embedding abstraction. Concrete providers (e.g. {@link OpenAIEmbedder})
 * extend {@link Embedder}; the client also accepts a bare async/sync function of
 * the same shape via {@link EmbedderLike}.
 */

/** A plain function that encodes text into a vector. */
export type EmbedderLike = (text: string) => number[] | Promise<number[]>;

/** Abstract base for anything that encodes text into a vector. */
export abstract class Embedder {
  /** Encode `text` into a fixed-length array of numbers. */
  abstract embed(text: string): number[] | Promise<number[]>;
}

/** Normalize an {@link Embedder}, a bare function, or undefined to one function. */
export function resolveEmbedder(
  embedder: Embedder | EmbedderLike | undefined,
): EmbedderLike | undefined {
  if (embedder === undefined) return undefined;
  if (embedder instanceof Embedder) return (text: string) => embedder.embed(text);
  // Structural fallback: an object exposing an `embed` method, or a plain function.
  const maybe = embedder as { embed?: unknown };
  if (typeof maybe.embed === "function") {
    return (text: string) => (maybe.embed as EmbedderLike)(text);
  }
  if (typeof embedder === "function") return embedder;
  throw new TypeError("embedder must be an Embedder, expose an embed() method, or be a function");
}
