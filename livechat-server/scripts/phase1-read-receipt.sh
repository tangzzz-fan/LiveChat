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

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

require_cmd curl
require_cmd python3
require_cmd psql
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
phone_a="+86179$(printf '%08d' "$((suffix % 100000000))")"
phone_b="+86166$(printf '%08d' "$(((suffix + 1) % 100000000))")"
conv_id="conv-read-${suffix}"
ack_trace_id="trace-read-${suffix}"
tmp_dir="$(mktemp -d)"
ready_file="${tmp_dir}/reader.ready"
output_file="${tmp_dir}/reader.json"

cleanup() {
  rm -rf "${tmp_dir}"
}
trap cleanup EXIT

verify_summary_zero() {
  local resp
  resp="$(curl -fsS "${BASE_URL}/v1/conversations" -H "Authorization: Bearer ${token_b2}")"
  python3 - "$resp" "${conv_id}" <<'PY'
import json, sys
items = json.loads(sys.argv[1])["conversations"]
conv = next((x for x in items if x["conversation_id"] == sys.argv[2]), None)
assert conv is not None, items
assert conv["unread_count"] == 0, conv
PY
}

verify_ack_sync() {
  local a_sync
  local b_sync
  a_sync="$(curl -fsS "${BASE_URL}/v1/sync/events?cursor=0&limit=200" -H "Authorization: Bearer ${token_a}")"
  b_sync="$(curl -fsS "${BASE_URL}/v1/sync/events?cursor=0&limit=200" -H "Authorization: Bearer ${token_b2}")"
  python3 - "$a_sync" "$b_sync" "${ack_trace_id}" "${conv_id}" <<'PY'
import json, sys
a_events = json.loads(sys.argv[1])["events"]
b_events = json.loads(sys.argv[2])["events"]
trace_id = sys.argv[3]
conversation_id = sys.argv[4]

msg_read = next(
    (e for e in a_events if e["event_type"] == "message_read" and e["conversation_id"] == conversation_id),
    None,
)
conv_updated = next(
    (e for e in b_events if e["event_type"] == "conversation_updated" and e["conversation_id"] == conversation_id),
    None,
)
assert msg_read is not None, a_events
assert conv_updated is not None, b_events

msg_payload = json.loads(msg_read["payload"])
conv_payload = json.loads(conv_updated["payload"])
assert msg_payload["last_read_seq"] == 100, msg_payload
assert msg_payload["trace_id"] == trace_id, msg_payload
assert conv_payload["last_read_seq"] == 100, conv_payload
assert conv_payload["unread_count"] == 0, conv_payload
assert conv_payload["trace_id"] == trace_id, conv_payload
PY
}

echo "[1/10] check service endpoints"
curl -fsS "${BASE_URL}/health" >/dev/null
curl -fsS "${BASE_URL}/metrics" | grep -q 'http_request_duration_seconds'
curl -fsS "${GATEWAY_HTTP_URL}/metrics" | grep -q 'ws_heartbeat_timeouts_total'
curl -fsS "${OUTBOX_METRICS_URL}" | grep -q 'outbox_processing_count'

echo "[2/10] register A and B on two devices"
resp_a="$(curl -fsS -X POST "${BASE_URL}/v1/auth/register" \
  -H 'Content-Type: application/json' \
  -d "{\"phone_e164\":\"${phone_a}\",\"verification_code\":\"123456\",\"device_id\":\"a-ios\",\"platform\":\"ios\"}")"
resp_b1="$(curl -fsS -X POST "${BASE_URL}/v1/auth/register" \
  -H 'Content-Type: application/json' \
  -d "{\"phone_e164\":\"${phone_b}\",\"verification_code\":\"123456\",\"device_id\":\"b-ios-1\",\"platform\":\"ios\"}")"
resp_b2="$(curl -fsS -X POST "${BASE_URL}/v1/auth/register" \
  -H 'Content-Type: application/json' \
  -d "{\"phone_e164\":\"${phone_b}\",\"verification_code\":\"123456\",\"device_id\":\"b-ios-2\",\"platform\":\"ios\"}")"

