#!/usr/bin/env bash
# push-delay-off.sh — 说明如何关闭推送注入
set -euo pipefail

ENV="${CHAT_ENV:-dev}"
if [ "$ENV" != "dev" ]; then
  echo "ERROR: CHAT_ENV must be 'dev' for chaos experiments (current: $ENV)"
  exit 1
fi

cat <<'EOF'
[chaos] Clear push inject env and restart outbox-consumer:

  unset PUSH_INJECT_DELAY_MS
  unset PUSH_INJECT_FAIL
  # kill and restart outbox-consumer
  cd livechat-server && make run-outbox-consumer

Then: bash livechat-server/scripts/chaos/health-check.sh
EOF
