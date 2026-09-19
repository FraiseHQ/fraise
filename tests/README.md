# Server-backed test suites

Two suites live here, and both drive a real fraise server

| Suite | Path | What it tests | Command |
| --- | --- | --- | --- |
| End to end | `tests/e2e/` | the server itself, over raw HTTP | `make test-e2e` |
| Server + MCP bridge | `tests/integration/` | `pkg/mcp` over stdio, against the daemon behind it | `make test-integration` |

The SDK's own tests are not here. They live beside the package in `sdk/python/src/tests/`.

## Which suite does a test belong in?

- **SDK unit** if it needs no server: it goes in the mirrored file under `sdk/python/src/tests/`, unmarked. Collaborators are mocked with `unittest.mock`.
- **SDK integration** if it exercises *the SDK's* behaviour against a live server — the query strings it generates, the JSON it parses, the exceptions it maps. Same mirrored file, under its `# -- integration` banner, carrying `@pytest.mark.integration`.
- **Server + MCP bridge** (`tests/integration/`) if it exercises what an MCP client sees: the handshake, the tool schemas, a `tools/call` round trip. These mirror `pkg/mcp`, not the SDK, and speak JSON-RPC over the bridge's stdin/stdout.
- **End to end** (`tests/e2e/`) if it exercises *the server's* behaviour and neither SDK nor bridge is the point. These mirror the server's concerns and send hand-written query strings over `requests`.
