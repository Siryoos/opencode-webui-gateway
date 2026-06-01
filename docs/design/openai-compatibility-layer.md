# OpenAI Compatibility Layer Design

- **Document:** `docs/design/openai-compatibility-layer.md`
- **Status:** Proposed
- **Date:** 2026-06-01
- **Owner role:** Senior Backend Engineer
- **Scope:** API contract design only. No implementation code.

## 1. Source policy

This design uses the existing project roadmap and discovery artifacts for Open WebUI, OpenCode, gateway topology, session lifecycle, authentication, and known unknowns.

The OpenAI field inventory is taken from the official OpenAI API reference for Chat Completions and Models because the Open WebUI discovery artifact explicitly does not enumerate the full Chat Completions wire schema.

This document does **not** define undocumented OpenCode schemas as facts. Any OpenCode request or response field whose exact type is not verified in the discovery artifacts remains `UNKNOWN`.

## 2. Non-goals

- No production code.
- No framework choice.
- No handler implementation.
- No persistence implementation.
- No OpenAI Assistants API.
- No OpenAI Responses API.
- No raw OpenCode API pass-through.
- No MCP or ACP gateway integration path.
- No claim of streaming support in Phase 1.
- No claim of multi-tenant isolation unless forwarded Open WebUI identity headers and downstream workspace isolation are verified.

## 3. Compatibility posture

The gateway exposes a narrow OpenAI-compatible surface to Open WebUI and translates that surface into OpenCode's stateful session API.

Required public endpoints:

| Endpoint | Phase | Purpose |
|---|---:|---|
| `GET /health` | 1 | Gateway health plus downstream OpenCode reachability. |
| `GET /v1/models` | 1 | Expose gateway-owned model IDs backed by OpenCode agents. |
| `POST /v1/chat/completions` | 1 | Non-streaming Chat Completions-compatible request/response translation. |

Downstream OpenCode endpoints used by this design:

| Gateway capability | OpenCode endpoint | Mapping status |
|---|---|---|
| Downstream health | `GET /global/health` | Documented. |
| Agent discovery / validation | `GET /agent` | Documented, but exact `Agent` schema is `UNKNOWN`. |
| Session creation | `POST /session` | Documented, but exact `Session` field-level schema must be verified before implementation. |
| Synchronous message execution | `POST /session/:id/message` | Documented, but request fields `messageID`, `model`, `agent`, `noReply`, `system`, `tools`, and `parts` have exact type gaps. |
| Future streaming submit | `POST /session/:id/prompt_async` | Out of Phase 1; exact schema must be verified. |
| Future streaming events | `GET /event` or `GET /global/event` | Out of Phase 1; event correlation is `UNKNOWN`. |
| Future cancellation | `POST /session/:id/abort` | Out of Phase 1; exact cancellation semantics are `UNKNOWN`. |

## 4. Shared authentication rules

### 4.1 Open WebUI to Gateway

| Rule | Design |
|---|---|
| Authentication scheme | Bearer token. |
| Required header | `Authorization: Bearer <gateway-api-key>`. |
| Applies to | `GET /v1/models` and `POST /v1/chat/completions`. |
| `GET /health` | May be unauthenticated only when bound to a trusted internal network; otherwise use the same Bearer rule. Deployment must choose explicitly. |
| Missing credentials | Reject before any OpenCode call. |
| Invalid credentials | Reject before any OpenCode call. |
| Forwarded Open WebUI user headers | Routing metadata only, never authentication proof. |

### 4.2 Gateway to OpenCode

| Rule | Design |
|---|---|
| Authentication scheme | HTTP Basic Auth when OpenCode is protected by `OPENCODE_SERVER_PASSWORD`. |
| Credential ownership | Gateway-owned secret. |
| Exposure rule | Never expose OpenCode credentials to Open WebUI, browser clients, request logs visible to users, or model-visible content. |
| Downstream auth failure | Normalize to a gateway error response; do not pass raw OpenCode credentials or full raw downstream error bodies upstream. |

## 5. Shared error envelope

The gateway uses one OpenAI-style error envelope for provider-facing failures.

| Field | Type | Required | Meaning |
|---|---|---:|---|
| `error.message` | string | Yes | Human-readable sanitized error summary. |
| `error.type` | string | Yes | High-level class such as `invalid_request_error`, `authentication_error`, or `server_error`. |
| `error.param` | string or null | Yes | Request field path when the error is parameter-specific; otherwise null. |
| `error.code` | string or null | Yes | Stable gateway error code when available; otherwise null. |

Error normalization rules:

