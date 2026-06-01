#!/usr/bin/env bash
set -euo pipefail

export OPENCODE_BASE_URL="${OPENCODE_BASE_URL:-http://127.0.0.1:4096}"
export OPENCODE_SERVER_USERNAME="${OPENCODE_SERVER_USERNAME:-opencode}"
export OPENCODE_SERVER_PASSWORD="${OPENCODE_SERVER_PASSWORD:-}"
export STREAM_DISCOVERY_TIMEOUT_SECONDS="${STREAM_DISCOVERY_TIMEOUT_SECONDS:-120}"

python3 - <<'PY'
import base64
import json
import os
import queue
import re
import sys
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
from datetime import date

BASE_URL = os.environ["OPENCODE_BASE_URL"].rstrip("/")
USERNAME = os.environ["OPENCODE_SERVER_USERNAME"]
PASSWORD = os.environ.get("OPENCODE_SERVER_PASSWORD", "")
TIMEOUT_SECONDS = int(os.environ["STREAM_DISCOVERY_TIMEOUT_SECONDS"])
DOC_PATH = "docs/discovery/phase2-streaming-live-verification.md"
PROMPT = "Reply with exactly: phase2 streaming verification ok"

event_records = []
event_types = []
target_events = []
text_event_records = []
token_sources = []
correlation_fields = set()
errors = []
connected_seen = threading.Event()
target_idle_seen = threading.Event()
stop_stream = threading.Event()
event_queue = queue.Queue()


def auth_header():
    if not PASSWORD:
        return {}
    raw = f"{USERNAME}:{PASSWORD}".encode("utf-8")
    return {"Authorization": "Basic " + base64.b64encode(raw).decode("ascii")}


def request(method, path, body=None, accept="application/json", timeout=30):
    headers = {"Accept": accept}
    headers.update(auth_header())
    data = None
    if body is not None:
        data = json.dumps(body).encode("utf-8")
        headers["Content-Type"] = "application/json"
    req = urllib.request.Request(BASE_URL + path, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            payload = resp.read()
            ctype = resp.headers.get("Content-Type", "")
            if payload and "json" in ctype:
                return resp.status, json.loads(payload.decode("utf-8"))
            if payload:
                return resp.status, payload.decode("utf-8", errors="replace")
            return resp.status, None
    except urllib.error.HTTPError as exc:
        payload = exc.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"{method} {path} returned HTTP {exc.code}: {redact_text(payload)}") from exc
    except urllib.error.URLError as exc:
        raise RuntimeError(f"{method} {path} failed: {redact_text(str(exc.reason))}") from exc


def redact_id(value):
    if not isinstance(value, str):
        return value
    if value.startswith("ses_"):
        return "ses_<redacted>"
    if value.startswith("msg_"):
        return "msg_<redacted>"
    if value.startswith("prt_"):
        return "prt_<redacted>"
    return value


def redact_text(value):
    if value is None:
        return ""
    text = str(value).replace(PROMPT, "<prompt-redacted>")
    text = text.replace(PASSWORD, "<password-redacted>") if PASSWORD else text
    text = re.sub(r"/[^\s'\"]+", "<path-redacted>", text)
    text = re.sub(r"```.*?```", "<block-redacted>", text, flags=re.S)
    text = " ".join(text.split())
    if len(text) > 80:
        return text[:77] + "..."
    return text


def walk(value):
    if isinstance(value, dict):
        yield value
        for child in value.values():
            yield from walk(child)
    elif isinstance(value, list):
        for child in value:
            yield from walk(child)


def first_key(value, names):
    for obj in walk(value):
        for name in names:
            if name in obj and obj[name] not in (None, ""):
                return obj[name]
    return None


def all_key_values(value, names):
    out = []
    for obj in walk(value):
        for name in names:
            if name in obj and obj[name] not in (None, ""):
                out.append(obj[name])
    return out


def token_shape(tokens):
    if not isinstance(tokens, dict):
        return "<none>"
    fields = []
    for key in sorted(tokens.keys()):
        val = tokens[key]
        if isinstance(val, dict):
            fields.append(f"{key}{{{','.join(sorted(val.keys()))}}}")
        else:
            fields.append(key)
    return "{" + ", ".join(fields) + "}"


def text_preview(value):
    texts = []
    for obj in walk(value):
        for key in ("text", "content", "delta"):
            val = obj.get(key)
            if isinstance(val, str) and val.strip():
                texts.append(redact_text(val))
    return texts[0] if texts else ""


