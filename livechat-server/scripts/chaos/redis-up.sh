#!/usr/bin/env bash
# redis-up.sh — 恢复 Redis
set -euo pipefail

ENV="${CHAT_ENV:-dev}"
if [ "$ENV" != "dev" ]; then
  echo "ERROR: CHAT_ENV must be 'dev' for chaos experiments (current: $ENV)"
  exit 1
fi

echo "[chaos] Starting Redis..."
brew services start redis 2>/dev/null || redis-server --daemonize yes 2>/dev/null || true

sleep 2
echo "[chaos] Redis started"
redis-cli PING && echo "[chaos] Redis health check: OK"