| Rule | Design |
|---|---|
| No fake success | Do not return HTTP 200 with an assistant message when the gateway or OpenCode failed. |
| No raw pass-through | Do not expose raw OpenCode errors directly as the OpenAI-compatible contract. |
| Sanitization | Downstream filesystem paths, credentials, shell output, internal hostnames, and stack traces must not be exposed unless explicitly classified safe. |
| UNKNOWN downstream errors | Preserve the category and status where safe; mark exact mapping as blocked until OpenCode live `/doc` and runtime behavior are verified. |

Canonical error classes:

| HTTP status | `error.type` | `error.code` | Trigger |
|---:|---|---|---|
| 400 | `invalid_request_error` | `invalid_json` | Body is not valid JSON where a JSON body is required. |
| 400 | `invalid_request_error` | `missing_required_parameter` | Required field is absent. |
| 400 | `invalid_request_error` | `invalid_type` | Field has the wrong JSON type. |
| 400 | `invalid_request_error` | `invalid_value` | Field has unsupported enum/range/value. |
| 400 | `invalid_request_error` | `unsupported_parameter` | Field is valid OpenAI syntax but unsupported by this gateway phase. |
| 401 | `authentication_error` | `missing_api_key` | Bearer token is absent. |
| 401 | `authentication_error` | `invalid_api_key` | Bearer token is present but invalid. |
| 404 | `invalid_request_error` | `model_not_found` | `model` does not map to a configured gateway model. |
| 502 | `server_error` | `opencode_bad_gateway` | OpenCode returns an unexpected or unusable response. |
| 503 | `server_error` | `opencode_unavailable` | OpenCode is unreachable or unhealthy. |
| 504 | `server_error` | `opencode_timeout` | OpenCode request times out. |

## 6. Endpoint: `GET /health`

### 6.1 Purpose

`GET /health` reports whether the gateway process is alive and whether downstream OpenCode is reachable through `GET /global/health`.

This endpoint is gateway-owned. It is not an OpenAI endpoint.

### 6.2 Request schema

| Component | Schema |
|---|---|
| Path params | None. |
| Query params | None. |
| Request body | None. |
| Required headers | Deployment-dependent; see shared authentication rules. |

### 6.3 Response schema

| Field | Type | Required | Source / ownership | Meaning |
|---|---|---:|---|---|
| `status` | enum: `ok`, `degraded`, `down` | Yes | Gateway-owned | Overall health. |
| `gateway.status` | enum: `ok`, `down` | Yes | Gateway-owned | Gateway process readiness. |
| `opencode.status` | enum: `ok`, `unreachable`, `unhealthy`, `unknown` | Yes | Gateway-owned | Downstream OpenCode health classification. |
| `opencode.healthy` | boolean or null | Yes | From OpenCode `/global/health` when available; otherwise null | Downstream `healthy` value. |
| `opencode.version` | string or null | Yes | From OpenCode `/global/health` when available; otherwise null | Downstream OpenCode version. |

### 6.4 Validation rules

| Rule | Behavior |
|---|---|
| Request body present | Ignore body or reject as invalid by deployment policy; recommended: reject with 400 to keep endpoint deterministic. |
| Query params present | Ignore unknown query params; health checks often add probes. |
| Downstream unavailable | Return `status: degraded` or `status: down` according to readiness policy. |
| Readiness vs liveness | Liveness may be `ok` while downstream OpenCode is unavailable; readiness must be `degraded` or `down` when OpenCode cannot serve requests. |

### 6.5 Authentication rules

| Rule | Behavior |
|---|---|
| Internal-only deployment | May allow unauthenticated health checks. |
| Public or shared network deployment | Must require the same Bearer authentication as provider endpoints. |
| Downstream auth | Gateway uses OpenCode Basic Auth when configured; failure must not expose credentials. |

### 6.6 Error responses

| Status | Trigger | Error envelope |
|---:|---|---|
| 401 | Missing or invalid Bearer token when health auth is enabled. | Shared error envelope. |
| 503 | Gateway process is alive but OpenCode is unreachable and readiness policy requires downstream availability. | Shared error envelope or health response with `status: down`, by deployment policy. |

## 7. Endpoint: `GET /v1/models`

### 7.1 Purpose

Expose a model list consumable by Open WebUI. Public model IDs are gateway-owned and map to curated OpenCode agent capability profiles. Raw OpenCode agents must not be exposed automatically until `GET /agent` response fields and permissions are verified.

### 7.2 Request schema

| Component | Schema |
|---|---|
| Path params | None. |
| Query params | None. |
| Request body | None. |
| Required headers | `Authorization: Bearer <gateway-api-key>`. |

### 7.3 Response schema

OpenAI-compatible model list:

