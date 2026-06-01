# Open WebUI OpenAI-Compatible Contract

Discovery date: 2026-06-01

Scope: exact documented Open WebUI expectations for connecting an external OpenAI-compatible provider/gateway, plus Hermes and OpenClaw reference behavior where Open WebUI documentation explicitly presents them as agent gateway integrations.

## Source policy

Open WebUI states that it focuses on standard protocols such as the OpenAI Chat Completions Protocol and expects backends to follow the universal Chat Completions standard. Where Open WebUI documentation does not define a field-level wire schema, this document marks field-level details as UNKNOWN rather than importing unstated OpenAI API fields.

Primary sources:

- Open WebUI OpenAI-compatible provider docs: `https://docs.openwebui.com/getting-started/quick-start/connect-a-provider/starting-with-openai-compatible/`
- Open WebUI environment configuration: `https://docs.openwebui.com/reference/env-configuration/`
- Open WebUI Hermes Agent integration: `https://docs.openwebui.com/getting-started/quick-start/connect-an-agent/hermes-agent/`
- Hermes Agent Open WebUI guide: `https://github.com/NousResearch/hermes-agent/blob/main/website/docs/user-guide/messaging/open-webui.md`
- Open WebUI OpenClaw integration: `https://docs.openwebui.com/getting-started/quick-start/connect-an-agent/openclaw/`
- OpenClaw OpenAI-compatible gateway docs: `https://docs.openclaw.ai/gateway/openai-http-api`

## Integration mode

| Requirement | Documented value | Source URL |
|---|---|---|
| Open WebUI integration style | External backend follows OpenAI Chat Completions Protocol | https://docs.openwebui.com/getting-started/quick-start/connect-a-provider/starting-with-openai-compatible/ |
| Unsupported proprietary backend handling | Use a pipe or middleware proxy when backend does not implement standard protocol | https://docs.openwebui.com/getting-started/quick-start/connect-a-provider/starting-with-openai-compatible/ |
| Admin UI location | Admin Settings → Connections → OpenAI → Add Connection | https://docs.openwebui.com/getting-started/quick-start/connect-a-provider/starting-with-openai-compatible/ |
| Connection fields | URL and API Key | https://docs.openwebui.com/getting-started/quick-start/connect-a-provider/starting-with-openai-compatible/ |
| Docker host access note | Replace `localhost` with `host.docker.internal` when Open WebUI container calls a model server on the host | https://docs.openwebui.com/getting-started/quick-start/connect-a-provider/starting-with-openai-compatible/ |

## Base URL and authentication configuration

| Config | Type | Default / behavior | Source URL |
|---|---|---|---|
| `OPENAI_API_BASE_URL` | `str` | Default `https://api.openai.com/v1`; configures OpenAI base API URL | https://docs.openwebui.com/reference/env-configuration/ |
| `OPENAI_API_BASE_URLS` | `str` | Semicolon-separated balanced OpenAI base API URLs | https://docs.openwebui.com/reference/env-configuration/ |
| `OPENAI_API_KEY` | `str` | Sets OpenAI API key | https://docs.openwebui.com/reference/env-configuration/ |
| Admin UI API key | API key entered for connection | Used by Open WebUI for provider authentication; exact header shape is documented for model verification as Bearer token | https://docs.openwebui.com/getting-started/quick-start/connect-a-provider/starting-with-openai-compatible/ |
| `/models` verification auth | Standard `Bearer` token | Open WebUI verifies connection by calling provider `/models` endpoint using a standard Bearer token | https://docs.openwebui.com/getting-started/quick-start/connect-a-provider/starting-with-openai-compatible/ |

## User/session forwarding headers

Open WebUI can forward user and session information to OpenAI API, Ollama API, MCP servers, and Tool servers when `ENABLE_FORWARD_USER_INFO_HEADERS=true`.

