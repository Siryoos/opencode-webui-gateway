# Open WebUI Runtime Verification Pending

Open WebUI was not running locally during initial Phase 1 implementation verification. A later user report identified the expected runtime target as container `open-webui-adina` at `http://127.0.0.1:3000`.

Checks attempted:

```bash
docker ps --format '{{.Names}} {{.Image}} {{.Ports}}'
curl -sS -i --max-time 5 http://127.0.0.1:3000/health
```

Result: no Open WebUI container or process was reachable on the default local port.

Before connecting Open WebUI, runtime curl tests must include:

- `Authorization: Bearer <token>`
- `X-OpenWebUI-User-Id: <user-id>`
- `X-OpenWebUI-Chat-Id: <chat-id>`

Open WebUI must be configured to forward user information headers. In Open WebUI deployments, this generally requires `ENABLE_FORWARD_USER_INFO_HEADERS=true`.

Remaining manual checks after the synchronous SSE shim:

- Configure Gateway URL as `http://host.docker.internal:8080/v1` or equivalent.
- Verify `adina-analysis` and `adina-execution` appear in the model selector.
- Send a chat from Open WebUI and confirm the request contains `stream: true`.
- Confirm the response succeeds as `text/event-stream` and ends with `data: [DONE]`.
- Confirm the SQLite ledger contains real Open WebUI user/chat IDs, not fallback identities.
