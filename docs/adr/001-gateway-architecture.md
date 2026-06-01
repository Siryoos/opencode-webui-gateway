# ADR-001: OpenCode Gateway Architecture

- **Status:** Proposed
- **Date:** 2026-06-01
- **Scope:** Architecture decision record for the OpenCode Gateway.
- **Decision class:** Architecture, integration boundary, state lifecycle, and protocol translation.
- **Implementation code:** Out of scope.

## Source of truth

This ADR is derived only from the project roadmap and discovery artifacts:

| Input | Role in this ADR |
|---|---|
| `roadmap.txt` / roadmap input | Defines project objective, component topography, API translation contract, session ledger, delivery phases, authentication tiers, and agent capability mapping. |
| `docs/discovery/opencode-api-audit.md` | Defines the documented OpenCode server API surface, authentication behavior, session/message/event endpoints, and schema unknowns. |
| `docs/discovery/openwebui-contract.md` | Defines Open WebUI's documented OpenAI-compatible provider expectations, authentication behavior, forwarded user/session headers, required `/v1/models`, required `/v1/chat/completions`, and streaming constraints. |
| `docs/discovery/gateway-gap-analysis.md` | Defines the required gateway-owned contracts, interface gaps, security gaps, session-state gap, and implementation blockers. |

No undocumented endpoint, request field, response field, or behavior is treated as fact in this ADR.

## Context

Open WebUI expects an OpenAI-compatible Chat Completions provider. OpenCode exposes a proprietary stateful server API organized around projects, sessions, messages, agents, events, files, tools, and server operations. OpenCode does not document native `GET /v1/models` or `POST /v1/chat/completions` endpoints.

The gateway is therefore required as a protocol translation and state-management boundary between:

1. a stateless OpenAI-compatible client surface consumed by Open WebUI; and
2. a stateful OpenCode server surface based on sessions and message endpoints.

The central architectural problem is context duplication. Open WebUI Chat Completions mode sends the message plus conversation history on each request. OpenCode preserves state inside an OpenCode session. Replaying the entire Open WebUI history into an existing OpenCode session would duplicate context and corrupt the agent's working state. The gateway must map Open WebUI chat identity to persistent OpenCode session identity and forward only the newest user turn to OpenCode.

## External dependencies

| Dependency | Required by | Documented contract used | ADR boundary |
|---|---|---|---|
| Open WebUI | Client/provider integration | OpenAI-compatible provider connection, API key/Bearer authentication, `/v1/models`, `/v1/chat/completions`, optional forwarded `X-OpenWebUI-*` headers. | Upstream client. The gateway must satisfy Open WebUI's provider-facing expectations. |
| OpenCode server | Downstream execution | `opencode serve`, Basic Auth when `OPENCODE_SERVER_PASSWORD` is set, `/global/health`, `/agent`, `/session`, `/session/:id/message`, `/session/:id/prompt_async`, `/event`, `/global/event`, `/session/:id/abort`. | Downstream execution engine. The gateway must not expose OpenCode credentials or OpenCode's raw server API directly to Open WebUI. |
| Hermes/Open WebUI reference docs | Integration precedent | Reference pattern for Open WebUI connecting to an agent through `/v1/models` and `/v1/chat/completions`. | Informative precedent only. It does not define OpenCode behavior. |
| OpenClaw/Open WebUI reference docs | Integration precedent | Reference pattern for OpenAI-compatible agent gateway, model IDs, Bearer/shared-secret auth, SSE format, and `[DONE]` termination. | Informative precedent only. It does not define OpenCode behavior. |
| Gateway-owned session ledger | Gateway architecture | Roadmap-defined mapping from Open WebUI user/chat/model identity to OpenCode session ID. | Internal gateway state. Storage engine is not selected by this ADR. |

## Non-goals

- Do not implement code.
- Do not define undocumented OpenCode request/response schemas.
- Do not invent OpenAI response fields beyond the verified gateway contract.
- Do not use MCP or ACP as the integration path.
- Do not expose raw OpenCode APIs directly as the Open WebUI provider contract.
- Do not claim multi-user isolation unless forwarded Open WebUI identity headers and downstream OpenCode isolation are explicitly configured and verified.

---

# Decisions

## Decision 1: Use a three-component topology