| Field | Type | Required | Mapping |
|---|---|---:|---|
| `object` | literal `list` | Yes | Gateway-owned OpenAI-compatible wrapper. |
| `data` | array of model objects | Yes | Gateway-owned configured model list. |
| `data[].id` | string | Yes | Public gateway model ID. Maps to configured OpenCode agent/profile. |
| `data[].object` | literal `model` | Yes | OpenAI-compatible model object field. |
| `data[].created` | integer Unix timestamp | Yes | Gateway-owned static/config timestamp. No OpenCode equivalent verified. |
| `data[].owned_by` | string | Yes | Gateway-owned owner label, recommended value: gateway project identifier. No OpenCode equivalent verified. |

### 7.4 OpenAI model field mapping

| OpenAI field | OpenCode equivalent | Gateway disposition |
|---|---|---|
| `object` | None | Return `list`. |
| `data[]` | `GET /agent` can validate configured agents, but exact `Agent` schema is `UNKNOWN`. | Gateway-owned curated list. |
| `data[].id` | OpenCode `agent` candidate, but exact agent ID/name field is `UNKNOWN`. | Public ID maps through gateway config; do not expose raw OpenCode IDs by default. |
| `data[].object` | None | Return `model`. |
| `data[].created` | UNKNOWN | Gateway-owned value. |
| `data[].owned_by` | UNKNOWN | Gateway-owned value. |

### 7.5 Validation rules

| Rule | Behavior |
|---|---|
| Missing or invalid Bearer token | Reject with 401. |
| Request body present | Reject with 400 `invalid_request_error`. |
| No configured models | Return `object: list` with `data: []` only if this is an intentional safe degraded mode; otherwise return 503. |
| Configured model maps to missing OpenCode agent | Do not expose that model unless configured as offline/disabled; recommended: fail readiness and omit from list. |
| Raw OpenCode agent has unknown permissions | Do not expose automatically. |

### 7.6 Authentication rules

| Rule | Behavior |
|---|---|
| Upstream auth | Required Bearer token. |
| Downstream auth | Use OpenCode Basic Auth if the gateway validates models against `GET /agent`. |
| Auth failure to OpenCode | Return 503 or omit dynamic validation by explicit deployment policy; never leak Basic Auth failure details. |

### 7.7 Error responses

| Status | Trigger | Error |
|---:|---|---|
| 400 | Request body is present and strict mode is enabled. | `invalid_request_error`, `invalid_request_body`. |
| 401 | Missing or invalid Bearer token. | `authentication_error`. |
| 503 | Model mapping cannot be loaded or OpenCode validation is required but unavailable. | `server_error`, `opencode_unavailable`. |

## 8. Endpoint: `POST /v1/chat/completions`

### 8.1 Purpose

Receive Open WebUI Chat Completions requests, route them to a persistent OpenCode session, send only the newest user turn to OpenCode, flatten eligible OpenCode text parts, and return a non-streaming OpenAI-compatible Chat Completion response.

Phase 1 is synchronous and non-streaming. `stream: true` is rejected until the Phase 2 streaming contract is verified.

### 8.2 Request schema: top-level fields

Required OpenAI fields:

| Field | Type | Required | Phase 1 validation |
|---|---|---:|---|
| `model` | string | Yes | Must match a configured public gateway model ID. |
| `messages` | array of message objects | Yes | Must contain at least one usable latest `user` message with text-only content. |

Optional OpenAI fields accounted for in Phase 1:

