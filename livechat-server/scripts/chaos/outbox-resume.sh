#!/usr/bin/env bash
# outbox-resume.sh — 恢复 outbox-consumer 进程
set -euo pipefail

ENV="${CHAT_ENV:-dev}"
if [ "$ENV" != "dev" ]; then
  echo "ERROR: CHAT_ENV must be 'dev' for chaos experiments (current: $ENV)"
  exit 1
fi

PID=$(pgrep -f "outbox-consumer" | head -1)
if [ -z "$PID" ]; then
  echo "[chaos] No outbox-consumer process found"
  exit 1
fi

echo "[chaos] Resuming outbox-consumer (PID=$PID)..."
kill -CONT "$PID"
echo "[chaos] Outbox-consumer resumed"
