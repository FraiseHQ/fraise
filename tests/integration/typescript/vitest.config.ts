import { defineConfig } from "vitest/config";

// Integration tests wait for a live server, so allow generous timeouts.
export default defineConfig({
  test: {
    testTimeout: 35_000,
    hookTimeout: 35_000,
  },
});