| Field | Type | Phase 1 validation |
|---|---|---|
| `audio` | object | Unsupported unless absent. Required only by OpenAI when audio output is requested, which this gateway does not support in Phase 1. |
| `frequency_penalty` | number | Accept if within `[-2, 2]`; ignored. |
| `function_call` | string or object | Unsupported. Deprecated by OpenAI in favor of `tool_choice`. |
| `functions` | array | Unsupported. Deprecated by OpenAI in favor of `tools`. |
| `logit_bias` | map | Unsupported. |
| `logprobs` | boolean | Unsupported when true; ignored when false or absent. |
| `max_completion_tokens` | number | Accepted if non-negative integer-like number; OpenCode mapping is `UNKNOWN`; ignored in Phase 1 unless live `/doc` verifies a downstream equivalent. |
| `max_tokens` | number | Deprecated OpenAI alias; same handling as `max_completion_tokens`. |
| `metadata` | object | Accepted for tracing/audit only; not forwarded to OpenCode. Must obey OpenAI shape if validated: up to 16 string key-value pairs, key length <= 64, value length <= 512. |
| `modalities` | array of `text` or `audio` | Must be absent or exactly text-only. Audio unsupported. |
| `n` | number | Must be absent or `1`. Values other than `1` unsupported. |
| `parallel_tool_calls` | boolean | Unsupported when true; ignored when false or absent. |
| `prediction` | object | Unsupported. |
| `presence_penalty` | number | Accept if within `[-2, 2]`; ignored. |
| `prompt_cache_key` | string | Ignored. |
| `prompt_cache_retention` | enum: `in_memory`, `24h` | Ignored. |
| `reasoning_effort` | enum | OpenCode equivalent is `UNKNOWN`; ignored in Phase 1. |
| `response_format` | object | `type: text` is accepted and ignored. `json_object` and `json_schema` are unsupported because no verified OpenCode constrained-output mapping exists. |
| `safety_identifier` | string | Accepted for audit/rate-limit policy only; not forwarded to OpenCode. |
| `seed` | number | Accepted only if integer-like; ignored because deterministic sampling mapping is `UNKNOWN`. |
| `service_tier` | enum | Ignored. |
| `stop` | string or array of string | Accepted if string or array of up to 4 strings; ignored because OpenCode stop mapping is `UNKNOWN`. |
| `store` | boolean | Ignored; gateway does not use OpenAI storage/distillation semantics. |
| `stream` | boolean | Must be absent or false in Phase 1. `true` is unsupported. |
| `stream_options` | object | Allowed only when `stream: true`; since streaming is unsupported in Phase 1, reject if present. |
| `temperature` | number | Accept if within `[0, 2]`; ignored because OpenCode sampling mapping is `UNKNOWN`. |
| `tool_choice` | string or object | Unsupported. |
| `tools` | array | Unsupported as OpenAI external tool definitions. OpenCode internal tools are configured through OpenCode agent profiles, not OpenAI tool calls. |
| `top_logprobs` | number | Unsupported. |
| `top_p` | number | Accept if within `[0, 1]`; ignored because OpenCode sampling mapping is `UNKNOWN`. |
| `user` | string | Accepted only as optional routing fallback if forwarded Open WebUI headers are unavailable and deployment explicitly allows it; otherwise ignored. Deprecated by OpenAI in favor of `safety_identifier` and `prompt_cache_key`. |
| `verbosity` | enum: `low`, `medium`, `high` | OpenCode equivalent is `UNKNOWN`; ignored. |
| `web_search_options` | object | Unsupported. Web access must be controlled by OpenCode agent permissions, not by OpenAI web-search request fields. |

### 8.3 Request schema: message fields

OpenAI message roles and nested fields are accounted for below.