### Decision

The gateway architecture uses three logical components:

```text
Open WebUI  --->  Gateway  --->  OpenCode server
 client           adapter        execution engine
```

| Component | Responsibility | Protocol surface |
|---|---|---|
| Open WebUI | User interface, chat client, provider configuration, optional user/session header forwarding. | Calls the gateway through an OpenAI-compatible provider connection. |
| Gateway | Authentication boundary, OpenAI-compatible API surface, request translation, response translation, model mapping, session ledger ownership, error boundary. | Exposes `GET /health`, `GET /v1/models`, and `POST /v1/chat/completions`. Calls OpenCode's documented server endpoints. |
| OpenCode server | Stateful agent execution against a workspace/project. | Exposes documented OpenCode server endpoints such as `/global/health`, `/agent`, `/session`, `/session/:id/message`, `/session/:id/prompt_async`, `/event`, and `/session/:id/abort`. |

### Rationale

The roadmap defines the local system as three nodes: Client Node, Gateway Node, and Execution Node. Discovery confirms this separation is necessary because Open WebUI expects OpenAI-compatible Chat Completions, while OpenCode exposes a different stateful API. The gateway is the only component that can reconcile these two incompatible contracts without modifying Open WebUI or OpenCode.

### Alternatives rejected

| Alternative | Rejection reason |
|---|---|
| Connect Open WebUI directly to OpenCode | Rejected because OpenCode does not document native `/v1/models` or `/v1/chat/completions` endpoints. |
| Use MCP or ACP as the gateway integration path | Rejected because the roadmap explicitly makes OpenAI-compatible Chat Completions the primary integration path and treats MCP/ACP as secondary or client-side. |
| Implement the integration as an Open WebUI pipe first | Rejected for this ADR because the roadmap target is an external OpenAI-compatible gateway. A pipe can be a prototype path, but it is not the gateway architecture being recorded here. |
| Collapse Gateway and OpenCode into one component | Rejected because it would blur authentication boundaries, state ownership, model mapping, and protocol translation responsibilities. |

### Risks

| Risk | Impact | Mitigation / constraint |
|---|---|---|
| Gateway becomes a catch-all control plane | High complexity and security risk. | Keep the gateway limited to documented provider translation, session routing, model mapping, auth boundaries, and future streaming adaptation. |
| OpenCode server behavior changes outside the documented API | Gateway contract can break. | Treat OpenCode `/doc` verification as a blocker before implementation. |
| Local topology may not equal production topology | Misleading deployment assumptions. | ADR defines logical components only; physical deployment is intentionally not selected. |

### Traceability

- Roadmap: Architectural Overview, Core Invariants, Container Topography.
- Discovery: Gateway gap analysis executive conclusion and interface gap matrix.

---

## Decision 2: Expose a minimal OpenAI-compatible gateway surface

### Decision

The gateway exposes only the minimum documented provider-facing API surface required by the roadmap and discovery:

| Gateway endpoint | Purpose | Downstream dependency |
|---|---|---|
| `GET /health` | Gateway health and downstream OpenCode reachability check. | OpenCode `GET /global/health`. |
| `GET /v1/models` | Present configured OpenCode-backed gateway model targets to Open WebUI. | OpenCode `GET /agent`, plus gateway-owned model mapping. |
| `POST /v1/chat/completions` | Receive Open WebUI Chat Completions requests and return agent responses. | OpenCode `POST /session`, `POST /session/:id/message`, and later `POST /session/:id/prompt_async` + event stream for streaming. |

No other gateway endpoint is accepted by this ADR.

### Rationale

The roadmap requires `GET /health`, `GET /v1/models`, and `POST /v1/chat/completions`. Discovery confirms Open WebUI uses `/v1/models` for provider verification/model discovery and `/v1/chat/completions` as the primary chat endpoint. Discovery also confirms OpenCode provides the downstream primitives needed to support these endpoints.

### Alternatives rejected

