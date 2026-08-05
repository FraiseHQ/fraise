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

import OpenAI from "openai";

import { Embedder } from "./embedder.ts";

export interface OpenAIEmbedderOptions {
  /** Embedding model. Defaults to text-embedding-3-small. */
  model?: string;
  /** A pre-configured OpenAI client; one is created from the env otherwise. */
  client?: OpenAI;
  /** Truncated output size (text-embedding-3-* only), e.g. to match a graph. */
  dimensions?: number;
  /** API key for the auto-created client; falls back to OPENAI_API_KEY. */
  apiKey?: string;
}

/**
 * Text embeddings from OpenAI's embeddings API. The default model is
 * `text-embedding-3-small`; `dimensions` optionally truncates the vector so it
 * can match a graph's fixed embedding size.
 *
 * Requires the optional `openai` peer dependency.
 */
export class OpenAIEmbedder extends Embedder {
  private readonly client: OpenAI;
  private readonly model: string;
  private readonly dimensions: number | undefined;

  constructor(options: OpenAIEmbedderOptions = {}) {
    super();
    this.client = options.client ?? new OpenAI(options.apiKey ? { apiKey: options.apiKey } : {});
    this.model = options.model ?? "text-embedding-3-small";
    this.dimensions = options.dimensions;
  }

  async embed(text: string): Promise<number[]> {
    const response = await this.client.embeddings.create({
      model: this.model,
      input: text,
      ...(this.dimensions !== undefined ? { dimensions: this.dimensions } : {}),
    });
    return response.data[0]!.embedding;
  }
}
