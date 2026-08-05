# Build context is the repository root (see docker-compose.yaml) so this image
# can build and install the local, unpublished fraise-sdk alongside the example.
FROM node:22-slim

# pnpm via corepack (the SDK pins its own pnpm version through packageManager).
RUN corepack enable

# 1. Build the local TypeScript SDK and pack it to a tarball (contains dist/
#    only, per the SDK's "files" field).
WORKDIR /sdk
COPY sdk/typescript/ ./
RUN pnpm install --frozen-lockfile && pnpm run build && pnpm pack --pack-destination /pkg

# 2. Install the example's own dependencies, then add the packed SDK tarball.
WORKDIR /app
COPY examples/openai-agents-ts/package.json ./
RUN pnpm install && pnpm add /pkg/fraise-sdk-*.tgz

# 3. The example source. Copied last so edits don't bust the dependency layers.
#    Run via Node's built-in TypeScript type-stripping — no bundler/tsx needed.
COPY examples/openai-agents-ts/agent.ts ./

CMD ["node", "--experimental-strip-types", "agent.ts"]