| OpenAI field | Type | OpenCode equivalent | Phase 1 disposition |
|---|---|---|---|
| `messages[].role = developer` | literal | UNKNOWN. Could conceptually merge into OpenCode `system`, but this is not verified. | Unsupported unless deployment explicitly aliases developer to system. Default: reject if present. |
| `messages[].role = system` | literal | OpenCode `system?` | Supported for text-only content. Multiple system messages are concatenated by gateway policy only if explicitly enabled; otherwise reject multiple system messages. |
| `messages[].role = user` | literal | OpenCode `parts` text part for the latest user turn. Exact `parts` schema is `UNKNOWN`. | Supported only for the newest user message and text-only content. Historical user messages are not replayed. |
| `messages[].role = assistant` | literal | Existing OpenCode session state. | Historical assistant messages are ignored to avoid context replay. Latest assistant message as final input is invalid. |
| `messages[].role = tool` | literal | No verified OpenCode equivalent for OpenAI tool-call continuation. | Unsupported. |
| `messages[].role = function` | literal | No verified OpenCode equivalent. Deprecated OpenAI function role. | Unsupported. |
| `messages[].name` | string | UNKNOWN | Ignored. |
| `developer.content` | string or text parts | UNKNOWN / possible `system?` | Unsupported by default; see developer role rule. |
| `developer.content[].type` | `text` | UNKNOWN | Unsupported by default. |
| `developer.content[].text` | string | UNKNOWN | Unsupported by default. |
| `system.content` | string or text parts | OpenCode `system?` | Supported if text-only; flatten to a string. Exact OpenCode type is `UNKNOWN` until `/doc` verification. |
| `system.content[].type` | `text` | OpenCode `system?` | Supported only when `text`. |
| `system.content[].text` | string | OpenCode `system?` | Supported. |
| `user.content` | string or content parts | OpenCode `parts` | Supported for latest user message if text-only. Exact OpenCode `parts` element schema is `UNKNOWN`. |
| `user.content[].type = text` | literal | OpenCode text part | Supported for latest user message. |
| `user.content[].text` | string | OpenCode text part | Supported for latest user message. |
| `user.content[].type = image_url` | literal | UNKNOWN | Unsupported. |
| `user.content[].image_url.url` | string URI/base64 | UNKNOWN | Unsupported. |
| `user.content[].image_url.detail` | enum | UNKNOWN | Unsupported. |
| `user.content[].type = input_audio` | literal | UNKNOWN | Unsupported. |
| `user.content[].input_audio.data` | base64 string | UNKNOWN | Unsupported. |
| `user.content[].input_audio.format` | enum: `wav`, `mp3` | UNKNOWN | Unsupported. |
| `user.content[].type = file` | literal | UNKNOWN | Unsupported. |
| `user.content[].file.file_data` | string | UNKNOWN | Unsupported. |
| `user.content[].file.file_id` | string | UNKNOWN | Unsupported. |
| `user.content[].file.filename` | string | UNKNOWN | Unsupported. |
| `assistant.content` | string or text/refusal parts | Existing OpenCode session state | Ignored for historical messages; invalid if latest message. |
| `assistant.audio.id` | string | UNKNOWN | Unsupported. |
| `assistant.refusal` | string | UNKNOWN | Ignored for historical messages; unsupported as active input. |
| `assistant.function_call.name` | string | UNKNOWN | Unsupported. Deprecated. |
| `assistant.function_call.arguments` | string | UNKNOWN | Unsupported. Deprecated. |
| `assistant.tool_calls[].id` | string | UNKNOWN | Unsupported. |
| `assistant.tool_calls[].type = function` | literal | UNKNOWN | Unsupported. |
| `assistant.tool_calls[].function.name` | string | UNKNOWN | Unsupported. |
| `assistant.tool_calls[].function.arguments` | string | UNKNOWN | Unsupported. |
| `assistant.tool_calls[].type = custom` | literal | UNKNOWN | Unsupported. |
| `assistant.tool_calls[].custom.name` | string | UNKNOWN | Unsupported. |
| `assistant.tool_calls[].custom.input` | string | UNKNOWN | Unsupported. |
| `tool.content` | string or text parts | UNKNOWN | Unsupported. |
| `tool.tool_call_id` | string | UNKNOWN | Unsupported. |
| `function.content` | string | UNKNOWN | Unsupported. Deprecated. |
| `function.name` | string | UNKNOWN | Unsupported. Deprecated. |

### 8.4 Request schema: tool and structured-output fields

| OpenAI field | Nested fields | OpenCode equivalent | Phase 1 disposition |
|---|---|---|---|
| `tools[]` | `type = function`, `function.name`, `function.description`, `function.parameters`, `function.strict` | No OpenAI external-tool loop equivalent. OpenCode has internal tools via agent config, but request schema remains `UNKNOWN`. | Unsupported. |
| `tools[]` | `type = custom`, `custom.name`, `custom.description`, `custom.format.type`, `custom.format.grammar.definition`, `custom.format.grammar.syntax` | UNKNOWN | Unsupported. |
| `tool_choice` | `none`, `auto`, `required` | UNKNOWN | Unsupported. |
| `tool_choice` | named function/custom choice | UNKNOWN | Unsupported. |
| `tool_choice.allowed_tools.mode` | `auto` or `required` | UNKNOWN | Unsupported. |
| `tool_choice.allowed_tools.tools[]` | tool definitions | UNKNOWN | Unsupported. |
| `response_format.type` | `text` | No downstream constraint needed | Accepted and ignored. |
| `response_format.type` | `json_object` | UNKNOWN | Unsupported. |
| `response_format.type` | `json_schema` | UNKNOWN | Unsupported. |
| `response_format.json_schema.name` | string | UNKNOWN | Unsupported. |
| `response_format.json_schema.description` | string | UNKNOWN | Unsupported. |
| `response_format.json_schema.schema` | JSON Schema object | UNKNOWN | Unsupported. |
| `response_format.json_schema.strict` | boolean | UNKNOWN | Unsupported. |
| `stream_options.include_usage` | boolean | Future OpenAI SSE usage chunk | Unsupported in Phase 1 because streaming is unsupported. |
| `stream_options.include_obfuscation` | boolean | UNKNOWN | Unsupported. |
| `web_search_options.search_context_size` | `low`, `medium`, `high` | UNKNOWN | Unsupported. |
| `web_search_options.user_location.type` | `approximate` | UNKNOWN | Unsupported. |
| `web_search_options.user_location.approximate.city` | string | UNKNOWN | Unsupported. |
| `web_search_options.user_location.approximate.country` | string | UNKNOWN | Unsupported. |
| `web_search_options.user_location.approximate.region` | string | UNKNOWN | Unsupported. |
| `web_search_options.user_location.approximate.timezone` | string | UNKNOWN | Unsupported. |

