#!/usr/bin/env bash
# db-pause.sh — 暂停 PostgreSQL
set -euo pipefail

ENV="${CHAT_ENV:-dev}"
if [ "$ENV" != "dev" ]; then
  echo "ERROR: CHAT_ENV must be 'dev' for chaos experiments (current: $ENV)"
  exit 1
fi

echo "[chaos] Stopping PostgreSQL..."
brew services stop postgresql@16 2>/dev/null || true

sleep 1
echo "[chaos] PostgreSQL stopped"