| Alternative | Rejection reason |
|---|---|
| Expose the entire OpenCode API through the gateway | Rejected because it expands the attack surface and violates the gateway's role as a narrow compatibility adapter. |
| Skip `GET /v1/models` and rely on manual model allowlisting | Rejected for the architecture because Open WebUI verifies `/models`; manual allowlisting is a fallback, not the intended provider contract. |
| Implement streaming-only first | Rejected because the roadmap Phase 1 is explicitly non-streaming and synchronous. |
| Implement OpenAI Assistants API, Responses API, or custom agent APIs | Rejected because they are not part of the roadmap or discovery contract. |

### Risks

| Risk | Impact | Mitigation / constraint |
|---|---|---|
| Open WebUI may require response fields not fully specified in discovery | Provider verification or chat rendering may fail. | Do not finalize field-level response schemas until Open WebUI behavior is verified. |
| OpenCode `/agent` response fields are `TYPE REFERENCE ONLY` / UNKNOWN | Model discovery cannot safely expose raw agent fields. | Keep model mapping gateway-owned and verify `/agent` via live `/doc` before implementation. |
| Gateway health may be mistaken for OpenCode readiness | False positive readiness. | `GET /health` must distinguish gateway liveness from downstream OpenCode health in the eventual design; exact response schema is not defined here. |

### Traceability

- Roadmap: API Translation Contract, Required Gateway Endpoints, Phase 1.
- Discovery: Open WebUI contract `/v1/models` and `/v1/chat/completions`; OpenCode endpoint mapping for session creation, message send, agent listing, and health.

---

## Decision 3: Treat the gateway as the state translation boundary

### Decision

The gateway owns a session ledger that maps Open WebUI chat identity to OpenCode session identity.

The logical ledger key is:

```text
(user identity, chat/session identity, gateway model id) -> OpenCode session id
```

The ledger must include, at minimum, the following logical data:

| Ledger field | Source |
|---|---|
| User identity | Forwarded Open WebUI user header when available; otherwise the deployment is single-user and must not claim multi-user isolation. |
| Chat/session identity | Forwarded Open WebUI chat/session header when available. |
| Gateway model id | `body.model` from the OpenAI-compatible chat request. |
| OpenCode session id | Returned by OpenCode session creation. |
| Created/updated timestamps | Gateway-owned lifecycle metadata for cleanup and routing. |

This ADR does not select the ledger storage engine. The roadmap mentions SQLite for MVP and Redis for distributed production as examples, but the storage decision is intentionally deferred.

### Rationale

Open WebUI sends full conversation history in Chat Completions mode. OpenCode sessions preserve state. The gateway must avoid replaying the full Open WebUI history into an existing OpenCode session. A ledger is therefore mandatory: it lets the gateway send only the latest user turn to the correct persistent OpenCode session.

### Alternatives rejected

| Alternative | Rejection reason |
|---|---|
| Create a new OpenCode session for every request | Rejected because it discards OpenCode's stateful session semantics and breaks continuity. |
| Replay full Open WebUI history into OpenCode on every request | Rejected because it duplicates context inside a stateful OpenCode session. |
| Use only `body.user` as the session key | Rejected because discovery only documents forwarded Open WebUI headers and notes exact body fields are not fully known from Open WebUI docs. |
| Use only OpenCode session list/search for routing | Rejected because no documented OpenCode endpoint provides a gateway-specific mapping from Open WebUI chat identity to OpenCode session identity. |
| Store ledger key only by chat id | Rejected because different gateway model ids may map to different OpenCode agents/capability profiles. |

### Risks

| Risk | Impact | Mitigation / constraint |
|---|---|---|
| Forwarded Open WebUI headers may not be enabled | Multi-user routing identity is unavailable. | Treat deployment as single-user unless `ENABLE_FORWARD_USER_INFO_HEADERS=true` is configured and verified. |
| Ledger corruption or stale entries | Requests may route to the wrong OpenCode session. | Future implementation must include explicit lifecycle validation and cleanup; this ADR only defines the required lifecycle boundary. |
| User isolation is overclaimed | Security incident: one user's chat routes into another user's OpenCode session/workspace. | Multi-tenant claims require Phase 3 routing and workspace isolation; Phase 1 is single-user only. |
| Storage engine chosen too early | Unnecessary complexity or poor production fit. | Defer SQLite/Redis decision to implementation ADR or design spec. |

### Traceability

- Roadmap: Core Invariants, Session & State Management, Session Ledger, Routing Logic Flow, Phase 1, Phase 3.
- Discovery: Session-state gap, gateway ledger requirement, user/session forwarding headers.