### 8.5 OpenAI top-level field mapping to OpenCode

| OpenAI field | OpenCode equivalent | Gateway action | Status |
|---|---|---|---|
| `model` | OpenCode `agent?`; optional OpenCode `model?` for provider/model override | Resolve public gateway model ID to a configured OpenCode agent/profile. Exact `agent` and `model` types are `UNKNOWN`. | Supported with `UNKNOWN` downstream type. |
| `messages` | OpenCode `system?` and `parts` | Extract system text and latest user text. Do not replay history. Exact `parts` schema is `UNKNOWN`. | Supported with `UNKNOWN` downstream type. |
| `audio` | UNKNOWN | Reject if present. | Unsupported. |
| `frequency_penalty` | UNKNOWN | Validate range, then ignore. | Ignored. |
| `function_call` | UNKNOWN | Reject if present. | Unsupported. |
| `functions` | UNKNOWN | Reject if present. | Unsupported. |
| `logit_bias` | UNKNOWN | Reject if present. | Unsupported. |
| `logprobs` | UNKNOWN | Reject if true. | Unsupported. |
| `max_completion_tokens` | UNKNOWN | Validate type; ignore unless OpenCode `/doc` verifies equivalent. | Ignored / UNKNOWN. |
| `max_tokens` | UNKNOWN | Validate type; ignore unless OpenCode `/doc` verifies equivalent. | Ignored / UNKNOWN. |
| `metadata` | None | Keep only for gateway observability if needed; do not forward. | Ignored. |
| `modalities` | UNKNOWN | Require text-only. | Unsupported for audio. |
| `n` | UNKNOWN | Require absent or `1`; reject `>1`. | Unsupported beyond `1`. |
| `parallel_tool_calls` | UNKNOWN | Reject when true. | Unsupported. |
| `prediction` | UNKNOWN | Reject if present. | Unsupported. |
| `presence_penalty` | UNKNOWN | Validate range, then ignore. | Ignored. |
| `prompt_cache_key` | None | Ignore. | Ignored. |
| `prompt_cache_retention` | None | Ignore. | Ignored. |
| `reasoning_effort` | UNKNOWN | Ignore. | Ignored / UNKNOWN. |
| `response_format` | UNKNOWN except text default | Accept `text`; reject JSON modes. | Partially supported. |
| `safety_identifier` | None | Use only for gateway audit/rate-limit policy if configured. | Ignored by OpenCode. |
| `seed` | UNKNOWN | Validate integer-like, then ignore. | Ignored. |
| `service_tier` | None | Ignore. | Ignored. |
| `stop` | UNKNOWN | Validate shape, then ignore unless OpenCode `/doc` verifies equivalent. | Ignored / UNKNOWN. |
| `store` | None | Ignore. | Ignored. |
| `stream` | `POST /session/:id/prompt_async` plus `GET /event` in future | Reject `true` in Phase 1. | Unsupported in Phase 1. |
| `stream_options` | Future SSE usage mapping | Reject in Phase 1. | Unsupported in Phase 1. |
| `temperature` | UNKNOWN | Validate range, then ignore. | Ignored. |
| `tool_choice` | UNKNOWN | Reject if present. | Unsupported. |
| `tools` | OpenCode `tools?` is not equivalent to OpenAI tools and schema is `UNKNOWN` | Reject. OpenCode internal tools are governed by agent config. | Unsupported. |
| `top_logprobs` | UNKNOWN | Reject if present. | Unsupported. |
| `top_p` | UNKNOWN | Validate range, then ignore. | Ignored. |
| `user` | Gateway session key fallback only by explicit policy | Prefer forwarded `X-OpenWebUI-*` headers. Do not forward to OpenCode. | Ignored by OpenCode. |
| `verbosity` | UNKNOWN | Ignore. | Ignored / UNKNOWN. |
| `web_search_options` | UNKNOWN | Reject. | Unsupported. |

### 8.6 Session resolution rules

| Step | Rule |
|---:|---|
| 1 | Authenticate the request before reading or routing any body content. |
| 2 | Resolve `body.model` against gateway-owned model mapping. |
| 3 | Build ledger key from forwarded Open WebUI user/chat headers plus `body.model` when headers are available. |
| 4 | If forwarded headers are unavailable, Phase 1 may operate only in single-user mode or in an explicitly configured fallback mode using `body.user`. |
| 5 | If no ledger entry exists, create an OpenCode session with `POST /session`. |
| 6 | If a ledger entry exists, reuse the mapped OpenCode session. |
| 7 | Extract only the newest user message from `messages`. |
| 8 | Extract text-only system instruction. |
| 9 | Send the latest user text to `POST /session/:id/message`. |
| 10 | Update ledger timestamp after a routing attempt, with exact failure semantics left to implementation design. |