def summarize_event(data, session_id=None):
    etype = first_key(data, ("type", "event")) or "<unknown>"
    sid = first_key(data, ("sessionID", "sessionId", "session_id"))
    mid = first_key(data, ("messageID", "messageId", "message_id"))
    part_type = None
    for obj in walk(data):
        if obj.get("type") and ("messageID" in obj or "text" in obj or str(obj.get("id", "")).startswith("prt_")):
            part_type = obj.get("type")
            break
    tokens = first_key(data, ("tokens", "usage"))
    preview = text_preview(data)
    if sid:
        correlation_fields.add("sessionID")
    if mid:
        correlation_fields.add("messageID")
    if part_type:
        correlation_fields.add("part.type")
    if tokens:
        token_sources.append(f"event `{etype}` token shape: `{token_shape(tokens)}`")
    record = {
        "type": str(etype),
        "sessionID": redact_id(sid) if sid else "",
        "messageID": redact_id(mid) if mid else "",
        "partType": str(part_type) if part_type else "",
        "tokenShape": token_shape(tokens) if tokens else "",
        "textPreview": preview,
    }
    if etype not in event_types:
        event_types.append(str(etype))
    event_records.append(record)
    if str(etype) == "server.connected":
        connected_seen.set()
    if session_id and sid == session_id:
        target_events.append(record)
        if str(etype) == "session.idle":
            target_idle_seen.set()
        if preview:
            text_event_records.append(record)
    event_queue.put(record)


def stream_events(session_id_holder):
    req = urllib.request.Request(BASE_URL + "/event", headers={"Accept": "text/event-stream", **auth_header()}, method="GET")
    try:
        with urllib.request.urlopen(req, timeout=TIMEOUT_SECONDS) as resp:
            event_name = None
            data_lines = []
            while not stop_stream.is_set():
                raw = resp.readline()
                if not raw:
                    break
                line = raw.decode("utf-8", errors="replace").rstrip("\r\n")
                if line.startswith(":"):
                    continue
                if line.startswith("event:"):
                    event_name = line[6:].strip()
                    continue
                if line.startswith("data:"):
                    data_lines.append(line[5:].strip())
                    continue
                if line == "" and data_lines:
                    payload = "\n".join(data_lines)
                    data_lines = []
                    try:
                        data = json.loads(payload)
                    except json.JSONDecodeError:
                        data = {"type": event_name or "<raw>", "text": payload}
                    if event_name and isinstance(data, dict) and "type" not in data:
                        data["type"] = event_name
                    summarize_event(data, session_id_holder.get("id"))
    except Exception as exc:
        if not stop_stream.is_set():
            errors.append(f"GET /event failed: {redact_text(exc)}")


def status_type(statuses, session_id):
    if isinstance(statuses, dict):
        status = statuses.get(session_id)
        if isinstance(status, dict):
            return status.get("type") or "<unknown>"
        if isinstance(status, str):
            return status
    return "<missing>"


def extract_final_texts(messages):
    texts = []
    for msg in messages if isinstance(messages, list) else []:
        for part in msg.get("parts", []):
            if isinstance(part, dict) and part.get("type") == "text" and isinstance(part.get("text"), str):
                texts.append(redact_text(part["text"]))
        info = msg.get("info", {}) if isinstance(msg, dict) else {}
        if isinstance(info.get("tokens"), dict):
            token_sources.append(f"message info token shape: `{token_shape(info['tokens'])}`")
    return texts


def print_pass(label):
    print(f"PASS {label}")


def print_fail(label):
    print(f"FAIL {label}")


def md_table(rows):
    if not rows:
        return "No target-session SSE events observed."
    lines = ["| Event type | Session ID | Message ID | Part type | Token metadata shape | Text preview |", "|---|---|---|---|---|---|"]
    for row in rows[:40]:
        lines.append("| " + " | ".join([
            row.get("type") or "",
            row.get("sessionID") or "",
            row.get("messageID") or "",
            row.get("partType") or "",
            row.get("tokenShape") or "",
            row.get("textPreview") or "",
        ]) + " |")
    return "\n".join(lines)


def write_doc(verdict, blocker, version, session_id, final_status, completion_signal, message_fetch_ok, message_texts):
    auth_mode = "Basic Auth configured; credentials were not printed." if PASSWORD else "unsecured local development; no Basic Auth header sent."
    token_doc = "\n".join(f"- {source}" for source in sorted(set(token_sources))) or "- No token metadata observed in SSE events or fetched messages."
    known_gaps = []
    if not text_event_records:
        known_gaps.append("No target-session text-bearing SSE event was observed during this run.")
    if not token_sources:
        known_gaps.append("Token metadata source was not observed during this run.")
    if not target_events:
        known_gaps.append("No target-session SSE correlation events were observed after `prompt_async`.")
    if errors:
        known_gaps.extend(errors)
    if not known_gaps:
        known_gaps.append("None from this live verification run.")
    doc = f"""# Phase 2 Streaming Live Verification

Date: {date.today().isoformat()}

Base URL: `{BASE_URL}`

Authentication mode: {auth_mode}

OpenCode version: `{version or '<unknown>'}`

## Verdict

{verdict}: {('enough runtime evidence exists to implement real streaming' if verdict == 'PASS' else 'OpenCode does not reliably emit enough data/events')}

Blocker: {blocker or 'None.'}

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
GET {BASE_URL}/global/health
GET {BASE_URL}/event
POST {BASE_URL}/session
POST {BASE_URL}/session/{redact_id(session_id or 'ses_<pending>')}/prompt_async
GET {BASE_URL}/session/status
GET {BASE_URL}/session/{redact_id(session_id or 'ses_<pending>')}/message
```

## Sanitized Observed SSE Events

{md_table(target_events)}

## Event Types Observed

{chr(10).join(f'- `{etype}`' for etype in event_types) or '- None.'}

## Correlation Fields Observed

{chr(10).join(f'- `{field}`' for field in sorted(correlation_fields)) or '- None.'}

## Completion Signal Chosen

{completion_signal} Final status recorded as `{final_status}` after `POST /session/:id/prompt_async`.

## Usage/Token Metadata Source

{token_doc}

## Final Message Fetch

`GET /session/:id/message` {'succeeded' if message_fetch_ok else 'did not complete successfully'}. Sanitized final text previews:

{chr(10).join(f'- `{text}`' for text in message_texts[:5]) or '- No text parts found in fetched messages.'}

## Known Gaps

{chr(10).join(f'- {gap}' for gap in known_gaps)}
"""
    with open(DOC_PATH, "w", encoding="utf-8") as fh:
        fh.write(doc)


