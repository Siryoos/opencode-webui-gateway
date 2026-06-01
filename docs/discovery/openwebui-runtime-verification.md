# Open WebUI Runtime Verification

Date: 2026-06-01

## Reported Runtime

Open WebUI container name: `open-webui-adina`

Open WebUI URL: `http://127.0.0.1:3000`

Gateway URL configured in Open WebUI: `http://host.docker.internal:8080/v1` or equivalent host-reachable gateway URL.

API key: redacted.

`ENABLE_FORWARD_USER_INFO_HEADERS=true` is required so Open WebUI sends:

- `X-OpenWebUI-User-Id`
- `X-OpenWebUI-Chat-Id`

## Verified / Reported Behavior

- `adina-analysis` appears in the model selector.
- `adina-execution` is expected from `GET /v1/models` and should appear in the model selector.
- Open WebUI sends `stream: true` for chat.
- Before the synchronous SSE shim, `stream: true` failed with `streaming is not supported in Phase 1`.
- After the synchronous SSE shim, `stream: true` is expected to succeed without using OpenCode `/event`, `/global/event`, or `prompt_async`.
- Gateway ledger must receive real `X-OpenWebUI-User-Id` and `X-OpenWebUI-Chat-Id` values.

## Sanitized Ledger Example

```text
user_id=<openwebui-user-id>
chat_id=<openwebui-chat-id>
model_id=adina-analysis
opencode_session_id=ses_<redacted>
```

## Known Limitations

- This is a synchronous SSE compatibility shim, not true streaming.
- Usage chunks are not emitted because usage is not fabricated.
- Phase 1 does not claim multi-user OpenCode workspace isolation.
- If Open WebUI does not forward user info headers, chat requests fail with `400`.
