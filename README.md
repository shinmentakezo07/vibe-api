# vibe-api

A Go proxy that exposes OpenAI-compatible (and Anthropic Messages) endpoints backed by the Mistral API. Clients speak OpenAI or Anthropic wire formats; the proxy translates requests and responses — including streaming — into Mistral's chat-completions format and relays the replies.

## Features

- **OpenAI compatibility** — `POST /v1/chat/completions`, `GET /v1/models`, `GET /v1/models/:id`, with OpenAI-style error envelopes.
- **Anthropic compatibility** — `POST /v1/messages`, translating Anthropic content blocks, tool use, thinking effort, and system prompts into Mistral messages, and rewriting responses back (streaming included as Anthropic SSE events).
- **Streaming** — SSE relayed line-by-line; Mistral chunks are rewritten into OpenAI or Anthropic shape on the fly.
- **Tool calling** — normalizes tool definitions and tool calls, and maps legacy OpenAI `functions`/`function_call` onto the modern `tools`/`tool_calls` surface.
- **Web dashboard** — React + Tailwind + shadcn/ui control plane for API-key pools, round-robin rotation, a dedicated model catalog, and live token, throughput, and latency analytics.
- **Optional auth** — set `PROXY_API_KEY` to require a matching bearer token on all `/v1` and `/api` routes.
- **CORS** — browser-based clients can call the proxy directly.
- **Debug logging** — `DEBUG=1` logs request bodies, headers, and upstream error payloads.

## Getting started

Requirements: Go 1.27+, Node 20+ for the dashboard, and a Mistral API key.

```sh
npm --prefix web install
npm --prefix web run build
go run ./cmd/vibe-api
```

The server listens on `:8080` and serves the dashboard at `/`. `MISTRAL_API_KEY`
can be set before startup or added later from the UI. Check `GET /healthz` for
liveness.

For frontend development with hot reload, run the proxy and Vite side by side.
Vite proxies `/v1` and `/api` to `:8080`:

```sh
go run ./cmd/vibe-api       # terminal 1
npm --prefix web run dev    # terminal 2 → http://localhost:5173
```

### Web dashboard

- **API keys** — a dedicated page at `/#keys` adds, enables, disables, and deletes multiple Mistral keys. New keys are validated before joining the pool. Optional round-robin rotates each request across enabled keys; when disabled, the first enabled key is used. Each key shows its own request count, token usage, last-used time, and its most recent upstream errors (status, route, model, and message), so it is obvious which key is absorbing traffic or failing. Secrets are never returned, only masked hints.
- **Models** — a dedicated page at `/#models` lists the full `GET /v1/models` catalog with search, capability filters, context windows, copyable ids, and live counts for tool calling, vision, and reasoning.
- **Usage** — the Overview page polls `/api/usage` every 3 seconds and shows total, cached, and reasoning tokens plus average per-request output TPS; rolling TPM/TPS, cache-hit, and reasoning-share gauges with a 60-second pulse; output throughput over the last 20 minutes; a p50/p95 latency distribution; and a request log with per-call token, cache, reasoning, latency, and output-TPS accounting. Prompt/completion/cached/reasoning usage is captured for OpenAI and Anthropic requests, streaming included.
- **Admin API** — `GET /api/status`, `GET /api/keys`, `POST /api/keys`
  (`{"label":"...","apiKey":"..."}`), `PATCH /api/keys/:id`
  (`{"enabled":false,"label":"..."}`), `DELETE /api/keys/:id`,
  `PUT /api/keys/round-robin` (`{"enabled":true}`), and `GET /api/usage`.

Without `DATABASE_URL`, keys and usage are in-memory and reset when the proxy
restarts. With PostgreSQL configured, the key pool, round-robin setting, and
every completion record are persisted.
When `PROXY_API_KEY` is set, the dashboard's `/api` calls require that bearer
token; serve the dashboard over HTTPS or a trusted reverse proxy before
sending real keys over a network.

## Docker and Railway

The API, dashboard, and admin endpoints share one port. `Dockerfile` builds the
Vite bundle and the Go binary in separate stages, then runs both from a small
Alpine image as a non-root user.

### Docker Compose