### 8.7 Response schema: non-streaming

OpenAI-compatible Chat Completion response:

| Field | Type | Required | Mapping |
|---|---|---:|---|
| `id` | string | Yes | Gateway-generated completion ID. No OpenCode equivalent required. |
| `object` | literal `chat.completion` | Yes | Gateway-owned OpenAI-compatible wrapper. |
| `created` | integer Unix timestamp | Yes | Gateway-generated response timestamp. |
| `model` | string | Yes | Echo public gateway model ID from request, not raw OpenCode agent unless intentionally identical. |
| `choices` | array | Yes | Exactly one choice in Phase 1. |
| `choices[].index` | number | Yes | Always `0` in Phase 1. |
| `choices[].message.role` | literal `assistant` | Yes | Gateway wrapper. |
| `choices[].message.content` | string | Yes | Concatenated OpenCode `TextPart.text` values. Exact part extraction must be verified against live `/doc`. |
| `choices[].message.refusal` | string or null | No | Not produced in Phase 1; null or omitted by compatibility policy. OpenCode equivalent is `UNKNOWN`. |
| `choices[].message.annotations` | array | No | Not produced. OpenCode equivalent is `UNKNOWN`. |
| `choices[].message.audio` | object or null | No | Not produced. Unsupported. |
| `choices[].message.function_call` | object | No | Not produced. Deprecated and unsupported. |
| `choices[].message.tool_calls` | array | No | Not produced. OpenAI tool-call loop unsupported. |
| `choices[].finish_reason` | enum | Yes | `stop` for successful completed text response unless verified OpenCode `finish` maps to a more specific OpenAI value. Exact OpenCode mapping is `UNKNOWN`. |
| `choices[].logprobs` | object or null | No | Always null or omitted; logprobs unsupported. |
| `usage` | object or null | No | Map from OpenCode `AssistantMessage.tokens` when present. Exact source location in response must be verified. |
| `service_tier` | enum | No | Not produced unless gateway chooses to echo accepted value; no OpenCode equivalent. |
| `system_fingerprint` | string | No | Not produced. No OpenCode equivalent. Deprecated in OpenAI docs. |

### 8.8 Usage mapping

| OpenAI usage field | OpenCode equivalent | Mapping status |
|---|---|---|
| `usage.prompt_tokens` | `AssistantMessage.tokens.input` | Supported if token metadata is present. |
| `usage.completion_tokens` | `AssistantMessage.tokens.output` | Supported if token metadata is present. |
| `usage.total_tokens` | `input + output + reasoning` or OpenAI-compatible total by gateway policy | Partially `UNKNOWN`; choose explicitly before implementation. |
| `usage.completion_tokens_details.reasoning_tokens` | `AssistantMessage.tokens.reasoning` | Supported if token metadata is present. |
| `usage.prompt_tokens_details.cached_tokens` | `AssistantMessage.tokens.cache.read` plus/or `cache.write` | `UNKNOWN`; OpenAI cached token semantics do not directly equal OpenCode cache read/write without verification. |

If OpenCode token metadata is absent or unverified, `usage` must be null or omitted rather than fabricated.

### 8.9 Response extraction rules

| OpenCode part type | Phase 1 response behavior |
|---|---|
| `TextPart` | Concatenate `text` fields in returned order. |
| `ReasoningPart` | Do not expose in Phase 1. Mapping to OpenAI visible content is a product decision, not a protocol fact. |
| `ToolPart` | Do not expose in Phase 1. Future streaming may render sanitized progress text. |
| `PatchPart` | Do not expose in Phase 1. Future UX may summarize patches, but not in this contract. |
| `FilePart` | Unsupported unless future verified mapping exists. |
| Unknown part type | Ignore or fail closed by deployment policy; recommended: fail closed during development. |

### 8.10 Validation rules