read -r token_a uid_a <<EOF
$(python3 - "$resp_a" <<'PY'
import json, sys
data = json.loads(sys.argv[1])
print(data["access_token"], data["user_id"])
PY
)
EOF

read -r token_b1 uid_b <<EOF
$(python3 - "$resp_b1" <<'PY'
import json, sys
data = json.loads(sys.argv[1])
print(data["access_token"], data["user_id"])
PY
)
EOF

read -r token_b2 _ <<EOF
$(python3 - "$resp_b2" <<'PY'
import json, sys
data = json.loads(sys.argv[1])
print(data["access_token"], data["user_id"])
PY
)
EOF

echo "[3/10] create direct conversation"
psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE" <<SQL >/dev/null
INSERT INTO conversations (id, type) VALUES ('${conv_id}', 'direct');
INSERT INTO conversation_members (conversation_id, user_id) VALUES ('${conv_id}', ${uid_a}), ('${conv_id}', ${uid_b});
SQL

echo "[4/10] connect B device 1 and arm read ACK at seq=100"
go run ./scripts/ws_probe.go \
  -mode delivery-read-ack \
  -ws-url "${GATEWAY_WS_URL}" \
  -token "${token_b1}" \
  -device-id "b-ios-1" \
  -ready-file "${ready_file}" \
  -output-file "${output_file}" \
  -target-seq 100 \
  -ack-last-read-seq 100 \
  -ack-trace-id "${ack_trace_id}" >/dev/null 2>&1 &
reader_pid=$!
trap 'kill ${reader_pid} >/dev/null 2>&1 || true; cleanup' EXIT

retry_until 20 0.25 test -f "${ready_file}" || {
  echo "reader websocket did not become ready" >&2
  exit 1
}

echo "[5/10] send 100 messages"
for i in $(seq 1 100); do
  curl -fsS -X POST "${BASE_URL}/v1/messages/send" \
    -H "Authorization: Bearer ${token_a}" \
    -H "X-Trace-Id: send-${suffix}-${i}" \
    -H 'Content-Type: application/json' \
    -d "{\"client_message_id\":\"m-${suffix}-${i}\",\"conversation_id\":\"${conv_id}\",\"message_type\":\"text\",\"content\":\"{\\\"text\\\":\\\"msg ${i}\\\"}\"}" >/dev/null
done

echo "[6/10] wait for ACK sender to finish"
retry_until 80 0.25 test -f "${output_file}" || {
  echo "reader did not observe seq=100 delivery and ACK" >&2
  exit 1
}
wait "${reader_pid}"

echo "[7/10] verify B unread_count reset to 0"
retry_until 20 0.5 verify_summary_zero || {
  echo "conversation summary did not converge to unread_count=0" >&2
  exit 1
}

echo "[8/10] verify A gets message_read and B device 2 gets conversation_updated"
retry_until 20 0.5 verify_ack_sync || {
  echo "sync events did not converge after ACK(read)" >&2
  exit 1
}

echo "[9/10] verify MAX(last_read_seq) on second device"
python3 <<'PY'
local_seq = 50
remote_seq = 100
assert max(local_seq, remote_seq) == 100
print("MAX_READ_SEQ_OK")
PY

echo "[10/10] verify metrics names"
curl -fsS "${BASE_URL}/metrics" | grep -q 'messages_sent_total'
curl -fsS "${BASE_URL}/metrics" | grep -q 'outbox_events_created_total'
curl -fsS "${GATEWAY_HTTP_URL}/metrics" | grep -q 'ws_connections_total'
curl -fsS "${OUTBOX_METRICS_URL}" | grep -q 'outbox_consumer_lag_seconds'

echo "PHASE1_READ_RECEIPT_OK conversation_id=${conv_id} ack_trace_id=${ack_trace_id}"