`compose.yaml` runs the proxy and PostgreSQL together. The database is exposed
only to the API container and its data lives in the `vibe-db` volume.

```sh
cp .env.example .env   # set APP_SECRET and optionally MISTRAL_API_KEY
docker compose up --build
```

Open `http://localhost:8080`; `/healthz` is the health check. The API waits for
Postgres to become healthy, and `APP_SECRET` encrypts stored keys. Stop with
`docker compose down` (data kept) or `docker compose down -v` (data deleted).

Set `APP_PORT` in `.env` to change the published port. `POSTGRES_PASSWORD`
defaults to `vibe` for local use; override it for anything shared.

### Image only

```sh
docker build -t vibe-api .
docker run --rm -p 8080:8080 -e MISTRAL_API_KEY=... vibe-api
```

Without `DATABASE_URL` the image keeps the key and usage in memory.

### Railway

1. Create a Railway project from this repository. Railway detects
   `railway.toml` and builds the `Dockerfile`.
2. Set variables:

| Variable | Value |
|---|---|
| `MISTRAL_API_KEY` | Your Mistral key (recommended; survives restarts) |
| `PROXY_API_KEY` | Optional bearer token for `/v1` and `/api` |
| `DATABASE_URL` | Optional PostgreSQL DSN; enables durable key and usage |
| `APP_SECRET` | Optional AES-256-GCM secret for stored keys |
| `ANTHROPIC_MODEL` | Optional Claude-model substitute, default `mistral-large-4-0` |

Railway injects `PORT`; the server binds to it automatically (see
`internal/config/config.go`). Do not set `LISTEN_ADDR` unless you need to
override it. `railway.toml` points the health check at `/healthz`.

If `MISTRAL_API_KEY` is not set, add keys from the **API keys** page. With a
database they survive redeploys; without it, runtime keys live in memory. An
environment key appears as the `Environment` entry and is restored from the
variable on restart. With `PROXY_API_KEY` enabled, protect the admin routes at
your reverse proxy, since the dashboard does not send that token.

### PostgreSQL persistence

Add a PostgreSQL database to the Railway project, then expose its connection
string to the API service as `DATABASE_URL` (for example
`${{Postgres.DATABASE_URL}}`). The server connects at startup, creates the
schema, and loads the key pool before listening.

The included `compose.yaml` already wires `DATABASE_URL` to its Postgres
service. To use an external database:

```sh
docker run --rm -p 8080:8080 \
  -e DATABASE_URL=postgres://vibe:vibe@host.docker.internal:5432/vibe \
  -e APP_SECRET=change-me \
  -e MISTRAL_API_KEY=... vibe-api
```

Tables created automatically:

- `api_keys` — labels, encrypted secrets, enabled state, timestamps.
- `settings` — round-robin state and the legacy single-key migration slot.
- `usage_records` — one row per billed completion with prompt, completion, total, cached-input, reasoning, and latency fields, indexed by `created_at`. Existing tables gain the new fields automatically on startup.

A key saved by the old single-key schema is imported as `Imported` on first
start. Set `APP_SECRET` to encrypt stored keys with AES-256-GCM. Without it keys
are stored as plaintext and a startup warning is logged; changing the secret
later makes existing encrypted keys unreadable. A unique `MISTRAL_API_KEY` from
the environment joins the pool as a runtime-only `Environment` key. Dashboard
writes update the pool and Postgres; the dashboard exposes whether storage is
`postgres` or `memory`.

### Example

```sh
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "mistral-large-4-0",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

Add `Authorization: Bearer $PROXY_API_KEY` when proxy authentication is enabled.

### Anthropic Messages

Point an Anthropic client at the proxy; Claude model names are replaced with
`ANTHROPIC_MODEL` (default `mistral-large-4-0`).

```sh
curl http://localhost:8080/v1/messages \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-sonnet-4-5",
    "max_tokens": 300,
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

```python
from anthropic import Anthropic

client = Anthropic(base_url="http://localhost:8080", api_key="local")
message = client.messages.create(
    model="claude-sonnet-4-5",
    max_tokens=300,
    messages=[{"role": "user", "content": "Hello!"}],
    tools=[{
        "name": "bash",
        "description": "Run a shell command",
        "input_schema": {
            "type": "object",
            "properties": {"command": {"type": "string"}},
            "required": ["command"],
        },
    }],
)
```

