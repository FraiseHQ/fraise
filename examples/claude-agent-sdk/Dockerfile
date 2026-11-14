# The build context is the repository root (see docker-compose.yaml), so this
# image can install fraise-sdk from this checkout, along with the example.
#
# The base is a plain minimal Ubuntu, not a python image. uv supplies both the
# interpreter and the resolver, so nothing here needs a preinstalled Python or
# pip. uv itself ships as a static binary, copied from its published image. The
# Claude Agent SDK drives Claude through the Claude Code CLI, and its wheel
# bundles the `claude` binary, so the image needs no Node.js.
FROM ubuntu@sha256:f144425ff09be612d6d9ad965196e9cdc23dae1f42110a8a11a3e9a8198759f7

COPY --from=ghcr.io/astral-sh/uv:0.9.17 /uv /uvx /usr/local/bin/

# ca-certificates is the one thing uv needs that Ubuntu lacks. Without it, the
# TLS fetch of the standalone interpreter fails.
# hadolint ignore=DL3008
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# UV_LINK_MODE=copy, because the cache and the venv are on different layers,
# where uv's default hardlinking cannot work. VIRTUAL_ENV and PATH put the
# venv's interpreter first, so the bare `python` in CMD resolves to it.
ENV UV_PYTHON_INSTALL_DIR=/opt/python \
    UV_LINK_MODE=copy \
    UV_COMPILE_BYTECODE=1 \
    VIRTUAL_ENV=/app/.venv \
    PATH="/app/.venv/bin:$PATH"

# Interpreter in its own layer so editing the example does not re-download it.
RUN uv python install 3.12

# Install the SDK with its `anthropic` extra, which brings in the Claude Agent
# SDK. Copied first so this layer is cached across changes to the example
# script.
COPY sdk/python /opt/fraise-sdk
RUN uv venv --python 3.12 "$VIRTUAL_ENV" \
    && uv pip install "/opt/fraise-sdk[anthropic]"

COPY examples/claude-agent-sdk/agent.py .

CMD ["python", "agent.py"]
