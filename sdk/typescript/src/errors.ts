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

/** Base class for every error raised by the SDK. */
export class FraiseError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "FraiseError";
  }
}

/**
 * A query could not be built from the given arguments — raised before any
 * request leaves the client (e.g. a fact value containing a single quote, or a
 * keyword with embedded whitespace, which the server's grammar cannot express).
 */
export class FraiseQueryError extends FraiseError {
  constructor(message: string) {
    super(message);
    this.name = "FraiseQueryError";
  }
}

/**
 * The server rejected a request or failed to execute it. Carries the HTTP status
 * code and the server-supplied error message (the `error` field of the body).
 */
export class FraiseApiError extends FraiseError {
  readonly statusCode: number;

  constructor(statusCode: number, message: string) {
    super(`fraise request failed [${statusCode}]: ${message}`);
    this.name = "FraiseApiError";
    this.statusCode = statusCode;
  }
}