---

## Decision 4: Define the non-streaming request lifecycle first

### Decision

Phase 1 request lifecycle is synchronous and non-streaming.

For `POST /v1/chat/completions`, the gateway lifecycle is:

1. Authenticate the Open WebUI request at the gateway boundary.
2. Read the requested gateway model id from `body.model`.
3. Read the Open WebUI chat/session identity from forwarded headers when available.
4. Resolve or create a ledger entry for `(user identity, chat/session identity, gateway model id)`.
5. If no OpenCode session exists for that key, create one through OpenCode `POST /session`.
6. Extract the system instruction and latest user message from `body.messages` according to the roadmap.
7. Do not replay historical conversation messages into an existing OpenCode session.
8. Translate the latest user message into the documented OpenCode message endpoint shape only after live schema verification. The public discovery currently marks key request fields such as `model`, `agent`, `parts`, `tools`, and `noReply` as UNKNOWN or requiring `/doc` verification.
9. Send the request to OpenCode `POST /session/:id/message` for synchronous response.
10. Flatten eligible returned text parts into one assistant response body.
11. Package the result in the verified OpenAI-compatible Chat Completions response shape.

This ADR does not define exact JSON request/response fields beyond what the roadmap and discovery already document.

### Rationale

The roadmap defines Phase 1 as non-streaming and synchronous. The discovery confirms OpenCode has a documented synchronous `POST /session/:id/message` endpoint. This lifecycle gives Open WebUI the expected provider shape while preserving OpenCode state semantics.

### Alternatives rejected

| Alternative | Rejection reason |
|---|---|
| Use OpenCode `POST /session/:id/prompt_async` in Phase 1 | Rejected because Phase 1 is non-streaming; async prompt belongs to the streaming phase. |
| Use OpenCode event streams for Phase 1 | Rejected because event correlation is explicitly UNKNOWN and Phase 1 is synchronous. |
| Translate every Open WebUI message into OpenCode parts | Rejected because it conflicts with the state-management invariant and duplicates history. |
| Invent a complete OpenCode message request schema now | Rejected because discovery marks exact request body schemas as requiring live `/doc` verification. |
| Return internal OpenCode tool/patch structures directly | Rejected because Open WebUI expects a Chat Completions-compatible assistant response, not OpenCode's native part model. |

### Risks

| Risk | Impact | Mitigation / constraint |
|---|---|---|
| Latest-user-message extraction is ambiguous for tool or non-standard roles | Incorrect prompt forwarding. | Exact role-handling policy must be verified against Open WebUI emitted payloads before implementation. |
| OpenCode response contains reasoning/tool/patch parts, not only text | Missing useful status details or leaking internal execution details. | Phase 1 flattens text only; formatting tool progress is deferred to streaming/UX design. |
| OpenAI-compatible response wrapper fields are not fully known from Open WebUI docs | Open WebUI rendering or provider checks may fail. | Verify the concrete target Open WebUI deployment before freezing wire schema. |

### Traceability

- Roadmap: Request Payload Mapping, Response Payload Mapping, Phase 1.
- Discovery: `POST /v1/chat/completions` requirement, `POST /session/:id/message`, request translation gaps, response translation gaps.

---

## Decision 5: Use two authentication boundaries

### Decision

The architecture uses two separate authentication boundaries:

| Boundary | Required behavior |
|---|---|
| Open WebUI → Gateway | Gateway validates the API key/Bearer token configured in Open WebUI's provider connection. |
| Gateway → OpenCode | Gateway authenticates to OpenCode using OpenCode server Basic Auth when `OPENCODE_SERVER_PASSWORD` protects the OpenCode server. |

OpenCode Basic Auth credentials are internal credentials. They must not be exposed to Open WebUI, browser clients, chat content, logs intended for users, or model-visible context.

User/session identity is not authentication by itself. Forwarded `X-OpenWebUI-*` headers are identity/context inputs for routing, auditing, authorization policy, and future multi-tenant partitioning only when forwarding is enabled and verified.

### Rationale

The roadmap defines a public/client tier secured by a static Bearer token and an internal tier secured by OpenCode Basic Auth. Discovery confirms Open WebUI provider verification uses Bearer authentication and OpenCode uses Basic Auth when configured with `OPENCODE_SERVER_PASSWORD`.

