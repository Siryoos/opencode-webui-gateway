# Phase 1 Implementation Report

Date: 2026-06-01

## Implemented Endpoints

- `GET /health`
- `GET /v1/models`
- `POST /v1/chat/completions`
- Synchronous SSE compatibility shim for `stream: true`.

## Verified OpenCode Schemas Used

The implementation uses only fields verified in `docs/discovery/live-contract-verification.md`:

- Session creation: `POST /session` with `title`.
- Message forwarding: `POST /session/:id/message` with `agent`, optional `system`, and text-only `parts`.
- Response extraction: concatenate returned parts where `type == "text"`.

OpenCode was unsecured during verification. The gateway supports unsecured local mode only when `ALLOW_UNSECURED_OPENCODE=true`; otherwise it requires `OPENCODE_SERVER_PASSWORD` and sends Basic Auth downstream.

## Open WebUI Runtime Status

Open WebUI initially was not running locally. The expected runtime is documented in `docs/discovery/openwebui-runtime-verification.md`: container `open-webui-adina`, URL `http://127.0.0.1:3000`, gateway base URL `http://host.docker.internal:8080/v1` or equivalent, with `ENABLE_FORWARD_USER_INFO_HEADERS=true`.

Open WebUI was observed to send `stream: true`. The gateway now supports that through a synchronous SSE shim so Open WebUI no longer receives `streaming is not supported in Phase 1`.

## Verified Host OpenCode Success

Known-good host mode:

```bash
cd /home/siryoos/opencode-docker/projects/agent-with-memory
opencode serve
```

Gateway environment:

```bash
GATEWAY_API_KEY=dev-secret
OPENCODE_BASE_URL=http://127.0.0.1:4096
DATABASE_PATH=./gateway.sqlite3
ALLOW_UNSECURED_OPENCODE=true
REQUIRE_AUTH_ON_HEALTH=false
REQUEST_TIMEOUT_SECONDS=120
```

Verified successful chat summary:

- `POST /v1/chat/completions`
- `model: adina-analysis`
- `stream: false`
- `X-OpenWebUI-User-Id: debug-user`
- `X-OpenWebUI-Chat-Id: host-opencode-chat-1`
- Response object: `chat.completion`
- Assistant content included: `Hello! My current project directory is /home/siryoos/opencode-docker/projects/agent-with-memory`.

## Model Mapping

| Public model | OpenCode agent |
|---|---|
| `adina-analysis` | `plan` |
| `adina-execution` | `build` |

No aliases, prompt-based routing, raw agent exposure, or fallback routing are implemented.

## Ledger Schema

```sql
CREATE TABLE IF NOT EXISTS session_ledger (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id TEXT NOT NULL,
  chat_id TEXT NOT NULL,
  model_id TEXT NOT NULL,
  opencode_session_id TEXT NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  UNIQUE(user_id, chat_id, model_id)
);
```

## No-Fallback Identity Policy

`POST /v1/chat/completions` requires:

- `X-OpenWebUI-User-Id`
- `X-OpenWebUI-Chat-Id`

The gateway does not use `body.user`, `single-user-local`, `default-chat`, prompt content, or any other fallback identity.

## How To Run Locally

```bash
export GATEWAY_API_KEY=dev-secret
export OPENCODE_BASE_URL=http://127.0.0.1:4096
export DATABASE_PATH=./gateway.sqlite3
export ALLOW_UNSECURED_OPENCODE=true
export REQUIRE_AUTH_ON_HEALTH=false
export REQUEST_TIMEOUT_SECONDS=120
go run ./cmd/gateway
```

## Curl Tests

```bash
curl -sS http://127.0.0.1:8080/health
curl -sS http://127.0.0.1:8080/v1/models -H 'Authorization: Bearer dev-secret'
curl -sS http://127.0.0.1:8080/v1/chat/completions \
  -H 'Authorization: Bearer dev-secret' \
  -H 'Content-Type: application/json' \
  -H 'X-OpenWebUI-User-Id: local-user' \
  -H 'X-OpenWebUI-Chat-Id: local-chat' \
  -d '{"model":"adina-analysis","messages":[{"role":"user","content":"Say hello."}]}'
```

## Open WebUI Connection

Configure an OpenAI-compatible provider with base URL `http://<gateway-host>:8080/v1` and API key `GATEWAY_API_KEY`. Enable forwarded user info headers so `X-OpenWebUI-User-Id` and `X-OpenWebUI-Chat-Id` are sent.

## Known Limitations

- `stream: true` is supported only by a synchronous SSE compatibility shim. It is not true OpenCode streaming.
- No OpenCode `/event`, `/global/event`, or `prompt_async` usage.
- No websockets.
- No MCP or ACP.
- No dynamic model discovery.
- No provider/model override.
- No cancellation.
- No multi-tenant workspace isolation claim.
- Usage is omitted rather than fabricated.
- Dockerized OpenCode requires a Python-capable runtime image for this workspace. The official image produced `Executable not found in $PATH: "python3"` during execution.

## Security Notes

- Gateway Bearer tokens are compared with constant-time comparison.
- Upstream Bearer tokens are never forwarded to OpenCode.
- Basic Auth is sent to OpenCode only when `OPENCODE_SERVER_PASSWORD` is configured.
- Secret values and Authorization headers are not logged.
- Unsecured OpenCode mode is explicitly local-development only.

## Test Results

Validation completed in this working session:

```bash
go test ./...  # 27 passed in 9 packages
go vet ./...   # no issues found
go build ./cmd/gateway  # success
GATEWAY_API_KEY=dev-secret DATABASE_PATH=./gateway.sqlite3 scripts/phase1-smoke.sh  # passed when the finalized gateway is bound to :8080
```

## Remaining Phase 2/3/4 Work

- Phase 2: streaming/SSE mapping and OpenCode event correlation.
- Phase 3: multi-tenant OpenCode routing and per-user workspace isolation.
- Phase 4: cancellation, idle cleanup, metrics, and tracing.
