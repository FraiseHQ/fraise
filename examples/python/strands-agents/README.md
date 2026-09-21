# Strands Agents + Fraise memory

This example gives a [Strands Agents](https://strandsagents.com/) Python agent
Fraise's `remember` and `recall` tools through the existing `fraise mcp` stdio
bridge. No framework-specific Fraise adapter is needed.

The script creates a new `Agent` for each turn, so the second turn has no
conversation history. It can answer only if it calls Fraise to recall the fact
stored in the first turn. Set `CONTROL_RUN=1` to run a third turn with Fraise
disabled; the prompt requires that agent to say `unknown` when it cannot verify
the answer.

## Run with Docker Compose

The compose file starts a local Fraise daemon and builds an agent image that
contains both the `fraise` MCP bridge and the Python dependencies:

```bash
export AWS_REGION=us-east-1
# Provide the credentials required by the Strands model provider.
docker compose run --rm agent
```

To include the no-memory control run:

```bash
CONTROL_RUN=1 docker compose run --rm agent
```

The script prints the tools discovered over MCP before invoking the model. The
Fraise daemon address is `http://fraise:9876` inside Compose.
