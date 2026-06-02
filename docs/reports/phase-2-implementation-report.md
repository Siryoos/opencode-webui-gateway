# Phase 2 Implementation Report

Date: 2026-06-01

## Verdict

PASS: Phase 2 real OpenCode streaming is implemented and documented for the verified OpenCode runtime.

This verdict depends on live OpenCode streaming verification passing. If `scripts/phase2-discover-streaming.sh` fails in a target environment, that environment is BLOCKED for Phase 2 and must not use fake chunking as a substitute.

## Implemented Files

- `internal/opencode/client.go`: added OpenCode `prompt_async`, `/event` subscription, long-frame-safe SSE parsing, and final message list support.
- `internal/openai/stream_writer.go`: added OpenAI-compatible SSE chunk writer with role, delta, stop, usage, and DONE chunks.
- `internal/openai/usage.go`: added OpenCode token metadata to OpenAI usage mapping.
- `internal/openai/types.go`: added streaming chunk usage support.
- `internal/streaming/mapper.go`: added OpenCode event-to-stream state machine and in-flight session locks.
- `internal/httpapi/server.go`: wired `stream=true` to real OpenCode async/event streaming while preserving `stream=false` synchronous behavior.
- `scripts/phase2-discover-streaming.sh`: added direct live OpenCode streaming discovery and report generation.
- `scripts/phase2-smoke.sh`: added gateway-level Phase 2 smoke coverage.
- `docs/discovery/phase2-streaming-live-verification.md`: recorded live OpenCode streaming verification.
- `docs/reports/phase-2-implementation-report.md`: this final implementation report.
- `README.md`: updated operator-facing Phase 2 streaming documentation.

## Runtime Verification Summary

OpenCode runtime verified: `1.15.13`.

Live discovery verified:

- `GET /global/health`
- `GET /event`
- `POST /session`
- `POST /session/:id/prompt_async`
- `GET /session/status`
- `GET /session/:id/message`
- Target-session event correlation through `sessionID`
- Text-bearing `message.part.delta` events
- Completion through target-session `session.idle`
- Real token metadata availability from OpenCode event/message structures

The observed discovery verdict in `docs/discovery/phase2-streaming-live-verification.md` is PASS.

## Event Mapping Table

| OpenCode event | Gateway behavior |
|---|---|
| `server.connected` | Confirms `/event` subscription is established before `prompt_async` is sent. |
| Events without target `properties.sessionID` | Ignored. |
| Target-session events before prompt submission | Ignored for completion purposes. |
| `message.part.delta` with `field == "text"` | Emits OpenAI `chat.completion.chunk` with `delta.content`. |
| `session.diff` | Emits at most one safe progress content delta per stream. Raw diff details are not exposed. |
| `session.idle` after target-session activity | Completes the stream with stop chunk and `data: [DONE]`. |
| Malformed SSE JSON | Emits/returns controlled sanitized error behavior. |
| OpenCode stream read timeout/cancel/error | Releases lock and emits sanitized error behavior. |

## Completion Detection Strategy

The gateway subscribes to `GET /event` before calling `POST /session/:id/prompt_async`. It waits for `server.connected`, submits the async prompt, then accepts completion only after target-session activity has been observed after prompt submission.

Completion is based on target-session `session.idle`. This avoids treating unrelated idle events or pre-prompt idle events as completion. Events from other OpenCode sessions are ignored.

## Usage Mapping Strategy

The gateway does not fabricate usage.

For `stream=true`, usage is fetched only when `stream_options.include_usage == true`. In that case the gateway performs a final `GET /session/:id/message` and maps real OpenCode token metadata:

| OpenCode token field | OpenAI usage field |
|---|---|
| `tokens.input` | `prompt_tokens` |
| `tokens.output` | `completion_tokens` |
| `tokens.total` | `total_tokens` |
| `tokens.input + tokens.output + tokens.reasoning` | `total_tokens` fallback when `total` is absent |
| `tokens.reasoning` | `completion_tokens_details.reasoning_tokens` |

If real token metadata is absent, usage is omitted.

## Tests Added

- OpenCode client tests for `prompt_async`, `/event` subscription, auth behavior, SSE parsing, malformed JSON, long frames, and cancellation.
- OpenAI stream writer tests for headers, flush behavior, role chunks, content deltas, stop chunks, usage chunks, and DONE.
- Streaming mapper tests for session filtering, prompt-submission state, text delta mapping, progress dedupe, completion detection, and in-flight lock release.
- HTTP API tests for `stream=false` preservation, real `stream=true` flow, optional usage fetch, auth failures, downstream auth isolation, malformed event JSON, timeouts, cancellation lock release, `session_busy`, and sanitized logging.
- `scripts/phase2-smoke.sh` for end-to-end gateway smoke coverage against a running gateway/OpenCode pair.

## Commands Run

Final acceptance command results from this documentation pass:

```text
go test ./...                        PASS, 83 passed in 10 packages
go vet ./...                         PASS, no issues found
go build ./cmd/gateway               PASS
scripts/phase1-smoke.sh              PASS, all checks passed against local gateway
scripts/phase2-discover-streaming.sh PASS, discovery verdict PASS
scripts/phase2-smoke.sh              PASS, all checks passed against local gateway
```

The Phase 2 smoke script skipped `/session/:id/message` debug-counter verification because this production gateway build does not expose debug counters. It did verify that normal `stream=true` emits OpenAI-compatible SSE chunks with incremental content deltas and `[DONE]`.

## Known Limitations

- Streaming requires OpenCode endpoints and event shapes verified for OpenCode `1.15.13`.
- There is no fake chunking fallback for `stream=true`.
- OpenAI tools/function-calling are not implemented.
- MCP and ACP are not implemented.
- Dynamic routing, raw OpenCode agent exposure, and provider/model override are not implemented.
- Raw OpenCode events, raw shell output, patches, secrets, and full prompt bodies are not exposed to Open WebUI.
- One active stream is allowed per OpenCode session ID.
- Usage is only emitted when real token metadata is available and `stream_options.include_usage == true`.
- Dockerized OpenCode still requires a project-compatible runtime image for workspaces that need tools such as `python3`.

## Phase 3/Phase 4 Statement

No Phase 3 or Phase 4 features were implemented.

Specifically, this phase does not include multi-tenant OpenCode container spawning, per-user workspace isolation, dynamic model routing, cancellation endpoints, idle cleanup, metrics, or tracing.