| Header | Default name | Source URL |
|---|---|---|
| User name | `X-OpenWebUI-User-Name` | https://docs.openwebui.com/reference/env-configuration/ |
| User id | `X-OpenWebUI-User-Id` | https://docs.openwebui.com/reference/env-configuration/ |
| User email | `X-OpenWebUI-User-Email` | https://docs.openwebui.com/reference/env-configuration/ |
| User role | `X-OpenWebUI-User-Role` | https://docs.openwebui.com/reference/env-configuration/ |
| Chat/session id | `X-OpenWebUI-Chat-Id` | https://docs.openwebui.com/reference/env-configuration/ |
| Message id | `X-OpenWebUI-Message-Id` | https://docs.openwebui.com/reference/env-configuration/ |

Documented use: per-user authorization, auditing, rate limiting, request tracing, and external tool event emitting.

## Required gateway endpoint: `GET /v1/models`

### Open WebUI requirement

Open WebUI verifies an OpenAI-compatible connection by calling the provider's `/models` endpoint with a standard Bearer token. If `/models` is unavailable or non-standard, verification can fail even if chat completions still work; in that case the admin can manually allowlist model IDs.

| Requirement | Status | Source URL |
|---|---|---|
| Path | `/models` relative to configured base URL; for a base URL ending in `/v1`, effective path is `/v1/models` | https://docs.openwebui.com/getting-started/quick-start/connect-a-provider/starting-with-openai-compatible/ |
| Method | GET | https://docs.openwebui.com/getting-started/quick-start/connect-a-provider/starting-with-openai-compatible/ |
| Auth | Standard Bearer token for verification | https://docs.openwebui.com/getting-started/quick-start/connect-a-provider/starting-with-openai-compatible/ |
| Response body field-level schema expected by Open WebUI | UNKNOWN in Open WebUI docs | https://docs.openwebui.com/getting-started/quick-start/connect-a-provider/starting-with-openai-compatible/ |

### Reference agent gateway behavior

| Reference | Documented model behavior | Source URL |
|---|---|---|
| Hermes | `GET /v1/models` should list `hermes-agent`; sample response begins `{ "object":"list","data":[{"id":"hermes-agent", ...}]}` | https://github.com/NousResearch/hermes-agent/blob/main/website/docs/user-guide/messaging/open-webui.md |
| OpenClaw | `GET /v1/models` returns an OpenClaw agent-target list; returned ids are `openclaw`, `openclaw/default`, and `openclaw/<agentId>` | https://docs.openclaw.ai/gateway/openai-http-api |

Minimum documented response fields from reference examples:

| Field | Type / value | Source URL |
|---|---|---|
| `object` | `"list"` in Hermes sample | https://github.com/NousResearch/hermes-agent/blob/main/website/docs/user-guide/messaging/open-webui.md |
| `data[].id` | model/agent target id; Hermes sample `hermes-agent`; OpenClaw ids `openclaw`, `openclaw/default`, `openclaw/<agentId>` | https://github.com/NousResearch/hermes-agent/blob/main/website/docs/user-guide/messaging/open-webui.md ; https://docs.openclaw.ai/gateway/openai-http-api |

Other model object fields: UNKNOWN from listed sources.

## Required gateway endpoint: `POST /v1/chat/completions`

### Open WebUI requirement

| Requirement | Documented value | Source URL |
|---|---|---|
| Protocol | OpenAI Chat Completions Protocol | https://docs.openwebui.com/getting-started/quick-start/connect-a-provider/starting-with-openai-compatible/ |
| Path | `/v1/chat/completions` for Chat Completions mode | https://github.com/NousResearch/hermes-agent/blob/main/website/docs/user-guide/messaging/open-webui.md |
| Request history behavior | Open WebUI sends the message and conversation history in Chat Completions mode | https://github.com/NousResearch/hermes-agent/blob/main/website/docs/user-guide/messaging/open-webui.md |
| API mode | Chat Completions is default/recommended in Hermes guide | https://github.com/NousResearch/hermes-agent/blob/main/website/docs/user-guide/messaging/open-webui.md |

### Documented request fields from sources

