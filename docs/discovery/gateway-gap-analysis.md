# OpenCode Gateway Gap Analysis

Discovery date: 2026-06-01

Scope: gap analysis between Open WebUI's OpenAI-compatible Chat Completions expectation and OpenCode's documented stateful server API. This is discovery documentation only. No production code is specified here.

## Source policy

Sources used:

- Project roadmap input: `/mnt/data/roadmap.txt`
- Project documentation links input: `/mnt/data/documentation-links.txt`
- OpenCode server docs: `https://opencode.ai/docs/server/`
- Open WebUI OpenAI-compatible provider docs: `https://docs.openwebui.com/getting-started/quick-start/connect-a-provider/starting-with-openai-compatible/`
- Open WebUI environment config docs: `https://docs.openwebui.com/reference/env-configuration/`
- Hermes/Open WebUI reference docs: `https://docs.openwebui.com/getting-started/quick-start/connect-an-agent/hermes-agent/` and `https://github.com/NousResearch/hermes-agent/blob/main/website/docs/user-guide/messaging/open-webui.md`
- OpenClaw/Open WebUI reference docs: `https://docs.openwebui.com/getting-started/quick-start/connect-an-agent/openclaw/` and `https://docs.openclaw.ai/gateway/openai-http-api`

Any requirement not directly documented is marked UNKNOWN.

## Executive conclusion

A gateway is required because Open WebUI expects an OpenAI-compatible Chat Completions interface, while OpenCode exposes a proprietary stateful server API organized around sessions, messages, events, agents, and project/workspace operations. OpenCode does not document a native `GET /v1/models` or `POST /v1/chat/completions` surface. OpenCode's documented bridge primitives are `GET /agent`, `POST /session`, `POST /session/:id/message`, `POST /session/:id/prompt_async`, `GET /event`, message read endpoints, and `POST /session/:id/abort`.

## Interface gap matrix

| Open WebUI / reference gateway expectation | OpenCode documented primitive | Gap | Status | Source URLs |
|---|---|---|---|---|
| `GET /v1/models` for model discovery | `GET /agent` lists available agents | Need translate OpenCode agents into OpenAI model list objects | REQUIRED | https://docs.openwebui.com/getting-started/quick-start/connect-a-provider/starting-with-openai-compatible/ ; https://opencode.ai/docs/server/ |
| `POST /v1/chat/completions` | `POST /session/:id/message` or `POST /session/:id/prompt_async` | Need translate request body and lifecycle | REQUIRED | https://github.com/NousResearch/hermes-agent/blob/main/website/docs/user-guide/messaging/open-webui.md ; https://opencode.ai/docs/server/ |
| Stateless request with full history in Chat Completions mode | Stateful OpenCode session | Need session ledger and latest-turn extraction policy; exact Open WebUI body shape is partly UNKNOWN | REQUIRED | https://github.com/NousResearch/hermes-agent/blob/main/website/docs/user-guide/messaging/open-webui.md ; https://opencode.ai/docs/server/ |
| Streaming SSE when `stream: true` | `GET /event` and `/global/event` SSE streams; `POST /session/:id/prompt_async` for async prompt | Need correlate OpenCode bus events to OpenAI SSE chunks | REQUIRED FOR PHASE 2; correlation schema UNKNOWN | https://docs.openclaw.ai/gateway/openai-http-api ; https://opencode.ai/docs/server/ |
| Auth with Open WebUI API key / Bearer token | OpenCode server Basic Auth with `OPENCODE_SERVER_PASSWORD` | Need two-tier auth: Open WebUI→Gateway Bearer, Gateway→OpenCode Basic | REQUIRED | https://docs.openwebui.com/reference/env-configuration/ ; https://opencode.ai/docs/server/ |
| User/session identity forwarding | Open WebUI can forward `X-OpenWebUI-*` headers when enabled | Need configure `ENABLE_FORWARD_USER_INFO_HEADERS=true`; otherwise multi-user routing identity is UNKNOWN | REQUIRED FOR MULTI-TENANT | https://docs.openwebui.com/reference/env-configuration/ |
| Cancellation | `POST /session/:id/abort` | Need map client abort/cancel to OpenCode abort; exact Open WebUI cancel signal/API UNKNOWN | PARTIAL | https://opencode.ai/docs/server/ |
| Agent/model routing | `GET /agent`, message body has `agent?`, `model?` | Need define model id → OpenCode agent mapping; exact `Agent` fields UNKNOWN until `/doc` verification | REQUIRED | https://opencode.ai/docs/server/ |
| Usage reporting | OpenCode `AssistantMessage.tokens` generated type includes token fields | Need map to OpenAI `usage`; exact message response metadata from live API must be verified | PARTIAL | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |

## Required OpenCode endpoints for MVP