### Alternatives rejected

| Alternative | Rejection reason |
|---|---|
| Let Open WebUI call OpenCode with Basic Auth directly | Rejected because it leaks downstream execution credentials and bypasses gateway policy/state controls. |
| Use forwarded user headers as authentication | Rejected because discovery documents them as forwarded metadata, not as proof of authentication. |
| Disable gateway auth for local development as the architecture default | Rejected because the roadmap requires stateless API key authorization between Open WebUI and Gateway. |
| Share the same secret for Open WebUI→Gateway and Gateway→OpenCode | Rejected because it collapses trust boundaries and increases blast radius. |

### Risks

| Risk | Impact | Mitigation / constraint |
|---|---|---|
| Bearer token verification behavior beyond `/models` is not fully documented | Inconsistent auth handling across endpoints. | Gateway auth policy must be consistent across its provider-facing endpoints; exact failure response fields are deferred. |
| Forwarded headers can be spoofed if gateway is publicly exposed | Session routing compromise. | Treat forwarded headers as trusted only from Open WebUI over a controlled network boundary. |
| OpenCode Basic Auth challenge/failure schema is unknown | Downstream auth errors may be hard to normalize. | Verify live OpenCode auth behavior before implementation. |

### Traceability

- Roadmap: Security and Permissions Specification, Authentication Tiers, Phase 1, Phase 3.
- Discovery: Open WebUI authentication requirements, OpenCode server authentication, security gaps.

---

## Decision 6: Make the gateway the error propagation boundary

### Decision

The gateway owns error propagation between Open WebUI and OpenCode.

The strategy is:

1. Do not return raw OpenCode native errors directly to Open WebUI as the provider contract.
2. Do not fabricate successful Chat Completions responses when OpenCode fails.
3. Preserve the meaningful error category and sanitized downstream context where available.
4. Keep exact JSON error field names undecided until Open WebUI's required error response behavior and OpenCode's live error schema are verified.
5. Treat UNKNOWN schemas as blockers, not as permission to invent fields.

Architectural error categories:

| Category | Boundary | Examples from documented gaps |
|---|---|---|
| Gateway authentication failure | Open WebUI → Gateway | Invalid/missing Bearer token. |
| Gateway request validation failure | Open WebUI → Gateway | Unsupported gateway model id, missing routable chat/message input, or unsupported streaming mode during Phase 1. |
| Downstream authentication failure | Gateway → OpenCode | Basic Auth rejected by OpenCode. Exact failure schema is UNKNOWN. |
| Downstream availability failure | Gateway → OpenCode | OpenCode health unavailable or request to required endpoint fails. |
| Downstream execution failure | Gateway → OpenCode | OpenCode message/session operation fails. Exact OpenCode error schemas/status codes are UNKNOWN. |
| Contract verification failure | Gateway-owned | Required OpenCode or Open WebUI schema is not verified; implementation must not proceed as if known. |

### Rationale

Open WebUI needs an OpenAI-compatible provider surface. OpenCode has a different API and unknown exact error schemas in the discovery. Raw pass-through would leak internal implementation details and likely violate the upstream provider contract. Fabricating success would be worse: it would hide execution failures and corrupt user trust.

### Alternatives rejected

| Alternative | Rejection reason |
|---|---|
| Pass raw OpenCode errors through unchanged | Rejected because OpenCode's native error shape is not documented and is not the Open WebUI provider contract. |
| Always return HTTP 200 with an assistant message containing the error | Rejected because it fabricates successful model behavior and makes failures indistinguishable from valid assistant output. |
| Define a full OpenAI error JSON schema in this ADR | Rejected because Open WebUI docs in the discovery do not fully specify required error fields and OpenCode exact error schemas are unknown. |
| Retry automatically on all downstream failures | Rejected because retry semantics, idempotency, and OpenCode session side effects are not documented. |

### Risks

| Risk | Impact | Mitigation / constraint |
|---|---|---|
| Open WebUI requires specific error body fields | Gateway errors may display poorly. | Verify target Open WebUI behavior before implementation. |
| OpenCode errors include sensitive workspace paths or command output | Information leakage. | Error propagation must sanitize downstream details. Exact sanitization policy is deferred. |
| Cancellation/error boundary is underdefined | User cancel may not stop OpenCode work. | Use documented `POST /session/:id/abort` only after exact cancel behavior is verified. |

