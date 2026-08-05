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

import { FraiseApiError, FraiseError } from "./errors.ts";
import { toRecallResult, type RecallResult } from "./models.ts";
import { buildRecall, buildRemember, VECTOR_PARAM } from "./query.ts";
import { resolveEmbedder, type Embedder, type EmbedderLike } from "./providers/embedder.ts";

const DEFAULT_BASE_URL = "http://localhost:9876";
const DEFAULT_TIMEOUT_MS = 10_000;

/**
 * Server versions this SDK is verified against. Keep in sync with COMPATIBILITY.md
 * and bump when a release starts relying on newer server behaviour.
 */
export const SUPPORTED_SERVER = ">=0.1.0,<0.2.0";
const SERVER_MIN: readonly [number, number, number] = [0, 1, 0];
const SERVER_MAX_EXCLUSIVE: readonly [number, number, number] = [0, 2, 0];

/** Parse `major.minor.patch` into a tuple, ignoring any pre-release suffix. */
function parseVersion(text: string): [number, number, number] | undefined {
  const parts = text.trim().replace(/^v/, "").split(".");
  if (parts.length < 3) return undefined;
  const out: number[] = [];
  for (const part of parts.slice(0, 3)) {
    const digits = /^\d+/.exec(part);
    if (!digits) return undefined;
    out.push(Number(digits[0]));
  }
  return [out[0]!, out[1]!, out[2]!];
}

/** Return <0, 0, or >0 comparing two version tuples lexicographically. */
function compareVersions(
  a: readonly [number, number, number],
  b: readonly [number, number, number],
): number {
  return a[0] - b[0] || a[1] - b[1] || a[2] - b[2];
}

export interface FraiseClientOptions {
  /** Base URL of the Fraise server. Defaults to http://localhost:9876. */
  baseUrl?: string;
  /** Per-request timeout in milliseconds. Defaults to 10000. */
  timeoutMs?: number;
  /** Encodes text to a vector so remember/recall can vectorise automatically. */
  embedder?: Embedder | EmbedderLike;
  /** Override the fetch implementation (e.g. for tests). Defaults to global fetch. */
  fetch?: typeof fetch;
}

/** Per-call knobs shared by remember and recall. */
interface VectorControls {
  /** Explicit embedding; wins over the configured embedder. */
  vector?: number[];
  /** true forces encoding (errors without an embedder), false skips it. */
  embed?: boolean;
  /** Override the client's default request timeout for this call. */
  timeoutMs?: number;
}

export interface RememberOptions extends VectorControls {
  graph?: number;
  topics?: readonly string[];
  entities?: readonly string[];
}

export interface RecallOptions extends VectorControls {
  graph?: number;
  /** Free-text phrase to embed for semantic search; defaults to the keywords. */
  query?: string;
  topics?: readonly string[];
  entities?: readonly string[];
  top?: number;
  depth?: number;
}

/**
 * A thin, `fetch`-based client over the Fraise query API. All memory operations
 * funnel through `POST /api/v1/q`; {@link remember} and {@link recall} are typed
 * conveniences over it, and {@link query} is the raw escape hatch.
 *
 * Provide an `embedder` to have remember/recall encode their text to a vector
 * automatically; any call can override with an explicit `vector` or opt out with
 * `embed: false`.
 */
export class FraiseClient {
  readonly baseUrl: string;
  readonly timeoutMs: number;
  private readonly fetchImpl: typeof fetch;
  private readonly embed: EmbedderLike | undefined;

  constructor(options: FraiseClientOptions = {}) {
    this.baseUrl = (options.baseUrl ?? DEFAULT_BASE_URL).replace(/\/+$/, "");
    this.timeoutMs = options.timeoutMs ?? DEFAULT_TIMEOUT_MS;
    this.fetchImpl = options.fetch ?? globalThis.fetch;
    this.embed = resolveEmbedder(options.embedder);
    if (this.fetchImpl === undefined) {
      throw new FraiseError("no fetch implementation available; pass options.fetch");
    }
  }

  /** Return true if the server answers its health check with 200. */
  async health(): Promise<boolean> {
    try {
      const response = await this.fetchImpl(`${this.baseUrl}/`, {
        signal: this.timeoutSignal(this.timeoutMs),
      });
      return response.status === 200;
    } catch {
      return false;
    }
  }

  /**
   * Return the server's reported version, or `undefined` if unavailable.
   *
   * Reads the `version` field from the health endpoint. `undefined` means the
   * server is unreachable, answered non-200, or predates version reporting.
   */
  async serverVersion(): Promise<string | undefined> {
    let response: Response;
    try {
      response = await this.fetchImpl(`${this.baseUrl}/`, {
        signal: this.timeoutSignal(this.timeoutMs),
      });
    } catch {
      return undefined;
    }
    if (response.status !== 200) return undefined;
    let body: unknown;
    try {
      body = await response.json();
    } catch {
      return undefined;
    }
    const version = typeof body === "object" && body ? (body as { version?: unknown }).version : undefined;
    return typeof version === "string" && version ? version : undefined;
  }

