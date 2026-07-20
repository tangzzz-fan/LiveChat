#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
GATEWAY_WS_URL="${GATEWAY_WS_URL:-ws://localhost:8081/ws}"
GATEWAY_HTTP_URL="${GATEWAY_HTTP_URL:-http://localhost:8081}"
OUTBOX_METRICS_URL="${OUTBOX_METRICS_URL:-http://localhost:8082/metrics}"
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
require_cmd go

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
phone_a="+86177$(printf '%08d' "$((suffix % 100000000))")"
phone_b="+86188$(printf '%08d' "$(((suffix + 1) % 100000000))")"
conv_id="conv-rt-${suffix}"
trace_id="trace-rt-${suffix}"
tmp_dir="$(mktemp -d)"
ready_file="${tmp_dir}/listener.ready"
output_file="${tmp_dir}/delivery.json"

cleanup() {
  rm -rf "${tmp_dir}"
}
trap cleanup EXIT

echo "[1/9] check postgres and redis"
psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE" -c 'select 1;' >/dev/null
redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" ping | grep -q '^PONG$'

echo "[2/9] check service endpoints"
curl -fsS "${BASE_URL}/health" >/dev/null
curl -fsS "${BASE_URL}/metrics" | grep -q 'http_requests_total'
curl -fsS "${GATEWAY_HTTP_URL}/health" >/dev/null
curl -fsS "${GATEWAY_HTTP_URL}/metrics" | grep -q 'ws_connections_active'
curl -fsS "${OUTBOX_METRICS_URL}" | grep -q 'outbox_pending_count'

echo "[3/9] register users"
resp_a="$(curl -fsS -X POST "${BASE_URL}/v1/auth/register" \
  -H 'Content-Type: application/json' \
  -d "{\"phone_e164\":\"${phone_a}\",\"verification_code\":\"123456\",\"device_id\":\"a-ios\",\"platform\":\"ios\"}")"
resp_b="$(curl -fsS -X POST "${BASE_URL}/v1/auth/register" \
  -H 'Content-Type: application/json' \
  -d "{\"phone_e164\":\"${phone_b}\",\"verification_code\":\"123456\",\"device_id\":\"b-ios\",\"platform\":\"ios\"}")"

read -r token_a uid_a <<EOF
$(python3 - "$resp_a" <<'PY'
import json, sys
data = json.loads(sys.argv[1])
print(data["access_token"], data["user_id"])
PY
)
EOF

read -r token_b uid_b <<EOF
$(python3 - "$resp_b" <<'PY'
import json, sys
data = json.loads(sys.argv[1])
print(data["access_token"], data["user_id"])
PY
)
EOF

echo "[4/9] create direct conversation"
psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE" <<SQL >/dev/null
INSERT INTO conversations (id, type) VALUES ('${conv_id}', 'direct');
INSERT INTO conversation_members (conversation_id, user_id) VALUES ('${conv_id}', ${uid_a}), ('${conv_id}', ${uid_b});
SQL

echo "[5/9] connect receiver websocket"
go run ./scripts/ws_probe.go \
  -mode delivery \
  -ws-url "${GATEWAY_WS_URL}" \
  -token "${token_b}" \
  -device-id "b-ios" \
  -ready-file "${ready_file}" \
  -output-file "${output_file}" >/dev/null 2>&1 &
listener_pid=$!
trap 'kill ${listener_pid} >/dev/null 2>&1 || true; cleanup' EXIT

retry_until 20 0.25 test -f "${ready_file}" || {
  echo "websocket listener did not become ready" >&2
  exit 1
}

echo "[6/9] send message"
curl -fsS -X POST "${BASE_URL}/v1/messages/send" \
  -H "Authorization: Bearer ${token_a}" \
  -H "X-Trace-Id: ${trace_id}" \
  -H 'Content-Type: application/json' \
  -d "{\"client_message_id\":\"m-${suffix}\",\"conversation_id\":\"${conv_id}\",\"message_type\":\"text\",\"content\":\"{\\\"text\\\":\\\"hello realtime\\\"}\"}" >/dev/null

echo "[7/9] assert websocket delivery"
retry_until 40 0.25 test -f "${output_file}" || {
  echo "delivery frame not observed within retry window" >&2
  exit 1
}
python3 - "${output_file}" "${conv_id}" "${trace_id}" <<'PY'
import json, sys
with open(sys.argv[1], "r", encoding="utf-8") as fh:
    data = json.load(fh)
assert data["conversation_id"] == sys.argv[2], data
assert data["frame_trace_id"] == sys.argv[3], data
PY

echo "[8/9] assert sync fallback event includes trace"
sync_resp="$(curl -fsS "${BASE_URL}/v1/sync/events?cursor=0" -H "Authorization: Bearer ${token_b}")"
python3 - "$sync_resp" "$trace_id" <<'PY'
import json, sys
events = json.loads(sys.argv[1])["events"]
target = next(e for e in events if e["event_type"] == "message_created")
payload = json.loads(target["payload"])
assert payload["trace_id"] == sys.argv[2], payload
PY

echo "[9/9] assert metrics names"
curl -fsS "${GATEWAY_HTTP_URL}/metrics" | grep -q 'ws_connections_total'
curl -fsS "${OUTBOX_METRICS_URL}" | grep -q 'outbox_consumer_lag_seconds'

wait "${listener_pid}"
echo "PHASE1_REALTIME_DELIVERY_OK conversation_id=${conv_id} trace_id=${trace_id}"
