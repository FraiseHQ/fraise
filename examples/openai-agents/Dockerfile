# Build context is the repository root (see docker-compose.yaml) so this image
# can install the local, unpublished fraise-sdk alongside the example.
#
# The base is a plain minimal Ubuntu, not a python image: uv supplies both the
# interpreter and the resolver, so nothing here needs a preinstalled Python or
# pip. uv itself ships as a static binary, copied from its published image.
FROM ubuntu@sha256:561618e2c15bf2397621dd04f96926663a3b5616c189cf7e38db7e82f5c538ea

COPY --from=ghcr.io/astral-sh/uv:0.9.17 /uv /uvx /usr/local/bin/

# ca-certificates is the one thing Ubuntu lacks that uv needs: without it the
# TLS fetch of the standalone interpreter fails.
# hadolint ignore=DL3008,DL3016
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# UV_LINK_MODE=copy: the cache and the venv are on different layers, where
# uv's default hardlinking cannot work. VIRTUAL_ENV + PATH put the venv's
# interpreter first, so the bare `python` in CMD resolves to it.
ENV UV_PYTHON_INSTALL_DIR=/opt/python \
    UV_LINK_MODE=copy \
    UV_COMPILE_BYTECODE=1 \
    VIRTUAL_ENV=/app/.venv \
    PATH="/app/.venv/bin:$PATH"

# Interpreter in its own layer so editing the example does not re-download it.
RUN uv python install 3.12

# Install the SDK and its OpenAI Agents dependencies. Copied first so this
# layer is cached across changes to the example script. The group is addressed
# by path because fraise-sdk declares [dependency-groups], not extras — the
# older "[openai]" form resolved to no extra and silently installed nothing.
COPY sdk/python /opt/fraise-sdk
RUN uv venv --python 3.12 "$VIRTUAL_ENV" \
    && uv pip install /opt/fraise-sdk \
    --group /opt/fraise-sdk/pyproject.toml:openai

COPY examples/openai-agents/agent.py .

CMD ["python", "agent.py"]