### Traceability

- Roadmap: Authentication tiers, Phase 4 cancellation, production hardening.
- Discovery: Security gaps, unknown error schemas/status codes, cancellation unknowns, OpenCode `POST /session/:id/abort`.

---

## Decision 7: Use gateway-owned model mapping instead of exposing raw OpenCode agents

### Decision

The gateway exposes stable public model ids to Open WebUI and maps each public model id to an OpenCode agent/capability profile.

Phase 1 mapping is gateway-owned and curated:

| Public model profile | Intended OpenCode capability profile | Roadmap constraint |
|---|---|---|
| Analysis profile | Read-oriented OpenCode agent configuration. | Read-only file access; shell and external web fetches denied in configuration. |
| Execution profile | Write-capable OpenCode agent configuration. | Destructive shell/system/Docker operations configured as ask or deny in OpenCode backend configuration. |

This ADR does not decide exact public model ids, exact OpenCode agent names, or exact agent configuration files. Discovery explicitly marks the exact `GET /agent` response fields and direct safety of using raw agent names as UNKNOWN until `/doc` verification.

### Rationale

The roadmap requires hardcoded Phase 1 model mapping with one analysis model and one execution model. It also requires capability separation because OpenCode agents can have different permissions. Discovery confirms `/v1/models` must present model ids to Open WebUI and OpenCode `GET /agent` lists agents, but the exact `Agent` schema is unknown.

A gateway-owned mapping decouples Open WebUI-facing model ids from OpenCode internal agent ids and gives the gateway a stable safety boundary.

### Alternatives rejected

| Alternative | Rejection reason |
|---|---|
| Expose every OpenCode agent returned by `GET /agent` | Rejected because the `Agent` schema is unknown and raw exposure may leak unsafe write/shell-capable agents. |
| Use raw OpenCode agent names as public model ids without namespacing or verification | Rejected because discovery explicitly marks this as unknown. |
| Use a single generic model id for all capabilities | Rejected because the roadmap requires distinct analysis and execution profiles. |
| Let users select arbitrary provider/model overrides without gateway policy | Rejected because the roadmap requires controlled mapping and safety boundaries. |

### Risks

| Risk | Impact | Mitigation / constraint |
|---|---|---|
| Mapping drifts from actual OpenCode configuration | Open WebUI shows models that do not work or have wrong permissions. | Validate mapping against `GET /agent` and OpenCode configuration before implementation. |
| Execution model is too permissive | Workspace or host damage. | Keep destructive operations configured as ask/deny in OpenCode; do not rely on model name alone for safety. |
| Public model ids become part of user workflows | Renaming breaks chats and ledger keys. | Treat public model ids as stable once published. |

### Traceability

- Roadmap: Phase 1 hardcoded model mapping router, Security and Permissions Specification, Agent Capability Mapping.
- Discovery: `GET /v1/models`, `GET /agent`, model routing gap, operator surface risk.

---

## Decision 8: Preserve future streaming compatibility without implementing streaming in Phase 1

### Decision

The architecture reserves a future streaming path but Phase 1 remains non-streaming.

Future streaming compatibility is defined as:

1. Open WebUI requests streaming with `stream: true`.
2. Gateway submits the latest user prompt to OpenCode through the documented async endpoint `POST /session/:id/prompt_async` after schema verification.
3. Gateway consumes OpenCode server events through `GET /event` or, only if justified by verification, `GET /global/event`.
4. Gateway correlates OpenCode events to the specific OpenCode session/message after the event correlation contract is verified.
5. Gateway emits OpenAI-compatible SSE to Open WebUI using documented SSE transport behavior: `Content-Type: text/event-stream`, `data: <json>` lines, and `data: [DONE]` termination.
6. Internal OpenCode tool/progress events may be rendered as text-based markdown progress only after a verified mapping exists.

This ADR does not define event names, delta chunk schemas, or correlation logic because discovery marks those details as UNKNOWN.

### Rationale

