# Compatibility matrix

Fraise's server and its SDKs are versioned and released independently (see the
per-package release tags: `v*` for the server, `python/v*` and `typescript/v*`
for the SDKs). This table records which SDK releases are verified against which
server releases. Compatibility is asserted here, not inferred from matching
version numbers.

Each SDK also declares its supported server range in code and can verify it at
runtime against a live server (see [Runtime check](#runtime-check)).

## Matrix

| SDK             | SDK version | Supported server (`fraise`) |
| --------------- | ----------- | --------------------------- |
| Python SDK      | 0.0.x       | `>=0.1.0, <0.2.0`           |
| TypeScript SDK  | 0.0.x       | `>=0.1.0, <0.2.0`           |

While the server is pre-1.0, a minor bump (`0.1 → 0.2`) may introduce breaking
changes, so each SDK pins a single supported minor. Once the server reaches
1.0, supported ranges widen to full major lines (`>=1.0.0, <2.0.0`).

## Runtime check

Both SDKs expose the server's reported version (from the health endpoint,
`GET /`) and a compatibility check against their declared range. The check is
explicit and opt-in — it makes no hidden network calls — and warns by default,
raising only in strict mode.

Python:

```python
from fraise_sdk.client import FraiseClient

client = FraiseClient()
client.server_version()            # -> "0.1.0" or None if unavailable
client.check_compatibility()       # warns on mismatch, returns bool
client.check_compatibility(strict=True)  # raises FraiseError on mismatch
```

TypeScript:

```ts
import { FraiseClient } from "fraise-sdk";

const client = new FraiseClient();
await client.serverVersion();               // "0.1.0" | undefined
await client.checkCompatibility();          // warns on mismatch, resolves boolean
await client.checkCompatibility({ strict: true }); // throws FraiseError on mismatch
```

## Updating this table on release

When cutting an SDK release that starts relying on newer server behaviour:

1. Bump the declared range in the SDK — `SUPPORTED_SERVER` (and the `_SERVER_MIN`
   / `_SERVER_MAX_EXCLUSIVE` bounds) in
   [`sdk/python/src/fraise_sdk/client.py`](sdk/python/src/fraise_sdk/client.py)
   and [`sdk/typescript/src/client.ts`](sdk/typescript/src/client.ts).
2. Update the row above.
3. Tag the SDK release (`python/vX.Y.Z` or `typescript/vX.Y.Z`).
