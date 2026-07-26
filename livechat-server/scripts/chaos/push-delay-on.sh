#!/usr/bin/env bash
# push-delay-on.sh — 打印如何开启推送延迟/失败注入（需重启 outbox-consumer）
set -euo pipefail

ENV="${CHAT_ENV:-dev}"
if [ "$ENV" != "dev" ]; then
  echo "ERROR: CHAT_ENV must be 'dev' for chaos experiments (current: $ENV)"
  exit 1
fi

DELAY_MS="${PUSH_INJECT_DELAY_MS:-5000}"
FAIL="${PUSH_INJECT_FAIL:-0}"

cat <<EOF
[chaos] Push inject (chaos 04)

Export before starting outbox-consumer:

  export CHAT_ENV=dev
  export PUSH_INJECT_DELAY_MS=${DELAY_MS}
  export PUSH_INJECT_FAIL=${FAIL}   # set to 1 to force failure

Then restart outbox-consumer, e.g.:

  cd livechat-server && make run-outbox-consumer

Observe: mock APNs logs show "push inject delay" / "push inject fail";
message send + sync must still succeed (push ≠ durability).

To clear: unset PUSH_INJECT_DELAY_MS PUSH_INJECT_FAIL and restart consumer,
or run: bash livechat-server/scripts/chaos/push-delay-off.sh
EOF