| Capability | Endpoint | Why required | Source URL |
|---|---|---|---|
| Downstream health | `GET /global/health` | Confirms OpenCode is reachable and reports version | https://opencode.ai/docs/server/ |
| Agent discovery | `GET /agent` | Provides available OpenCode agents for `/v1/models` mapping | https://opencode.ai/docs/server/ |
| Session creation | `POST /session` | Creates persistent OpenCode state for an Open WebUI chat/model route | https://opencode.ai/docs/server/ |
| Synchronous message send/response | `POST /session/:id/message` | Non-streaming MVP completion path | https://opencode.ai/docs/server/ |
| Message inspection | `GET /session/:id/message`, `GET /session/:id/message/:messageID` | Debugging, recovery, and possible response fetch path | https://opencode.ai/docs/server/ |
| Cancel | `POST /session/:id/abort` | Aborts a running session | https://opencode.ai/docs/server/ |

## Required OpenCode endpoints for streaming phase

| Capability | Endpoint | Gap | Source URL |
|---|---|---|---|
| Async submit | `POST /session/:id/prompt_async` | Body is documented as same as `/session/:id/message`, but exact body fields need `/doc` verification | https://opencode.ai/docs/server/ |
| Event stream | `GET /event` | Event stream exists; exact event names and message correlation rules need `/doc`/runtime verification | https://opencode.ai/docs/server/ |
| Optional global stream | `GET /global/event` | Global event stream exists; when to prefer over `/event` UNKNOWN | https://opencode.ai/docs/server/ |

## Required gateway-owned contracts

These are not provided by OpenCode; the gateway must own them.

| Contract | Required behavior | Evidence / source |
|---|---|---|
| OpenAI-compatible discovery | Expose `GET /v1/models`; Open WebUI verifies `/models` with Bearer token | https://docs.openwebui.com/getting-started/quick-start/connect-a-provider/starting-with-openai-compatible/ |
| OpenAI-compatible chat | Expose `POST /v1/chat/completions`; Hermes reference uses this path as default Chat Completions mode | https://github.com/NousResearch/hermes-agent/blob/main/website/docs/user-guide/messaging/open-webui.md |
| Streaming | If supporting streaming, emit SSE with `Content-Type: text/event-stream`, `data: <json>`, and `data: [DONE]` termination | https://docs.openclaw.ai/gateway/openai-http-api |
| User headers | Consume `X-OpenWebUI-User-Id`, `X-OpenWebUI-Chat-Id`, and related headers only when forwarding is enabled | https://docs.openwebui.com/reference/env-configuration/ |
| Downstream auth | Use Basic Auth toward OpenCode when `OPENCODE_SERVER_PASSWORD` protects OpenCode server | https://opencode.ai/docs/server/ |

## Session-state gap

Open WebUI Chat Completions mode sends the message and conversation history on each request according to the Hermes guide. OpenCode sessions preserve state server-side. Therefore, replaying the entire Open WebUI history into an existing OpenCode session would duplicate context.

Documented facts:

| Fact | Source URL |
|---|---|
| Open WebUI sends `POST /v1/chat/completions` with message and conversation history in Hermes Chat Completions mode | https://github.com/NousResearch/hermes-agent/blob/main/website/docs/user-guide/messaging/open-webui.md |
| OpenCode has sessions created by `POST /session` and messages sent to a session by `POST /session/:id/message` | https://opencode.ai/docs/server/ |

Gateway ledger requirement:

| Field | Status | Source / reason |
|---|---|---|
| Open WebUI user id | Documented forwarded header `X-OpenWebUI-User-Id` | https://docs.openwebui.com/reference/env-configuration/ |
| Open WebUI chat id | Documented forwarded header `X-OpenWebUI-Chat-Id` | https://docs.openwebui.com/reference/env-configuration/ |
| Open WebUI message id | Documented forwarded header `X-OpenWebUI-Message-Id` | https://docs.openwebui.com/reference/env-configuration/ |
| Model id | Documented OpenAI-compatible request field in OpenClaw agent-first contract; Open WebUI exact field list UNKNOWN but Chat Completions implies standard request | https://docs.openclaw.ai/gateway/openai-http-api |
| OpenCode session id | Returned in `Session.id` generated type | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| Timestamps | Gateway-owned; not dictated by source docs | UNKNOWN as external contract |

## Request translation gaps

### `POST /v1/chat/completions` → `POST /session/:id/message`

| Input concept | Target OpenCode field | Status | Source URL |
|---|---|---|---|
| Selected Open WebUI model/agent target | `agent?` and/or `model?` in OpenCode message request | OpenCode fields documented by name only; exact type UNKNOWN | https://opencode.ai/docs/server/ |
| Conversation messages | `parts` and possibly `system` in OpenCode message request | OpenCode fields documented by name only; exact `parts` element schema UNKNOWN | https://opencode.ai/docs/server/ |
| System instruction | `system?` | Field documented by name only; type UNKNOWN from public server page | https://opencode.ai/docs/server/ |
| Provider/model override | `model?` | Field documented by name only; exact object schema UNKNOWN | https://opencode.ai/docs/server/ |
| Tool availability | `tools?` | Field documented by name only; generated `UserMessage.tools` is `{ [key: string]: boolean }`, but request schema still needs `/doc` verification | https://opencode.ai/docs/server/ ; https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |

