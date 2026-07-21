#!/usr/bin/env bash
# db-resume.sh — 恢复 PostgreSQL
set -euo pipefail

ENV="${CHAT_ENV:-dev}"
if [ "$ENV" != "dev" ]; then
  echo "ERROR: CHAT_ENV must be 'dev' for chaos experiments (current: $ENV)"
  exit 1
fi

echo "[chaos] Starting PostgreSQL..."
brew services start postgresql@16 2>/dev/null || true

sleep 3
echo "[chaos] PostgreSQL started"
pg_isready && echo "[chaos] PostgreSQL health check: OK"
