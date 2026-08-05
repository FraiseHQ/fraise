# Build context is the repository root (see docker-compose.yaml) so this image
# can install the local, unpublished fraise-sdk alongside the example.
FROM python:3.12-slim

COPY --from=ghcr.io/astral-sh/uv:0.9.17 /uv /uvx /bin/

# Let uv install into the image's system interpreter rather than a venv.
ENV UV_SYSTEM_PYTHON=1

WORKDIR /app

# Install the SDK with its OpenAI Agents extra. Copied first so this layer is
# cached across changes to the example script.
COPY sdk/python /opt/fraise-sdk
RUN uv pip install "/opt/fraise-sdk[openai]"

COPY examples/openai-agents/agent.py .

CMD ["python", "agent.py"]
