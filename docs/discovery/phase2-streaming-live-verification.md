# Phase 2 Streaming Live Verification

Date: 2026-06-02

Base URL: `http://127.0.0.1:4096`

Authentication mode: unsecured local development; no Basic Auth header sent.

OpenCode version: `1.15.13`

## Verdict

PASS: enough runtime evidence exists to implement real streaming

Blocker: None.

## Exact Commands Run

```bash
chmod +x scripts/phase2-discover-streaming.sh
scripts/phase2-discover-streaming.sh
rtk go test ./...
rtk go vet ./...
rtk go build ./cmd/gateway
```

The script performs these sanitized OpenCode calls without printing passwords or Basic Auth headers:

```bash
GET http://127.0.0.1:4096/global/health
GET http://127.0.0.1:4096/event
POST http://127.0.0.1:4096/session
POST http://127.0.0.1:4096/session/ses_<redacted>/prompt_async
GET http://127.0.0.1:4096/session/status
GET http://127.0.0.1:4096/session/ses_<redacted>/message
```

## Sanitized Observed SSE Events

| Event type | Session ID | Message ID | Part type | Token metadata shape | Text preview |
|---|---|---|---|---|---|
| session.created | ses_<redacted> |  |  | {cache{read,write}, input, output, reasoning} |  |
| session.updated | ses_<redacted> |  |  | {cache{read,write}, input, output, reasoning} |  |
| session.next.agent.switched | ses_<redacted> |  |  |  |  |
| session.next.model.switched | ses_<redacted> |  |  |  |  |
| message.updated | ses_<redacted> |  |  |  |  |
| message.part.updated | ses_<redacted> | msg_<redacted> | text |  | <prompt-redacted> |
| session.updated | ses_<redacted> |  |  | {cache{read,write}, input, output, reasoning} |  |
| session.status | ses_<redacted> |  |  |  |  |
| message.updated | ses_<redacted> |  |  | {cache{read,write}, input, output, reasoning} |  |
| session.updated | ses_<redacted> |  |  | {cache{read,write}, input, output, reasoning} |  |
| session.diff | ses_<redacted> |  |  |  |  |
| message.updated | ses_<redacted> |  |  |  |  |
| session.status | ses_<redacted> |  |  |  |  |
| message.part.updated | ses_<redacted> | msg_<redacted> | step-start |  |  |
| message.part.updated | ses_<redacted> | msg_<redacted> | text |  |  |
| message.part.delta | ses_<redacted> | msg_<redacted> |  |  | phase |
| message.part.delta | ses_<redacted> | msg_<redacted> |  |  | 2 |
| message.part.delta | ses_<redacted> | msg_<redacted> |  |  | streaming |
| message.part.delta | ses_<redacted> | msg_<redacted> |  |  | verification |
| message.part.delta | ses_<redacted> | msg_<redacted> |  |  | ok |
| message.part.updated | ses_<redacted> | msg_<redacted> | text |  | phase2 streaming verification ok |
| message.part.updated | ses_<redacted> | msg_<redacted> | step-finish | {cache{read,write}, input, output, reasoning, total} |  |
| message.updated | ses_<redacted> |  |  | {cache{read,write}, input, output, reasoning, total} |  |
| message.updated | ses_<redacted> |  |  | {cache{read,write}, input, output, reasoning, total} |  |
| session.status | ses_<redacted> |  |  |  |  |
| session.status | ses_<redacted> |  |  |  |  |
| session.idle | ses_<redacted> |  |  |  |  |
| session.updated | ses_<redacted> |  |  | {cache{read,write}, input, output, reasoning} |  |
| session.diff | ses_<redacted> |  |  |  |  |
| message.updated | ses_<redacted> |  |  |  |  |

## Event Types Observed

- `server.connected`
- `session.created`
- `session.updated`
- `session.next.agent.switched`
- `session.next.model.switched`
- `message.updated`
- `message.part.updated`
- `session.status`
- `session.diff`
- `message.part.delta`
- `session.idle`

## Correlation Fields Observed

- `messageID`
- `part.type`
- `sessionID`

## Completion Signal Chosen

`GET /event` emitted `session.idle` for the target session. `GET /session/status` returned no target entry by the time it was polled. Final status recorded as `idle` after `POST /session/:id/prompt_async`.

## Usage/Token Metadata Source

- event `message.part.updated` token shape: `{cache{read,write}, input, output, reasoning, total}`
- event `message.updated` token shape: `{cache{read,write}, input, output, reasoning, total}`
- event `message.updated` token shape: `{cache{read,write}, input, output, reasoning}`
- event `session.created` token shape: `{cache{read,write}, input, output, reasoning}`
- event `session.updated` token shape: `{cache{read,write}, input, output, reasoning}`
- message info token shape: `{cache{read,write}, input, output, reasoning, total}`
- session token shape: `{cache{read,write}, input, output, reasoning}`

## Final Message Fetch

`GET /session/:id/message` succeeded. Sanitized final text previews:

- `<prompt-redacted>`
- `phase2 streaming verification ok`

## Known Gaps

- None from this live verification run.
