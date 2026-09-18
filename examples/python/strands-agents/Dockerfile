# The build context is the repository root (see docker-compose.yaml).
FROM golang@sha256:0d1d3a794be25f809dd2cb3160d8c73276c4056a9f8242a138e908ddeee7b6b6 AS fraise-build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/fraise ./cmd/server

FROM python:3.12-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=fraise-build /out/fraise /usr/local/bin/fraise
COPY sdk/python /opt/fraise-sdk
COPY examples/python/strands-agents/agent.py /app/agent.py

WORKDIR /app
RUN python -m pip install --no-cache-dir /opt/fraise-sdk strands-agents

CMD ["python", "agent.py"]
