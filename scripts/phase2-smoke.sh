#!/usr/bin/env bash
set -euo pipefail

export GATEWAY_BASE_URL="${GATEWAY_BASE_URL:-http://127.0.0.1:8080}"
export GATEWAY_API_KEY="${GATEWAY_API_KEY:-dev-secret}"
export TEST_USER_ID="${TEST_USER_ID:-phase2-smoke-user-$(date +%s)}"
export TEST_CHAT_ID="${TEST_CHAT_ID:-phase2-smoke-chat-$(date +%s)}"
export STREAM_SMOKE_TIMEOUT_SECONDS="${STREAM_SMOKE_TIMEOUT_SECONDS:-120}"
export DATABASE_PATH="${DATABASE_PATH:-./gateway.sqlite3}"

python3 - <<'PY'
import json
import os
import sqlite3
import sys
import time
import urllib.error
import urllib.request

BASE_URL = os.environ["GATEWAY_BASE_URL"].rstrip("/")
API_KEY = os.environ["GATEWAY_API_KEY"]
USER_ID = os.environ["TEST_USER_ID"]
CHAT_ID = os.environ["TEST_CHAT_ID"]
TIMEOUT = int(os.environ["STREAM_SMOKE_TIMEOUT_SECONDS"])
DB_PATH = os.environ["DATABASE_PATH"]


def pass_(label):
    print(f"PASS {label}")


def fail(label):
    print(f"FAIL {label}", file=sys.stderr)
    raise SystemExit(1)


def request(method, path, body=None, stream=False, timeout=30):
    headers = {"Authorization": f"Bearer {API_KEY}"}
    data = None
    if body is not None:
        data = json.dumps(body).encode("utf-8")
        headers["Content-Type"] = "application/json"
    req = urllib.request.Request(BASE_URL + path, data=data, headers=headers, method=method)
    try:
        resp = urllib.request.urlopen(req, timeout=timeout)
    except urllib.error.HTTPError as exc:
        detail = exc.read().decode("utf-8", errors="replace")[:120]
        fail(f"{method} {path} returned HTTP {exc.code}: {detail}")
    except urllib.error.URLError as exc:
        fail(f"{method} {path} failed: {exc.reason}")
    if stream:
        return resp
    with resp:
        payload = resp.read().decode("utf-8", errors="replace")
        return resp.status, resp.headers, payload


def json_request(method, path, body=None, timeout=30):
    status, headers, payload = request(method, path, body=body, timeout=timeout)
    try:
        return status, headers, json.loads(payload)
    except json.JSONDecodeError:
        fail(f"{method} {path} returned non-JSON response")


def chat_body(model, stream, prompt):
    return {
        "model": model,
        "stream": stream,
        "messages": [{"role": "user", "content": prompt}],
    }


def chat_headers():
    return {
        "Authorization": f"Bearer {API_KEY}",
        "Content-Type": "application/json",
        "X-OpenWebUI-User-Id": USER_ID,
        "X-OpenWebUI-Chat-Id": CHAT_ID,
    }


def post_chat(body, stream=False):
    data = json.dumps(body).encode("utf-8")
    req = urllib.request.Request(BASE_URL + "/v1/chat/completions", data=data, headers=chat_headers(), method="POST")
    try:
        return urllib.request.urlopen(req, timeout=TIMEOUT if stream else min(TIMEOUT, 120))
    except urllib.error.HTTPError as exc:
        detail = exc.read().decode("utf-8", errors="replace")[:120]
        fail(f"POST /v1/chat/completions returned HTTP {exc.code}: {detail}")
    except urllib.error.URLError as exc:
        fail(f"POST /v1/chat/completions failed: {exc.reason}")


def read_stream(resp):
    chunks = []
    saw_done = False
    saw_content_delta = False
    deadline = time.monotonic() + TIMEOUT
    with resp:
        while time.monotonic() < deadline:
            raw = resp.readline()
            if not raw:
                break
            line = raw.decode("utf-8", errors="replace").strip()
            if not line.startswith("data:"):
                continue
            payload = line[5:].strip()
            if payload == "[DONE]":
                saw_done = True
                break
            try:
                obj = json.loads(payload)
            except json.JSONDecodeError:
                fail("stream=true emitted malformed JSON chunk")
            chunks.append(obj)
            if obj.get("object") == "chat.completion.chunk":
                for choice in obj.get("choices", []):
                    content = choice.get("delta", {}).get("content")
                    if isinstance(content, str) and content:
                        saw_content_delta = True
    return chunks, saw_content_delta, saw_done