Supported: system prompts, text/image content, `tool_use` / `tool_result`
replay, `thinking.budget_tokens` (mapped to `reasoning_effort`), `tool_choice`,
and streaming SSE events. In streams, `message_start.usage.input_tokens` is `0`
because Mistral reports prompt tokens only at the end; the authoritative
cumulative count arrives in `message_delta.usage.input_tokens`.

## Configuration

All settings come from environment variables (see `internal/config/config.go`).

| Variable | Default | Purpose |
|---------------|----------------------------|--------------------------------------------------|
| `MISTRAL_API_KEY` | unset (set from UI) | Upstream credential; can also be added at runtime |
| `MISTRAL_BASE_URL` | `https://api.mistral.ai` | Upstream API origin |
| `PROXY_API_KEY` | unset (auth disabled) | Required bearer token for inbound requests |
| `LISTEN_ADDR` | `$PORT`, else `:8080` | HTTP bind address |
| `PORT` | — | Platform-provided port, used when `LISTEN_ADDR` is unset |
| `ANTHROPIC_MODEL` | `mistral-large-4-0` | Model substituted for Claude model names on the Anthropic route |
| `UPSTREAM_USER_AGENT` | `Mistral-Vibe/3.1.3` | User-Agent sent upstream when the caller does not supply one |
| `WEB_DIR` | `web/dist` | Built dashboard directory served at `/` |
| `DEBUG` | unset | `1` logs request bodies/headers and upstream error payloads |
| `DATABASE_URL` | unset (memory) | PostgreSQL DSN; enables durable key and usage storage |
| `APP_SECRET` | unset | Encrypts the stored key with AES-256-GCM; required to read encrypted keys |

## Project layout

```
├── Dockerfile                 # multi-stage dashboard + Go build, one runtime port
├── compose.yaml               # API + PostgreSQL with a persistent volume
├── .env.example               # local Compose configuration template
├── railway.toml               # Railway Dockerfile build + /healthz check
├── cmd/vibe-api/main.go       # entry point: config, HTTP server, graceful shutdown
├── internal/
│   ├── config/config.go       # environment-based configuration and validation
│   ├── mistral/client.go      # authenticated upstream HTTP client (SSE-friendly)
│   ├── openai/openai.go       # OpenAI wire types (errors, stream flag)
│   ├── server/                # Gin router, middleware, handlers, SSE relays
│   │   ├── server.go          #   route registration
│   │   ├── handlers.go        #   OpenAI endpoints
│   │   ├── anthropic.go       #   Anthropic Messages endpoint
│   │   ├── admin.go           #   key management + usage endpoints
│   │   ├── ui.go              #   dashboard static serving / SPA fallback
│   │   ├── stream.go          #   SSE relay + chunk rewriting
│   │   └── middleware.go      #   auth, CORS, request logging
│   ├── keys/                  # key pool, enablement, and round-robin selection
│   ├── store/                 # PostgreSQL key + usage persistence and migrations
│   ├── usage/                 # token accounting types and in-memory fallback
│   └── translator/            # request/response/stream translation
│       ├── request.go         #   OpenAI → Mistral request normalization
│       ├── response.go        #   Mistral → OpenAI response/chunk rewriting
│       ├── anthropic.go       #   Anthropic Messages ⇄ Mistral requests/responses
│       ├── anthropic_stream.go #  Mistral chunks → Anthropic SSE events
│       ├── tools.go           #   tool-call mapping and legacy function support
│       ├── translator_test.go #   OpenAI-side translation tests
│       └── anthropic_test.go  #   Anthropic translation and stream tests
└── web/                       # React + Vite + Tailwind + shadcn/ui dashboard
    ├── src/components/ui/     # shadcn component sources
    ├── src/components/        # pages and panels (models, usage, keys)
    └── src/lib/api.ts         # typed client for proxy + admin endpoints
```

## Building and testing

```sh
go build ./...                    # compile everything
go test ./...                     # Go test suite
npm --prefix web run build        # production dashboard build
go build -o vibe-api ./cmd/vibe-api
```

The server handles SIGINT/SIGTERM and drains in-flight requests with a 10-second grace period.
