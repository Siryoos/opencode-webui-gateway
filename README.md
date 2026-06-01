# opencode-webui-gateway

OpenAI-compatible gateway that lets Open WebUI use OpenCode through a narrow `/v1` provider surface.

## Phase 1 Scope

Non-streaming MVP only:

- `GET /health`
- `GET /v1/models`
- `POST /v1/chat/completions`
- Bearer auth from Open WebUI to Gateway
- Basic Auth from Gateway to OpenCode
- Static model routing
- SQLite-backed Session Ledger
- No streaming
- No MCP
- No ACP
- No multi-tenant container routing

## Documentation

- `docs/roadmap.txt`
- `docs/discovery/opencode-api-audit.md`
- `docs/discovery/openwebui-contract.md`
- `docs/discovery/gateway-gap-analysis.md`
- `docs/adr/001-gateway-architecture.md`
- `docs/design/openai-compatibility-layer.md`
- `docs/design/session-ledger.md`
- `docs/design/model-routing.md`
- `docs/security/authentication.md`
