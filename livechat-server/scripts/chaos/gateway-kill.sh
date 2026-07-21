#!/usr/bin/env bash
# gateway-kill.sh — 终止 gateway 进程（模拟节点宕机）
set -euo pipefail

ENV="${CHAT_ENV:-dev}"
if [ "$ENV" != "dev" ]; then
  echo "ERROR: CHAT_ENV must be 'dev' for chaos experiments (current: $ENV)"
  exit 1
fi

PID=$(pgrep -f "/gateway" | head -1)
if [ -z "$PID" ]; then
  echo "[chaos] No gateway process found"
  exit 1
fi

echo "[chaos] Killing gateway (PID=$PID)..."
kill -9 "$PID"
echo "[chaos] Gateway killed"
echo "[chaos] Restart with: make run-gateway"
