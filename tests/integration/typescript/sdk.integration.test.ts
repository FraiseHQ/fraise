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

// Integration test: the TypeScript SDK client can connect to a live Fraise
// server (http://fraise:9876 inside the compose network).

import { beforeAll, describe, expect, it } from "vitest";
import { FraiseClient } from "fraise-sdk";

const FRAISE_URL = process.env.FRAISE_URL ?? "http://localhost:9876";

describe("FraiseClient integration", () => {
  const client = new FraiseClient({ baseUrl: FRAISE_URL });

  // Wait for the server's health check before asserting.
  beforeAll(async () => {
    const deadline = Date.now() + 30_000;
    while (Date.now() < deadline) {
      if (await client.health()) return;
      await new Promise((resolve) => setTimeout(resolve, 500));
    }
    throw new Error(`fraise server not reachable at ${FRAISE_URL}`);
  });

  it("connects to a live Fraise server", async () => {
    expect(await client.health()).toBe(true);
    expect(await client.serverVersion()).toBeTypeOf("string");
  });
});
