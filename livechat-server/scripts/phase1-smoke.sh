#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
PGHOST="${PGHOST:-localhost}"
PGPORT="${PGPORT:-5432}"
PGUSER="${PGUSER:-livechat}"
PGDATABASE="${PGDATABASE:-livechat}"
REDIS_HOST="${REDIS_HOST:-localhost}"
REDIS_PORT="${REDIS_PORT:-6379}"

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

require_cmd curl
require_cmd python3
require_cmd psql
require_cmd redis-cli

export PGPASSWORD="${PGPASSWORD:-livechat}"

retry_until() {
  local attempts="$1"
  local delay_seconds="$2"
  shift 2

  local i
  for ((i = 1; i <= attempts; i++)); do
    if "$@"; then
      return 0
    fi
    sleep "$delay_seconds"
  done
  return 1
}

suffix="$(date +%s)"
phone_a="+86139$(printf '%08d' "$((suffix % 100000000))")"
phone_b="+86158$(printf '%08d' "$(((suffix + 1) % 100000000))")"
conv_id="conv-smoke-${suffix}"

echo "[1/10] check postgres"
psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE" -c 'select 1;' >/dev/null

echo "[2/10] check redis"
redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" ping | grep -q '^PONG$'

echo "[3/10] check message-service health"
health_json="$(curl -fsS "${BASE_URL}/health")"
python3 - "$health_json" <<'PY'
import json, sys
data = json.loads(sys.argv[1])
assert data["status"] == "ok", data
assert data["details"]["postgres"] == "ok", data
assert data["details"]["redis"] == "ok", data
PY

echo "[4/10] check metrics endpoint"
metrics_output="$(curl -fsS "${BASE_URL}/metrics")"
grep -q "http_requests_total" <<<"$metrics_output" || {
  echo "metrics endpoint does not expose expected counters" >&2
  exit 1
}

echo "[5/10] register users (new two-step auth)"
resp_a="$(curl -fsS -X POST "${BASE_URL}/v1/auth/request_code" \
  -H 'Content-Type: application/json' \
  -d "{\"phone_e164\":\"${phone_a}\"}")"

resp_a_v="$(curl -fsS -X POST "${BASE_URL}/v1/auth/verify_code" \
  -H 'Content-Type: application/json' \
  -d "{\"phone_e164\":\"${phone_a}\",\"verification_code\":\"123456\",\"device_id\":\"a-ios\",\"platform\":\"ios\"}")"

resp_b="$(curl -fsS -X POST "${BASE_URL}/v1/auth/request_code" \
  -H 'Content-Type: application/json' \
  -d "{\"phone_e164\":\"${phone_b}\"}")"

resp_b_v="$(curl -fsS -X POST "${BASE_URL}/v1/auth/verify_code" \
  -H 'Content-Type: application/json' \
  -d "{\"phone_e164\":\"${phone_b}\",\"verification_code\":\"123456\",\"device_id\":\"b-android\",\"platform\":\"android\"}")"

read -r token_a uid_a <<EOF
$(python3 - "$resp_a_v" <<'PY'
import json, sys
data = json.loads(sys.argv[1])
print(data["access_token"], data["user_id"])
PY
)
EOF

read -r token_b uid_b <<EOF
$(python3 - "$resp_b_v" <<'PY'
import json, sys
data = json.loads(sys.argv[1])
print(data["access_token"], data["user_id"])
PY
)
EOF

echo "[6/10] create direct conversation"
psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE" <<SQL >/dev/null
INSERT INTO conversations (id, type) VALUES ('${conv_id}', 'direct');
INSERT INTO conversation_members (conversation_id, user_id) VALUES ('${conv_id}', ${uid_a}), ('${conv_id}', ${uid_b});
SQL

echo "[7/10] send message and verify idempotency"
send_resp_1="$(curl -fsS -X POST "${BASE_URL}/v1/messages/send" \
  -H "Authorization: Bearer ${token_a}" \
  -H 'Content-Type: application/json' \
  -d "{\"client_message_id\":\"m1\",\"conversation_id\":\"${conv_id}\",\"message_type\":\"text\",\"content\":\"{\\\"text\\\":\\\"Hello from A\\\"}\"}")"

send_resp_2="$(curl -fsS -X POST "${BASE_URL}/v1/messages/send" \
  -H "Authorization: Bearer ${token_a}" \
  -H 'Content-Type: application/json' \
  -d "{\"client_message_id\":\"m1\",\"conversation_id\":\"${conv_id}\",\"message_type\":\"text\",\"content\":\"{\\\"text\\\":\\\"Hello from A\\\"}\"}")"

read -r server_message_id conversation_seq <<EOF
$(python3 - "$send_resp_1" "$send_resp_2" <<'PY'
import json, sys
first = json.loads(sys.argv[1])
second = json.loads(sys.argv[2])
assert first["is_duplicate"] is False, first
assert second["is_duplicate"] is True, second
assert first["conversation_seq"] == 1, first
assert second["conversation_seq"] == first["conversation_seq"], (first, second)
assert first["server_message_id"] == second["server_message_id"], (first, second)
print(first["server_message_id"], first["conversation_seq"])
PY
)
EOF

echo "[8/10] verify conversation summary"
conversations_resp="$(curl -fsS "${BASE_URL}/v1/conversations" -H "Authorization: Bearer ${token_b}")"
python3 - "$conversations_resp" "$conv_id" <<'PY'
import json, sys
data = json.loads(sys.argv[1])
cid = sys.argv[2]
items = data["conversations"]
match = next((x for x in items if x["conversation_id"] == cid), None)
assert match is not None, data
assert match["unread_count"] >= 1, match
PY

echo "[9/10] verify sync events"
verify_sync_events() {
  local sync_resp
  sync_resp="$(curl -fsS "${BASE_URL}/v1/sync/events?cursor=0" -H "Authorization: Bearer ${token_b}")"
  python3 - "$sync_resp" "$conv_id" <<'PY'
import json, sys
data = json.loads(sys.argv[1])
cid = sys.argv[2]
events = data["events"]
match = next((e for e in events if e["event_type"] == "message_created"), None)
if match is None:
    sys.exit(1)
payload = json.loads(match["payload"])
if payload["conversation_id"] != cid:
    sys.exit(1)
PY
}

retry_until 10 0.5 verify_sync_events || {
  echo "sync events not visible within retry window" >&2
  exit 1
}

echo "[10/10] verify message backfill"
messages_resp="$(curl -fsS "${BASE_URL}/v1/conversations/${conv_id}/messages?from_seq=1" -H "Authorization: Bearer ${token_b}")"
python3 - "$messages_resp" "$server_message_id" <<'PY'
import json, sys
data = json.loads(sys.argv[1])
server_message_id = sys.argv[2]
messages = data["messages"]
assert len(messages) >= 1, data
match = next((m for m in messages if m["server_message_id"] == server_message_id), None)
assert match is not None, data
assert match["conversation_seq"] == 1, match
PY

echo "PHASE1_SMOKE_OK conversation_id=${conv_id} server_message_id=${server_message_id}"