The roadmap Phase 2 requires native-feeling streaming by mapping OpenCode SSE to OpenAI chunked streaming. Discovery confirms OpenCode documents event streams and async prompt submission, while Open WebUI/OpenClaw reference behavior documents SSE transport and `[DONE]` termination. Discovery also warns that exact OpenCode event sequence and correlation rules require `/doc` and runtime verification.

### Alternatives rejected

| Alternative | Rejection reason |
|---|---|
| Implement streaming in Phase 1 | Rejected because Phase 1 is explicitly non-streaming and exact event correlation is unknown. |
| Fake streaming by chunking the final non-streaming response | Rejected because it gives a misleading UX and does not represent OpenCode progress/events. |
| Use `/global/event` by default | Rejected because discovery marks when to prefer `/global/event` over `/event` as UNKNOWN. |
| Expose OpenCode SSE directly to Open WebUI | Rejected because Open WebUI expects OpenAI-compatible SSE chunks, not OpenCode-native events. |

### Risks

| Risk | Impact | Mitigation / constraint |
|---|---|---|
| OpenCode event stream cannot be reliably correlated to a submitted prompt | Streaming cannot be safely implemented. | Block streaming until `/doc` and runtime event correlation are verified. |
| Tool/progress event formatting leaks sensitive execution details | Security and UX issue. | Do not render internal events until a sanitization and formatting policy is defined. |
| OpenAI-compatible chunk schema expected by Open WebUI differs from reference behavior | Streaming may fail despite using SSE transport. | Verify against target Open WebUI deployment before implementation. |

### Traceability

- Roadmap: Phase 2 Streaming and UX Polish.
- Discovery: OpenCode `/prompt_async`, `/event`, `/global/event`, Open WebUI/OpenClaw streaming requirements, streaming unknowns.

---

# Request lifecycle summary

## `GET /health`

1. Gateway receives health request.
2. Gateway checks its own ability to serve requests.
3. Gateway verifies downstream OpenCode through `GET /global/health`.
4. Gateway reports health without exposing downstream credentials.

Exact response schema is not defined in this ADR.

## `GET /v1/models`

1. Gateway authenticates the Open WebUI Bearer token.
2. Gateway reads configured gateway-owned model mappings.
3. Gateway may verify available OpenCode agents through `GET /agent` after schema verification.
4. Gateway returns an OpenAI-compatible model list shape accepted by Open WebUI.

Minimum documented model-list fields from references are `object: "list"` and `data[].id`; other fields remain UNKNOWN until verified.

## `POST /v1/chat/completions` non-streaming

1. Gateway authenticates the Open WebUI Bearer token.
2. Gateway resolves model mapping from `body.model`.
3. Gateway resolves session key from forwarded Open WebUI metadata when available plus model id.
4. Gateway retrieves or creates the matching OpenCode session through the ledger and `POST /session`.
5. Gateway extracts the system prompt and latest user message.
6. Gateway sends only the latest user message to `POST /session/:id/message`.
7. Gateway flattens eligible text parts from OpenCode response.
8. Gateway returns the verified Chat Completions-compatible response.

## `POST /v1/chat/completions` streaming future path

1. Gateway authenticates the request.
2. Gateway resolves the same ledger/session mapping.
3. Gateway submits prompt asynchronously through `POST /session/:id/prompt_async`.
4. Gateway consumes OpenCode event stream.
5. Gateway correlates events to the active session/message.
6. Gateway emits OpenAI-compatible SSE chunks and terminates with `[DONE]`.

This path is reserved for Phase 2 and blocked by event schema/correlation verification.

# Session lifecycle summary

| Lifecycle stage | Decision |
|---|---|
| Creation trigger | First request for `(user identity, chat/session identity, gateway model id)` with no existing ledger entry. |
| Downstream creation | Use OpenCode `POST /session` after request routing determines a session is needed. |
| Reuse trigger | Later requests with the same composite key reuse the same OpenCode session id. |
| Message forwarding | Forward only the newest user message into the existing OpenCode session. |
| Update | Refresh gateway-owned updated timestamp after successful routing attempt. |
| Cancellation | Future control-plane behavior maps cancellation to OpenCode `POST /session/:id/abort` only after exact cancellation semantics are verified. |
| Garbage collection | Future Phase 4 behavior; roadmap gives idle-session cleanup as a production hardening goal, but this ADR does not define timing or deletion mechanics. |
| Multi-tenancy | Phase 3 only; requires forwarded Open WebUI user metadata, strict ledger partitioning by user id, and separate OpenCode routing/workspaces. |

