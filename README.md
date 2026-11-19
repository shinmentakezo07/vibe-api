# vibe-api

A Go proxy that exposes OpenAI-compatible (and Anthropic Messages) endpoints backed by the Mistral API. Clients speak OpenAI or Anthropic wire formats; the proxy translates requests and responses — including streaming — into Mistral's chat-completions format and relays the replies.

## Features

- **OpenAI compatibility** — `POST /v1/chat/completions`, `GET /v1/models`, `GET /v1/models/:id`, with OpenAI-style error envelopes.
- **Anthropic compatibility** — `POST /v1/messages`, translating Anthropic content blocks, tool use, thinking effort, and system prompts into Mistral messages, and rewriting responses back (streaming included as Anthropic SSE events).
- **Streaming** — SSE relayed line-by-line; Mistral chunks are rewritten into OpenAI or Anthropic shape on the fly.
- **Tool calling** — normalizes tool definitions and tool calls, and maps legacy OpenAI `functions`/`function_call` onto the modern `tools`/`tool_calls` surface.
- **Web dashboard** — React + Tailwind + shadcn/ui control plane for runtime API-key setup, live model discovery, and token usage.
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

- **API key** — validated against Mistral before being stored in memory; swapped in immediately, no restart. `Clear` removes it. The key is never returned by the API, only a masked hint.
- **Models** — fetched from `GET /v1/models` with search and capability filters.
- **Usage** — the proxy records prompt/completion tokens for OpenAI and Anthropic requests, streaming included. The dashboard polls `/api/usage` every 3 seconds and renders per-model and per-minute token flow plus recent requests.
- **Admin API** — `GET /api/status`, `POST /api/key` (`{"apiKey":"..."}`), `DELETE /api/key`, `GET /api/usage`.

Usage and the runtime key are in-memory and reset when the proxy restarts.
When `PROXY_API_KEY` is set, the dashboard's `/api` calls require that bearer
token; serve the dashboard over HTTPS or a trusted reverse proxy before
sending real keys over a network.

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
| `LISTEN_ADDR` | `:8080` | HTTP bind address |
| `ANTHROPIC_MODEL` | `mistral-large-4-0` | Model substituted for Claude model names on the Anthropic route |
| `UPSTREAM_USER_AGENT` | `Mistral-Vibe/3.1.3` | User-Agent sent upstream when the caller does not supply one |
| `WEB_DIR` | `web/dist` | Built dashboard directory served at `/` |
| `DEBUG` | unset | `1` logs request bodies/headers and upstream error payloads |

## Project layout

```
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
│   ├── usage/                 # in-memory token accounting and aggregation
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
    ├── src/components/        # dashboard panels (models, usage, key dialog)
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