## Response translation gaps

| OpenCode source | OpenAI-compatible target | Gap | Source URL |
|---|---|---|---|
| `{ info: Message, parts: Part[] }` from `POST /session/:id/message` | Chat completion response | Need flatten `TextPart.text`; decide handling of `ReasoningPart`, `ToolPart`, `PatchPart`; exact expected OpenAI response fields UNKNOWN from Open WebUI docs | https://opencode.ai/docs/server/ ; https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `AssistantMessage.tokens` | `usage` | Generated type has `input`, `output`, `reasoning`, `cache.read`, `cache.write`; mapping to OpenAI usage fields must be explicitly defined by gateway | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| OpenCode events | Streaming chunks | Need map `message.part.updated` delta to OpenAI SSE delta; exact stream contract must be verified | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts ; https://docs.openclaw.ai/gateway/openai-http-api |

## Security gaps

| Area | Gap | Required classification | Source URL |
|---|---|---|---|
| Open WebUI → Gateway auth | Open WebUI stores/sends API key; gateway must validate Bearer token. Exact retry/verification behavior beyond `/models` UNKNOWN | REQUIRED | https://docs.openwebui.com/reference/env-configuration/ ; https://docs.openwebui.com/getting-started/quick-start/connect-a-provider/starting-with-openai-compatible/ |
| Gateway → OpenCode auth | OpenCode uses Basic Auth if password env var set | REQUIRED | https://opencode.ai/docs/server/ |
| Operator surface risk | OpenClaw warns OpenAI-compatible agent endpoint can be full operator-access surface; analogous risk applies if OpenCode agent has write/shell tools enabled | RISK; policy must be explicit | https://docs.openclaw.ai/gateway/openai-http-api |
| User isolation | Open WebUI can forward user headers; OpenCode server itself does not document per-user isolation in `opencode serve` page | UNKNOWN / GATEWAY RESPONSIBILITY | https://docs.openwebui.com/reference/env-configuration/ ; https://opencode.ai/docs/server/ |

## Unknowns that must block implementation until verified

1. Exact OpenCode `POST /session/:id/message` request body schema from live `/doc`.
2. Exact OpenCode `POST /session/:id/prompt_async` request body schema from live `/doc`.
3. Exact `GET /agent` response fields.
4. Whether `agent` names from `GET /agent` can be safely used directly as `/v1/models` ids or need namespacing.
5. Exact OpenCode SSE event sequence for one async prompt.
6. Exact OpenCode cancellation behavior: whether `POST /session/:id/abort` aborts only the active message, all work in session, or broader session execution.
7. Exact Open WebUI cancel behavior for external OpenAI-compatible providers.
8. Exact Open WebUI Chat Completions request body fields emitted in current configured deployment.
9. Exact non-streaming Chat Completions response fields Open WebUI requires beyond content display.
10. Whether Open WebUI forwards `X-OpenWebUI-*` headers to OpenAI connections in the target deployment; requires `ENABLE_FORWARD_USER_INFO_HEADERS=true` and runtime verification.

## Discovery validation commands for next phase

These are audit commands, not production code:

```bash
# OpenCode docs/spec verification
curl -sS -u "$OPENCODE_SERVER_USERNAME:$OPENCODE_SERVER_PASSWORD" \
  http://127.0.0.1:4096/global/health

curl -sS -u "$OPENCODE_SERVER_USERNAME:$OPENCODE_SERVER_PASSWORD" \
  http://127.0.0.1:4096/doc

curl -sS -u "$OPENCODE_SERVER_USERNAME:$OPENCODE_SERVER_PASSWORD" \
  http://127.0.0.1:4096/agent

curl -sS -u "$OPENCODE_SERVER_USERNAME:$OPENCODE_SERVER_PASSWORD" \
  -X POST http://127.0.0.1:4096/session \
  -H 'Content-Type: application/json' \
  -d '{"title":"gateway-discovery"}'
```

## Minimal phase-1 acceptance gates

1. `docs/discovery/opencode-api-audit.md` lists all OpenCode endpoints documented on the public server page.
2. `docs/discovery/openwebui-contract.md` documents `/v1/models`, `/v1/chat/completions`, streaming, and authentication with source URLs.
3. `docs/discovery/gateway-gap-analysis.md` marks each unresolved request/response schema field as UNKNOWN or TYPE REFERENCE ONLY.
4. No endpoint, request field, response field, or behavior is presented as fact without a source URL.
5. No production gateway code is produced.