# Authentication boundary summary

| Flow | Trust level | Decision |
|---|---|---|
| Browser/user → Open WebUI | Outside this ADR | Open WebUI owns end-user authentication. |
| Open WebUI → Gateway | Provider boundary | Bearer/API key validation required. |
| Gateway → OpenCode | Internal execution boundary | Basic Auth required when OpenCode password is configured. |
| Forwarded user headers | Routing/audit metadata | Used only when enabled and verified; not standalone authentication. |
| OpenCode credentials | Secret internal dependency | Must never be exposed to Open WebUI or model-visible prompts. |

# Error propagation summary

| Error source | Propagation decision |
|---|---|
| Gateway auth error | Reject at gateway before downstream OpenCode call. |
| Invalid/unsupported model mapping | Reject at gateway; do not call arbitrary OpenCode agent. |
| Missing routable identity in multi-tenant mode | Reject or downgrade only by explicit deployment policy; do not silently cross-route users. |
| OpenCode auth/connectivity error | Surface as gateway provider error with sanitized downstream context. |
| OpenCode execution error | Surface as failure, not as fake assistant success. |
| Unknown schema/contract | Treat as blocker for implementation, not as an inferred behavior. |

Exact error response JSON is intentionally not defined.

# Consequences

## Positive

- Open WebUI can integrate with OpenCode through a familiar OpenAI-compatible provider surface.
- OpenCode state is preserved without replaying full chat history.
- Authentication boundaries remain clean.
- Model exposure can be curated by capability and risk profile.
- Phase 1 can be non-streaming while leaving a credible path to Phase 2 streaming.

## Negative

- Gateway must own durable state and becomes a critical routing component.
- Exact wire schemas are still blocked by discovery unknowns.
- Multi-user isolation cannot be honestly claimed in Phase 1.
- Streaming cannot be implemented safely until OpenCode event correlation is verified.

## Follow-up decisions required before implementation

1. Exact OpenCode `POST /session/:id/message` request body schema from live `/doc`.
2. Exact OpenCode `POST /session/:id/prompt_async` request body schema from live `/doc`.
3. Exact `GET /agent` response fields and safe mapping from OpenCode agents to public model ids.
4. Exact OpenCode error schemas and auth failure behavior.
5. Exact Open WebUI request body emitted by the target deployment.
6. Exact Open WebUI-compatible non-streaming response fields required for display and provider behavior.
7. Exact OpenCode SSE event sequence and event-to-message correlation semantics.
8. Exact cancellation behavior for Open WebUI external providers and OpenCode `POST /session/:id/abort`.
9. Ledger storage engine and lifecycle policy.
10. Multi-tenant OpenCode routing/workspace isolation strategy.

# Acceptance criteria check

| Criterion | Status |
|---|---|
| Component topology defined | Satisfied: Open WebUI, Gateway, OpenCode. |
| Request lifecycle defined | Satisfied for health, models, non-streaming chat, and future streaming path. |
| Session lifecycle defined | Satisfied with ledger-based routing and explicit Phase 1/Phase 3 boundaries. |
| Authentication boundaries defined | Satisfied with Open WebUI→Gateway Bearer and Gateway→OpenCode Basic Auth. |
| Error propagation strategy defined | Satisfied at architectural level; exact JSON intentionally deferred because schemas are unknown. |
| Model mapping strategy defined | Satisfied with gateway-owned curated mapping and rejected raw OpenCode agent exposure. |
| Future streaming compatibility defined | Satisfied as Phase 2-compatible path, blocked on verified event correlation. |
| Every decision traceable to roadmap | Satisfied via traceability notes per decision. |
| Every external dependency documented | Satisfied in External dependencies section. |
| No implementation code | Satisfied. |
| No invented APIs | Satisfied; only roadmap gateway endpoints and documented OpenCode/OpenWebUI endpoints are referenced. |
| No invented future features | Satisfied; future work is limited to roadmap phases and discovery-identified endpoints/unknowns. |
