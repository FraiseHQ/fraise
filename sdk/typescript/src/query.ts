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
 * Builders that turn structured arguments into Fraise query strings. Pure, with
 * no I/O — the wire format lives here, and the client only handles transport.
 *
 *   remember@<graph> '<value>' [topic:<t>]... [entity:<e>]... [vec:$<name>]
 *   recall@<graph> <keyword>... [topic:<t>]... [entity:<e>]...
 *                  [top:<n>] [depth:<n>] [vec:$<name>]
 *
 * The vector itself never appears in the string — the caller sends it out of
 * band in the request parameters, and only the `vec:$<name>` placeholder is
 * emitted here.
 */

import { FraiseQueryError } from "./errors.ts";

/**
 * Name bound to the out-of-band vector in the request parameters. A query
 * carries at most one vector, so a single fixed placeholder keeps client and
 * builder in lock-step.
 */
export const VECTOR_PARAM = "v";

/** Validate and return a bare grammar token (a keyword, topic or entity). */
function token(kind: string, value: string): string {
  const trimmed = value.trim();
  if (trimmed === "") throw new FraiseQueryError(`${kind} must not be empty`);
  if (/\s/.test(trimmed)) {
    throw new FraiseQueryError(`${kind} must not contain whitespace: ${JSON.stringify(value)}`);
  }
  return trimmed;
}

function clauses(prefix: string, values: readonly string[] | undefined): string[] {
  if (!values || values.length === 0) return [];
  return values.map((v) => `${prefix}:${token(prefix, v)}`);
}

/** Wrap a fact value in the single quotes the grammar requires. */
function quoteValue(value: string): string {
  if (value.trim() === "") throw new FraiseQueryError("fact value must not be empty");
  // The grammar has no escape inside a single-quoted phrase, so an embedded
  // apostrophe would close the phrase early.
  if (value.includes("'")) {
    throw new FraiseQueryError(
      `fact value must not contain a single quote ('); the query grammar has no ` +
        `way to escape it: ${JSON.stringify(value)}`,
    );
  }
  return `'${value}'`;
}

function selector(graph: number): string {
  if (!Number.isInteger(graph)) throw new FraiseQueryError(`graph must be an integer, got ${graph}`);
  if (graph < 0) throw new FraiseQueryError(`graph must be non-negative, got ${graph}`);
  return `@${graph}`;
}

export interface RememberQuery {
  graph?: number | undefined;
  topics?: readonly string[] | undefined;
  entities?: readonly string[] | undefined;
  withVector?: boolean | undefined;
}

/** Build a `remember` query string that stores `value` in a graph. */
export function buildRemember(value: string, opts: RememberQuery = {}): string {
  const parts = [`remember${selector(opts.graph ?? 0)}`, quoteValue(value)];
  parts.push(...clauses("topic", opts.topics));
  parts.push(...clauses("entity", opts.entities));
  if (opts.withVector) parts.push(`vec:$${VECTOR_PARAM}`);
  return parts.join(" ");
}

export interface RecallQuery {
  keywords?: readonly string[] | undefined;
  graph?: number | undefined;
  topics?: readonly string[] | undefined;
  entities?: readonly string[] | undefined;
  top?: number | undefined;
  depth?: number | undefined;
  withVector?: boolean | undefined;
}

/** Build a `recall` query string. Needs at least one seed. */
export function buildRecall(opts: RecallQuery = {}): string {
  const parts = [`recall${selector(opts.graph ?? 0)}`];
  parts.push(...(opts.keywords ?? []).map((k) => token("keyword", k)));
  parts.push(...clauses("topic", opts.topics));
  parts.push(...clauses("entity", opts.entities));
  if (opts.top !== undefined) {
    if (opts.top <= 0) throw new FraiseQueryError(`top must be positive, got ${opts.top}`);
    parts.push(`top:${opts.top}`);
  }
  if (opts.depth !== undefined) {
    if (opts.depth <= 0) throw new FraiseQueryError(`depth must be positive, got ${opts.depth}`);
    parts.push(`depth:${opts.depth}`);
  }
  if (opts.withVector) parts.push(`vec:$${VECTOR_PARAM}`);

  if (parts.length === 1) {
    throw new FraiseQueryError(
      "recall needs at least one seed: keywords, a vector, or a topic/entity filter",
    );
  }
  return parts.join(" ");
}
