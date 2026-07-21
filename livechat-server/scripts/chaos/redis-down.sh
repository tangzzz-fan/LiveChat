#!/usr/bin/env bash
# redis-down.sh — 停止 Redis（用于故障演练）
set -euo pipefail

ENV="${CHAT_ENV:-dev}"
if [ "$ENV" != "dev" ]; then
  echo "ERROR: CHAT_ENV must be 'dev' for chaos experiments (current: $ENV)"
  exit 1
fi

echo "[chaos] Stopping Redis..."
brew services stop redis 2>/dev/null || redis-cli SHUTDOWN 2>/dev/null || true

sleep 1
echo "[chaos] Redis stopped"
