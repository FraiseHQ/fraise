# Build context is the repository root (see docker-compose.yaml) so this image
# can install the local, unpublished fraise-sdk alongside the example.
#
# The base is a plain minimal Ubuntu, not a python image: uv supplies both the
# interpreter and the resolver, so nothing here needs a preinstalled Python or
# pip. The Claude Agent SDK drives Claude through the Claude Code CLI, so this
# image additionally needs Node.js and the `claude` binary.
FROM ubuntu:sha256:561618e2c15bf2397621dd04f96926663a3b5616c189cf7e38db7e82f5c538ea

COPY --from=ghcr.io/astral-sh/uv:0.9.17 /uv /uvx /usr/local/bin/

# Node.js (for the Claude Code CLI) from the distro repos, then the CLI itself.
# ca-certificates also lets uv fetch the standalone interpreter over TLS.
# DL3008/DL3016: this is a demo image that should track the current CLI and
# distro packages; pinning them here would go stale silently.
# hadolint ignore=DL3008,DL3016
RUN apt-get update \
    && apt-get install -y --no-install-recommends nodejs npm ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && npm install -g @anthropic-ai/claude-code

WORKDIR /app

# UV_LINK_MODE=copy: the cache and the venv are on different layers, where
# uv's default hardlinking cannot work. VIRTUAL_ENV + PATH put the venv's
# interpreter first, so a bare `python` resolves to it.
ENV UV_PYTHON_INSTALL_DIR=/opt/python \
    UV_LINK_MODE=copy \
    UV_COMPILE_BYTECODE=1 \
    VIRTUAL_ENV=/app/.venv \
    PATH="/app/.venv/bin:$PATH"

# Interpreter in its own layer so editing the example does not re-download it.
RUN uv python install 3.12

# Install the SDK and its Claude Agent SDK dependencies. Copied first so this
# layer is cached across changes to the example script. The group is addressed
# by path because fraise-sdk declares [dependency-groups], not extras — the
# older "[anthropic]" form resolved to no extra and silently installed nothing.
COPY sdk/python /opt/fraise-sdk
RUN uv venv --python 3.12 "$VIRTUAL_ENV" \
    && uv pip install /opt/fraise-sdk \
    --group /opt/fraise-sdk/pyproject.toml:anthropic

COPY examples/claude-agent-sdk/agent.py .
