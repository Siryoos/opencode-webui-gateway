# Live OpenCode Contract Verification

Date: 2026-06-01

Base URL: `http://127.0.0.1:4096`

Authentication mode: unsecured local development. `OPENCODE_SERVER_PASSWORD` was not configured, so verification commands sent no Basic Auth header.

## Results

| Check | Result |
|---|---|
| `GET /global/health` | Passed. |
| `GET /doc` | Passed. Returned OpenAPI JSON. |
| `GET /agent` | Passed. Returned agent objects with `name`, `description`, `mode`, `native`, `permission`, and `options` fields. |
| Agent `plan` exists | Yes. |
| Agent `build` exists | Yes. |
| `POST /session` | Passed. |
| `POST /session/:id/message` | Passed with text-only `parts`. |

OpenCode version from `/global/health`: `1.15.13`.

## Verified Request Schemas Used

Session creation request body used:

```json
{"title":"gateway-live-contract-verification"}
```

Message request body used:

```json
{
  "agent": "plan",
  "parts": [
    { "type": "text", "text": "Reply with exactly: gateway verification ok" }
  ]
}
```

Verified message fields used for Phase 1 implementation:

| Field | Verified type | Use |
|---|---|---|
| `agent` | string | Static route target, `plan` or `build`. |
| `system` | string | Optional system instruction. |
| `parts` | array | Required message parts. |
| `parts[].type` | string enum, `text` for text input | Text-only prompt part. |
| `parts[].text` | string | Latest user text. |

Provider/model override is not implemented because Phase 1 does not require it and it is out of scope unless explicitly proven and approved.

## Observed Session Response Fields

Sanitized sample:

```json
{
  "id": "ses_17c5cecd3ffeh1c61vb8weIujv",
  "slug": "proud-knight",
  "projectID": "<project-id>",
  "directory": "<workspace-path>",
  "path": "",
  "cost": 0,
  "tokens": { "input": 0, "output": 0, "reasoning": 0, "cache": { "read": 0, "write": 0 } },
  "title": "gateway-live-contract-verification",
  "version": "1.15.13",
  "time": { "created": 1780324963116, "updated": 1780324963116 }
}
```

The gateway uses only `id` from the session response.

## Observed Agent Response Fields

Sanitized agent object fields observed:

```json
{
  "name": "build",
  "description": "The default agent. Executes tools based on configured permissions.",
  "mode": "primary",
  "native": true,
  "permission": [ { "permission": "*", "pattern": "*", "action": "allow" } ],
  "options": {}
}
```

The gateway uses only `name` for strict route-target availability validation when needed. It does not expose raw agents.

## Observed Message Response Shape

Sanitized response:

```json
{
  "info": {
    "parentID": "msg_<redacted>",
    "role": "assistant",
    "mode": "plan",
    "agent": "plan",
    "cost": 0,
    "tokens": { "total": 11055, "input": 11048, "output": 7, "reasoning": 0, "cache": { "write": 0, "read": 0 } },
    "modelID": "gpt-5.5",
    "providerID": "openai",
    "finish": "stop",
    "id": "msg_<redacted>",
    "sessionID": "ses_<redacted>"
  },
  "parts": [
    { "type": "step-start", "id": "prt_<redacted>", "sessionID": "ses_<redacted>", "messageID": "msg_<redacted>" },
    { "type": "text", "text": "gateway verification ok", "id": "prt_<redacted>", "sessionID": "ses_<redacted>", "messageID": "msg_<redacted>" },
    { "type": "step-finish", "tokens": { "total": 11055, "input": 11048, "output": 7, "reasoning": 0, "cache": { "write": 0, "read": 0 } }, "id": "prt_<redacted>" }
  ]
}
```

Exact text part shape observed:

```json
{
  "type": "text",
  "text": "gateway verification ok",
  "time": { "start": 1780324987718, "end": 1780324987787 },
  "metadata": { "openai": { "itemId": "<redacted>", "phase": "final_answer" } },
  "id": "prt_<redacted>",
  "sessionID": "ses_<redacted>",
  "messageID": "msg_<redacted>"
}
```

Non-text part shapes observed: `step-start` and `step-finish`. Phase 1 ignores all non-text parts.

Token metadata exists on `info.tokens` and on the observed `step-finish` part. The gateway does not currently emit OpenAI usage because the project requires not fabricating usage and a precise OpenAI usage policy remains out of Phase 1 implementation.

## Curl Commands Used

```bash
curl -sS -i --max-time 10 http://127.0.0.1:4096/global/health
curl -sS -i --max-time 10 http://127.0.0.1:4096/doc
curl -sS -i --max-time 10 http://127.0.0.1:4096/agent
curl -sS -i --max-time 20 -X POST http://127.0.0.1:4096/session -H 'Content-Type: application/json' -d '{"title":"gateway-live-contract-verification"}'
curl -sS -i --max-time 120 -X POST http://127.0.0.1:4096/session/ses_17c5cecd3ffeh1c61vb8weIujv/message -H 'Content-Type: application/json' -d '{"agent":"plan","parts":[{"type":"text","text":"Reply with exactly: gateway verification ok"}]}'
```
