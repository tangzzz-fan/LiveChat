#!/usr/bin/env bash
# outbox-pause.sh — 暂停 outbox-consumer 进程
set -euo pipefail

ENV="${CHAT_ENV:-dev}"
if [ "$ENV" != "dev" ]; then
  echo "ERROR: CHAT_ENV must be 'dev' for chaos experiments (current: $ENV)"
  exit 1
fi

# Match the binary itself, not shell wrappers whose command line merely
# contains "outbox-consumer" (pausing a wrapper leaves the consumer running).
PID=$(pgrep -x "outbox-consumer" | head -1)
if [ -z "$PID" ]; then
  echo "[chaos] No outbox-consumer process found (expects the built binary; run 'make build' then './outbox-consumer')"
  exit 1
fi

echo "[chaos] Pausing outbox-consumer (PID=$PID)..."
kill -STOP "$PID"
echo "[chaos] Outbox-consumer paused"
