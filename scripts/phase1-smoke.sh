#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
API_KEY="${GATEWAY_API_KEY:-dev-secret}"
DB_PATH="${DATABASE_PATH:-./gateway.sqlite3}"

pass() { printf 'PASS %s\n' "$1"; }
fail() { printf 'FAIL %s\n' "$1" >&2; exit 1; }

request() {
  curl -sS "$@"
}

health=$(request "$BASE_URL/health") || fail 'GET /health failed'
printf '%s' "$health" | grep -q '"status"' || fail 'GET /health missing status'
pass 'GET /health'

models=$(request "$BASE_URL/v1/models" -H "Authorization: Bearer $API_KEY") || fail 'GET /v1/models failed'
printf '%s' "$models" | grep -q 'adina-analysis' || fail 'adina-analysis missing'
printf '%s' "$models" | grep -q 'adina-execution' || fail 'adina-execution missing'
printf '%s' "$models" | grep -q '"id":"plan"' && fail 'raw plan model exposed'
printf '%s' "$models" | grep -q '"id":"build"' && fail 'raw build model exposed'
pass 'GET /v1/models'

chat_false=$(request "$BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer $API_KEY" \
  -H 'Content-Type: application/json' \
  -H 'X-OpenWebUI-User-Id: smoke-user' \
  -H 'X-OpenWebUI-Chat-Id: smoke-chat' \
  -d '{"model":"adina-analysis","stream":false,"messages":[{"role":"user","content":"Reply with a short smoke-test greeting."}]}') || fail 'stream=false chat failed'
printf '%s' "$chat_false" | grep -q '"object":"chat.completion"' || fail 'stream=false did not return chat.completion'
pass 'POST /v1/chat/completions stream=false'

chat_true=$(request "$BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer $API_KEY" \
  -H 'Content-Type: application/json' \
  -H 'X-OpenWebUI-User-Id: smoke-user' \
  -H 'X-OpenWebUI-Chat-Id: smoke-chat' \
  -d '{"model":"adina-analysis","stream":true,"messages":[{"role":"user","content":"Reply with a short streaming smoke-test greeting."}]}') || fail 'stream=true chat failed'
printf '%s' "$chat_true" | grep -q 'chat.completion.chunk' || fail 'stream=true missing chunks'
printf '%s' "$chat_true" | grep -q 'data: \[DONE\]' || fail 'stream=true missing DONE'
pass 'POST /v1/chat/completions stream=true'

missing_user_status=$(curl -sS -o /tmp/gateway-smoke-missing-user.json -w '%{http_code}' "$BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer $API_KEY" \
  -H 'Content-Type: application/json' \
  -H 'X-OpenWebUI-Chat-Id: smoke-chat' \
  -d '{"model":"adina-analysis","messages":[{"role":"user","content":"hi"}]}')
[ "$missing_user_status" = "400" ] || fail "missing user returned $missing_user_status"
pass 'missing X-OpenWebUI-User-Id returns 400'

missing_chat_status=$(curl -sS -o /tmp/gateway-smoke-missing-chat.json -w '%{http_code}' "$BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer $API_KEY" \
  -H 'Content-Type: application/json' \
  -H 'X-OpenWebUI-User-Id: smoke-user' \
  -d '{"model":"adina-analysis","messages":[{"role":"user","content":"hi"}]}')
[ "$missing_chat_status" = "400" ] || fail "missing chat returned $missing_chat_status"
pass 'missing X-OpenWebUI-Chat-Id returns 400'

unknown_model_status=$(curl -sS -o /tmp/gateway-smoke-unknown-model.json -w '%{http_code}' "$BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer $API_KEY" \
  -H 'Content-Type: application/json' \
  -H 'X-OpenWebUI-User-Id: smoke-user' \
  -H 'X-OpenWebUI-Chat-Id: smoke-chat' \
  -d '{"model":"unknown","messages":[{"role":"user","content":"hi"}]}')
[ "$unknown_model_status" = "404" ] || fail "unknown model returned $unknown_model_status"
pass 'unknown model returns 404'

if [ -f "$DB_PATH" ]; then
  python3 - "$DB_PATH" <<'PY'
import sqlite3, sys
db = sys.argv[1]
con = sqlite3.connect(db)
same = con.execute("select count(*) from session_ledger where user_id='smoke-user' and chat_id='smoke-chat' and model_id='adina-analysis'").fetchone()[0]
if same != 1:
    raise SystemExit(f"expected one adina-analysis ledger row, got {same}")
con.close()
PY
  pass 'same user/chat/model reuses one ledger row'
else
  fail "database not found at $DB_PATH"
fi

request "$BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer $API_KEY" \
  -H 'Content-Type: application/json' \
  -H 'X-OpenWebUI-User-Id: smoke-user' \
  -H 'X-OpenWebUI-Chat-Id: smoke-chat' \
  -d '{"model":"adina-execution","messages":[{"role":"user","content":"Reply with a short execution smoke-test greeting."}]}' >/dev/null || fail 'different model chat failed'

python3 - "$DB_PATH" <<'PY'
import sqlite3, sys
db = sys.argv[1]
con = sqlite3.connect(db)
rows = con.execute("select count(*) from session_ledger where user_id='smoke-user' and chat_id='smoke-chat' and model_id in ('adina-analysis','adina-execution')").fetchone()[0]
if rows < 2:
    raise SystemExit(f"expected separate rows for different models, got {rows}")
con.close()
PY
pass 'different model creates separate ledger row'