  /**
   * Verify the live server falls within this SDK's supported range
   * ({@link SUPPORTED_SERVER}).
   *
   * Resolves `true` when the server version is in range. On a mismatch — or when
   * the version can't be determined — the default logs a warning via
   * `console.warn` and resolves `false`; pass `{ strict: true }` to throw a
   * {@link FraiseError} instead. Call it explicitly when you want the guarantee.
   */
  async checkCompatibility(options: { strict?: boolean } = {}): Promise<boolean> {
    const strict = options.strict ?? false;
    const version = await this.serverVersion();
    if (version === undefined) {
      const message = `could not determine fraise server version at ${this.baseUrl}`;
      if (strict) throw new FraiseError(message);
      console.warn(message);
      return false;
    }

    const parsed = parseVersion(version);
    const inRange =
      parsed !== undefined &&
      compareVersions(parsed, SERVER_MIN) >= 0 &&
      compareVersions(parsed, SERVER_MAX_EXCLUSIVE) < 0;
    if (!inRange) {
      const message =
        `fraise server ${version} is outside this SDK's supported range ` +
        `${SUPPORTED_SERVER}; behaviour may be undefined`;
      if (strict) throw new FraiseError(message);
      console.warn(message);
      return false;
    }
    return true;
  }

  /** Store `value` as a fact. See {@link RememberOptions}. */
  async remember(value: string, options: RememberOptions = {}): Promise<void> {
    const vector = await this.resolveVector(options, value);
    const text = buildRemember(value, {
      graph: options.graph ?? 0,
      topics: options.topics,
      entities: options.entities,
      withVector: vector !== undefined,
    });
    await this.query(text, {
      parameters: vector ? { [VECTOR_PARAM]: vector } : undefined,
      timeoutMs: options.timeoutMs,
    });
  }

  /** Search for facts and return them ranked by relevance. See {@link RecallOptions}. */
  async recall(keywords: readonly string[] = [], options: RecallOptions = {}): Promise<RecallResult> {
    const embedText = options.query ?? keywords.join(" ");
    const vector = await this.resolveVector(options, embedText);
    const text = buildRecall({
      keywords,
      graph: options.graph ?? 0,
      topics: options.topics,
      entities: options.entities,
      top: options.top,
      depth: options.depth,
      withVector: vector !== undefined,
    });
    const body = await this.query(text, {
      parameters: vector ? { [VECTOR_PARAM]: vector } : undefined,
      timeoutMs: options.timeoutMs,
    });
    const results = (body as { results?: Parameters<typeof toRecallResult>[0] }).results;
    return toRecallResult(results);
  }

  /** Send a raw query string and return the decoded JSON body. */
  async query(
    text: string,
    options: {
      parameters?: Record<string, number[]> | undefined;
      timeoutMs?: number | undefined;
    } = {},
  ): Promise<Record<string, unknown>> {
    const payload: Record<string, unknown> = { query: text };
    if (options.parameters) payload["parameters"] = options.parameters;

    let response: Response;
    try {
      response = await this.fetchImpl(`${this.baseUrl}/api/v1/q`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
        signal: this.timeoutSignal(options.timeoutMs ?? this.timeoutMs),
      });
    } catch (cause) {
      throw new FraiseError(`could not reach fraise at ${this.baseUrl}: ${String(cause)}`);
    }

    // Every response the server produces is JSON — decode first so an error
    // body's `error` field can be surfaced verbatim.
    let body: unknown;
    try {
      body = await response.json();
    } catch {
      body = {};
    }

    if (!response.ok) {
      const message =
        (typeof body === "object" && body && (body as { error?: string }).error) || "unknown error";
      throw new FraiseApiError(response.status, message);
    }
    return (typeof body === "object" && body ? body : {}) as Record<string, unknown>;
  }

  /**
   * Decide the vector to send: an explicit `vector` wins; otherwise encode when
   * asked (`embed: true`) or when an embedder is configured and the text is
   * non-empty. `embed: false` never encodes.
   */
  private async resolveVector(controls: VectorControls, text: string): Promise<number[] | undefined> {
    if (controls.vector !== undefined) return controls.vector;
    if (controls.embed === false) return undefined;
    if (controls.embed === true && this.embed === undefined) {
      throw new FraiseError(
        "embed: true but this client has no embedder; construct it with new FraiseClient({ embedder })",
      );
    }
    if (this.embed === undefined || text.trim() === "") return undefined;
    return this.embed(text);
  }

  private timeoutSignal(ms: number): AbortSignal {
    return AbortSignal.timeout(ms);
  }
}