| Rule | Error |
|---|---|
| Missing `model` | 400 `missing_required_parameter`, `param: model`. |
| Unknown `model` | 404 `model_not_found`, `param: model`. |
| Missing `messages` | 400 `missing_required_parameter`, `param: messages`. |
| `messages` is empty | 400 `invalid_value`, `param: messages`. |
| No latest user message can be extracted | 400 `invalid_request_error`, `param: messages`. |
| Latest user message is not text-only | 400 `unsupported_parameter`, `param: messages`. |
| `stream: true` | 400 `unsupported_parameter`, `param: stream`. |
| `n` other than `1` | 400 `unsupported_parameter`, `param: n`. |
| `tools` or `tool_choice` present | 400 `unsupported_parameter`. |
| `response_format.type` is `json_object` or `json_schema` | 400 `unsupported_parameter`, `param: response_format`. |
| Numeric controls outside official ranges | 400 `invalid_value`. |
| OpenCode session cannot be created | 503 or 502 according to downstream failure category. |
| OpenCode response has no usable text part | 502 `opencode_bad_gateway` unless product policy allows empty assistant content. |

### 8.11 Authentication rules

| Rule | Behavior |
|---|---|
| Missing or invalid gateway Bearer token | Reject before session lookup. |
| Forwarded `X-OpenWebUI-User-Id` | Use as routing identity only when header forwarding is enabled and gateway trusts Open WebUI network boundary. |
| Forwarded `X-OpenWebUI-Chat-Id` | Use as chat/session identity only when available. |
| Forwarded `X-OpenWebUI-Message-Id` | May be used to derive or validate OpenCode `messageID`, but exact OpenCode `messageID` type remains `UNKNOWN`. |
| Gateway to OpenCode | Use Basic Auth when configured. |

### 8.12 Error responses

| Status | Trigger | Error |
|---:|---|---|
| 400 | Invalid JSON, missing fields, invalid type/range, unsupported OpenAI field. | Shared error envelope. |
| 401 | Missing/invalid Bearer token. | Shared error envelope. |
| 404 | Unknown gateway model ID. | Shared error envelope. |
| 502 | OpenCode response cannot be translated safely. | Shared error envelope. |
| 503 | OpenCode unavailable or auth to OpenCode failed. | Shared error envelope. |
| 504 | OpenCode execution timeout. | Shared error envelope. |

## 9. Future streaming compatibility

Phase 1 rejects `stream: true`.

Phase 2 must not simply chunk the final non-streaming response. Real streaming requires:

| Requirement | Status |
|---|---|
| Submit latest prompt through `POST /session/:id/prompt_async`. | Documented endpoint; exact request schema requires `/doc` verification. |
| Subscribe to OpenCode `GET /event` or `GET /global/event`. | Documented endpoint; event selection and correlation are `UNKNOWN`. |
| Correlate events to the active session/message. | `UNKNOWN`; blocker. |
| Emit OpenAI-compatible SSE chunks. | Required for Open WebUI-style streaming; exact chunk compatibility must be verified. |
| Terminate stream with `data: [DONE]`. | Required by OpenAI-style SSE compatibility. |
| Optional usage chunk when `stream_options.include_usage=true`. | Future only; requires reliable final usage accounting. |

## 10. Implementation blockers

These are not optional polish. They block implementation if not resolved:

1. Exact OpenCode `POST /session/:id/message` request body schema from live `GET /doc`.
2. Exact OpenCode `POST /session/:id/prompt_async` request body schema from live `GET /doc`.
3. Exact OpenCode `GET /agent` response schema and safe agent identifier field.
4. Exact OpenCode `Part` union from live schema and runtime samples.
5. Exact OpenCode error status/body behavior.
6. Exact OpenCode Basic Auth failure behavior.
7. Exact OpenCode `messageID` constraints.
8. Exact OpenCode `model` object schema for provider/model override.
9. Exact OpenWebUI request payload emitted by the target deployment.
10. Exact OpenWebUI behavior when `stream: true` is rejected.
11. Whether target Open WebUI forwards `X-OpenWebUI-*` headers with `ENABLE_FORWARD_USER_INFO_HEADERS=true`.
12. Exact policy for deriving single-user fallback session identity when headers are absent.

## 11. Acceptance criteria check

| Criterion | Status |
|---|---|
| `GET /health` documented | Satisfied. |
| `GET /v1/models` documented | Satisfied. |
| `POST /v1/chat/completions` documented | Satisfied. |
| Request schema per endpoint | Satisfied. |
| Response schema per endpoint | Satisfied. |
| Validation rules per endpoint | Satisfied. |
| Authentication rules per endpoint | Satisfied. |
| Error responses per endpoint | Satisfied. |
| Every OpenAI top-level Chat Completions request field accounted for | Satisfied in section 8.2 and 8.5. |
| Nested OpenAI message/tool/response-format fields accounted for | Satisfied in sections 8.3 and 8.4. |
| Every OpenAI field mapped to OpenCode equivalent, ignored, unsupported, or UNKNOWN | Satisfied. |
| Unknown mappings labeled `UNKNOWN` | Satisfied. |
| No implementation code | Satisfied. |
