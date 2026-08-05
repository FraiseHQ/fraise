# Build context is the repository root (see docker-compose.yaml) so this image
# can install the local, unpublished fraise-sdk alongside the example.
#
# The Claude Agent SDK drives Claude through the Claude Code CLI, so this image
# needs Node.js and the `claude` binary in addition to Python.
FROM python:3.12-slim

COPY --from=ghcr.io/astral-sh/uv:0.9.17 /uv /uvx /bin/

# Node.js (for the Claude Code CLI) from the distro repos, then the CLI itself.
RUN apt-get update \
    && apt-get install -y --no-install-recommends nodejs npm ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && npm install -g @anthropic-ai/claude-code

# Let uv install into the image's system interpreter rather than a venv.
ENV UV_SYSTEM_PYTHON=1

WORKDIR /app

# Install the SDK with its Claude Agent SDK extra. Copied first so this layer is
# cached across changes to the example script.
COPY sdk/python /opt/fraise-sdk
RUN uv pip install "/opt/fraise-sdk[anthropic]"

COPY examples/claude-agent-sdk/agent.py .

CMD ["python", "agent.py"]
