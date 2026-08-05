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

/** One recalled fact and how strongly it matched the query. */
export interface Hit {
  value: string;
  score: number;
  timestamp?: string;
}

/**
 * The result set of a `recall`: how many facts matched and, in ranked order,
 * what they were.
 */
export interface RecallResult {
  count: number;
  hits: Hit[];
}

/** Shape of the `results` object in a successful query response. */
interface RawResults {
  count?: number;
  hits?: Array<{ value: string; score: number; timestamp?: string }>;
}

/** Build a {@link RecallResult} from the server's `results` payload. */
export function toRecallResult(results: RawResults | undefined): RecallResult {
  const hits: Hit[] = (results?.hits ?? []).map((h) => {
    const hit: Hit = { value: h.value, score: Number(h.score) };
    if (h.timestamp !== undefined) hit.timestamp = h.timestamp;
    return hit;
  });
  // Prefer the server-reported count, falling back to the hit count so the two
  // never disagree if the field is ever omitted.
  return { count: results?.count ?? hits.length, hits };
}