def main():
    version = ""
    session_id = ""
    final_status = "<missing>"
    completion_signal = "No completion signal was observed."
    message_fetch_ok = False
    message_texts = []
    blocker = ""
    verdict = "BLOCKED"
    session_id_holder = {}

    try:
        status, health = request("GET", "/global/health", timeout=15)
        if status != 200:
            raise RuntimeError(f"GET /global/health returned {status}")
        version = health.get("version", "") if isinstance(health, dict) else ""
        print_pass("GET /global/health")

        thread = threading.Thread(target=stream_events, args=(session_id_holder,), daemon=True)
        thread.start()
        if not connected_seen.wait(timeout=min(20, TIMEOUT_SECONDS)):
            raise RuntimeError("GET /event did not receive server.connected before timeout")
        print_pass("GET /event server.connected")

        status, session = request("POST", "/session", {"title": "gateway-phase2-streaming-live-verification"}, timeout=30)
        if status not in (200, 201) or not isinstance(session, dict) or not session.get("id"):
            raise RuntimeError("POST /session did not return a session id")
        session_id = session["id"]
        session_id_holder["id"] = session_id
        if isinstance(session.get("tokens"), dict):
            token_sources.append(f"session token shape: `{token_shape(session['tokens'])}`")
        print_pass("POST /session")

        body = {"agent": "plan", "parts": [{"type": "text", "text": PROMPT}]}
        status, _ = request("POST", f"/session/{urllib.parse.quote(session_id)}/prompt_async", body, timeout=30)
        if status != 204:
            raise RuntimeError(f"POST /session/:id/prompt_async returned {status}, expected 204")
        print_pass("POST /session/:id/prompt_async 204")

        deadline = time.monotonic() + TIMEOUT_SECONDS
        saw_target = False
        while time.monotonic() < deadline:
            if target_events:
                saw_target = True
                break
            time.sleep(0.25)
        if not saw_target:
            raise RuntimeError("/event emitted no events correlated to the target session after prompt_async")
        print_pass("/event target-session events")

        while time.monotonic() < deadline:
            status, statuses = request("GET", "/session/status", timeout=15)
            polled_status = status_type(statuses, session_id)
            if polled_status != "<missing>":
                final_status = polled_status
            if polled_status == "idle":
                completion_signal = "`GET /session/status` returned `idle` for the target session."
                final_status = "idle"
                break
            if target_idle_seen.is_set():
                completion_signal = "`GET /event` emitted `session.idle` for the target session. `GET /session/status` returned no target entry by the time it was polled."
                final_status = "idle"
                break
            time.sleep(1)
        if final_status != "idle":
            raise RuntimeError(f"target session did not reach idle status; last status {final_status}")
        print_pass("target session idle")

        status, messages = request("GET", f"/session/{urllib.parse.quote(session_id)}/message", timeout=30)
        if status != 200:
            raise RuntimeError(f"GET /session/:id/message returned {status}")
        message_fetch_ok = True
        message_texts = extract_final_texts(messages)
        if not message_texts:
            raise RuntimeError("GET /session/:id/message returned no final text parts")
        print_pass("GET /session/:id/message")

        if not text_event_records:
            blocker = "Target session completed and final messages were fetchable, but no text-bearing target-session SSE event was observed."
            print_fail("text-bearing target-session SSE event")
        else:
            print_pass("text-bearing target-session SSE event")
            verdict = "PASS"
    except Exception as exc:
        blocker = redact_text(exc)
        errors.append(blocker)
        print_fail(blocker)
    finally:
        stop_stream.set()
        write_doc(verdict, blocker, version, session_id, final_status, completion_signal, message_fetch_ok, message_texts)
        print(f"Discovery doc written: {DOC_PATH}")
        print(f"Discovery verdict: {verdict}")
        if blocker:
            print(f"Blocker: {blocker}")
        if verdict != "PASS":
            sys.exit(1)


if __name__ == "__main__":
    main()
PY