def ledger_count(*models):
    if not os.path.exists(DB_PATH):
        fail(f"database not found at {DB_PATH}")
    con = sqlite3.connect(DB_PATH)
    try:
        placeholders = ",".join("?" for _ in models)
        row = con.execute(
            f"select count(*) from session_ledger where user_id = ? and chat_id = ? and model_id in ({placeholders})",
            (USER_ID, CHAT_ID, *models),
        ).fetchone()
        return int(row[0])
    finally:
        con.close()


def fetch_debug_counters():
    for path in ("/__debug/opencode/counters", "/debug/opencode/counters"):
        req = urllib.request.Request(BASE_URL + path, headers={"Authorization": f"Bearer {API_KEY}"}, method="GET")
        try:
            with urllib.request.urlopen(req, timeout=5) as resp:
                if resp.status != 200:
                    continue
                return json.loads(resp.read().decode("utf-8"))
        except Exception:
            continue
    return None


status, _, health = json_request("GET", "/health", timeout=15)
if status != 200 or "status" not in health:
    fail("GET /health missing status")
pass_("GET /health")

status, _, models = json_request("GET", "/v1/models", timeout=15)
model_ids = {item.get("id") for item in models.get("data", []) if isinstance(item, dict)}
if status != 200 or not {"adina-analysis", "adina-execution"}.issubset(model_ids):
    fail("GET /v1/models missing expected public models")
if {"plan", "build"} & model_ids:
    fail("GET /v1/models exposed raw OpenCode agents")
pass_("GET /v1/models")

with post_chat(chat_body("adina-analysis", False, "Reply with a short Phase 2 stream=false smoke greeting.")) as resp:
    body = json.loads(resp.read().decode("utf-8", errors="replace"))
if body.get("object") != "chat.completion":
    fail("stream=false did not return chat.completion")
pass_("stream=false chat still works")

if ledger_count("adina-analysis") != 1:
    fail("same user/chat/model did not resolve to exactly one ledger row after stream=false")

counters_before = fetch_debug_counters()
resp = post_chat(chat_body("adina-analysis", True, "Reply with a short incremental Phase 2 streaming smoke greeting."), stream=True)
ctype = resp.headers.get("Content-Type", "")
if "text/event-stream" not in ctype:
    fail(f"stream=true returned Content-Type {ctype!r}")
pass_("stream=true returns text/event-stream")

chunks, saw_content_delta, saw_done = read_stream(resp)
if not any(chunk.get("object") == "chat.completion.chunk" for chunk in chunks):
    fail("stream=true missing chat.completion.chunk")
pass_("stream=true emits chat.completion.chunk")
if not saw_content_delta:
    fail("stream=true missing incremental content delta before DONE")
pass_("stream=true emits incremental content delta before DONE")
if not saw_done:
    fail("stream=true missing data: [DONE]")
pass_("stream=true emits data: [DONE]")

counters_after = fetch_debug_counters()
if counters_before is not None and counters_after is not None:
    before = int(counters_before.get("session_message_get", counters_before.get("get_session_message", 0)))
    after = int(counters_after.get("session_message_get", counters_after.get("get_session_message", 0)))
    if after != before:
        fail("stream=true used GET /session/:id/message without usage request")
    pass_("stream=true does not use /session/:id/message when debug counters are available")
else:
    pass_("/session/:id/message counter verification skipped; no debug counters available")

if ledger_count("adina-analysis") != 1:
    fail("stream=true did not reuse existing ledger row")
pass_("ledger reuse still works")

with post_chat(chat_body("adina-execution", False, "Reply with a short execution-model Phase 2 smoke greeting.")) as resp:
    body = json.loads(resp.read().decode("utf-8", errors="replace"))
if body.get("object") != "chat.completion":
    fail("different model chat did not return chat.completion")
if ledger_count("adina-analysis", "adina-execution") != 2:
    fail("different model did not create a separate ledger row")
pass_("different model creates separate ledger row")
PY