Open WebUI docs do not enumerate all Chat Completions fields. OpenClaw's OpenAI-compatible gateway documentation explicitly lists supported request fields for its Chat Completions surface; these are reference behavior, not an Open WebUI-owned exhaustive schema.

| Field | Type / constraint | Source URL |
|---|---|---|
| `model` | Agent target in OpenClaw; e.g. `openclaw`, `openclaw/default`, `openclaw/<agentId>` | https://docs.openclaw.ai/gateway/openai-http-api |
| `user` | If present in OpenClaw, used to derive stable session key | https://docs.openclaw.ai/gateway/openai-http-api |
| `stream` | `true` enables SSE streaming in OpenClaw | https://docs.openclaw.ai/gateway/openai-http-api |
| `tools` | array of `{ "type": "function", "function": { ... } }` | https://docs.openclaw.ai/gateway/openai-http-api |
| `tool_choice` | `"auto"`, `"none"`, `"required"`, or `{ "type": "function", "function": { "name": "..." } }` | https://docs.openclaw.ai/gateway/openai-http-api |
| `messages[*].role` | `"tool"` follow-up turns are supported by OpenClaw; other role values are not enumerated in the listed sources | https://docs.openclaw.ai/gateway/openai-http-api |
| `messages[*].tool_call_id` | binds tool results to a prior tool call | https://docs.openclaw.ai/gateway/openai-http-api |
| `max_completion_tokens` | number; preferred over `max_tokens` when both sent | https://docs.openclaw.ai/gateway/openai-http-api |
| `max_tokens` | number; legacy alias accepted | https://docs.openclaw.ai/gateway/openai-http-api |
| `temperature` | number; best-effort forwarded | https://docs.openclaw.ai/gateway/openai-http-api |
| `top_p` | number; best-effort forwarded | https://docs.openclaw.ai/gateway/openai-http-api |
| `frequency_penalty` | number, range `-2.0` to `2.0`; out-of-range returns `400 invalid_request_error` in OpenClaw | https://docs.openclaw.ai/gateway/openai-http-api |
| `presence_penalty` | number, range `-2.0` to `2.0`; out-of-range returns `400 invalid_request_error` in OpenClaw | https://docs.openclaw.ai/gateway/openai-http-api |
| `seed` | integer number; non-integer returns `400 invalid_request_error` in OpenClaw | https://docs.openclaw.ai/gateway/openai-http-api |
| `stop` | string or array of up to 4 strings; invalid entries return `400 invalid_request_error` in OpenClaw | https://docs.openclaw.ai/gateway/openai-http-api |
| `stream_options.include_usage` | if true, OpenClaw emits trailing usage chunk before `[DONE]` | https://docs.openclaw.ai/gateway/openai-http-api |

Fields required by Open WebUI specifically: UNKNOWN in listed Open WebUI docs beyond the general Chat Completions protocol expectation.

### Documented response fields from sources

Open WebUI docs do not enumerate the full Chat Completions response schema. OpenClaw documents tool-call-specific response shapes.

| Response shape | Field | Type / value | Source URL |
|---|---|---|---|
| Non-streaming tool response | `choices[0].finish_reason` | `"tool_calls"` | https://docs.openclaw.ai/gateway/openai-http-api |
| Non-streaming tool response | `choices[0].message.tool_calls[].id` | UNKNOWN scalar type, field documented | https://docs.openclaw.ai/gateway/openai-http-api |
| Non-streaming tool response | `choices[0].message.tool_calls[].type` | `"function"` | https://docs.openclaw.ai/gateway/openai-http-api |
| Non-streaming tool response | `choices[0].message.tool_calls[].function.name` | UNKNOWN scalar type, field documented | https://docs.openclaw.ai/gateway/openai-http-api |
| Non-streaming tool response | `choices[0].message.tool_calls[].function.arguments` | JSON string | https://docs.openclaw.ai/gateway/openai-http-api |
| Non-streaming tool response | `choices[0].message.content` | assistant commentary before tool call, possibly empty | https://docs.openclaw.ai/gateway/openai-http-api |
| Streaming tool response | `delta.tool_calls` | incremental chunks carrying tool identity and argument fragments | https://docs.openclaw.ai/gateway/openai-http-api |
| Streaming final chunk | `finish_reason` | `"tool_calls"` when tool call emitted | https://docs.openclaw.ai/gateway/openai-http-api |

All non-tool response fields such as `id`, `object`, `created`, `model`, `choices[].message.role`, `choices[].message.content`, and `usage`: UNKNOWN from the Open WebUI docs and not fully specified by the listed reference docs. Do not treat them as accepted until verified against Open WebUI behavior or an explicit OpenAI-compatible schema source approved for the project.

## Streaming requirements

| Requirement | Documented value | Source URL |
|---|---|---|
| Streaming transport | Server-Sent Events | https://docs.openclaw.ai/gateway/openai-http-api |
| Activation | `stream: true` | https://docs.openclaw.ai/gateway/openai-http-api |
| Response content type | `text/event-stream` | https://docs.openclaw.ai/gateway/openai-http-api |
| Event line format | `data: <json>` | https://docs.openclaw.ai/gateway/openai-http-api |
| Termination | `data: [DONE]` | https://docs.openclaw.ai/gateway/openai-http-api |
| Usage in stream | If `stream_options.include_usage=true`, trailing usage chunk before `[DONE]` | https://docs.openclaw.ai/gateway/openai-http-api |
| Hermes behavior | Streaming is enabled by default and shows inline progress indicators before final response | https://docs.openwebui.com/getting-started/quick-start/connect-an-agent/hermes-agent/ |
| Hermes request flow | Open WebUI sends `POST /v1/chat/completions`; response streams back to Open WebUI | https://github.com/NousResearch/hermes-agent/blob/main/website/docs/user-guide/messaging/open-webui.md |

Exact chunk JSON schema for normal text deltas: UNKNOWN from Open WebUI docs and listed reference docs, except for OpenClaw tool-call chunks above.

## Authentication requirements

| Area | Requirement | Source URL |
|---|---|---|
| Open WebUI connection to gateway | API Key configured in connection settings or `OPENAI_API_KEY` | https://docs.openwebui.com/reference/env-configuration/ |
| Verification auth | `/models` called with standard Bearer token | https://docs.openwebui.com/getting-started/quick-start/connect-a-provider/starting-with-openai-compatible/ |
| Hermes reference | `API_SERVER_KEY` is entered as the Open WebUI API key | https://docs.openwebui.com/getting-started/quick-start/connect-an-agent/hermes-agent/ |
| Hermes verification | `Authorization: Bearer your-secret-key` against `/v1/models`; 401 if key mismatch | https://github.com/NousResearch/hermes-agent/blob/main/website/docs/user-guide/messaging/open-webui.md |
| OpenClaw reference | Shared-secret auth uses `Authorization: Bearer <token-or-password>` | https://docs.openclaw.ai/gateway/openai-http-api |
| OpenClaw reference | Trusted proxy mode can inject identity headers; private ingress open auth can require no auth header | https://docs.openclaw.ai/gateway/openai-http-api |

## Contract implications for OpenCode Gateway

1. The gateway must expose a `/v1`-based OpenAI-compatible surface to Open WebUI.
2. `GET /v1/models` is needed for smooth Open WebUI model discovery. If not implemented or if non-standard, users must manually allowlist model IDs.
3. `POST /v1/chat/completions` is the primary required chat endpoint.
4. Streaming must use SSE when `stream: true` and terminate with `data: [DONE]` if emulating OpenClaw/Hermes behavior.
5. User/session identity should come from forwarded headers only when `ENABLE_FORWARD_USER_INFO_HEADERS=true`; otherwise identity headers are unavailable and session routing must be considered UNKNOWN or single-user.
6. Open WebUI sends full conversation history in Chat Completions mode according to the Hermes guide; a stateful OpenCode gateway must avoid blindly replaying history into an existing OpenCode session.

