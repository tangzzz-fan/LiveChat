#!/usr/bin/env bash
# health-check.sh — 恢复后系统状态校验
set -euo pipefail

FAIL=0
check() {
  if "$@"; then
    echo "  ✓ $1"
  else
    echo "  ✗ $1"
    FAIL=$((FAIL+1))
  fi
}

echo "=== System Health Check ==="

# Process checks
check "message-service running" pgrep -f "message-service" > /dev/null
check "gateway running" pgrep -f "gateway" > /dev/null
check "outbox-consumer running" pgrep -f "outbox-consumer" > /dev/null

# DB checks
check "PostgreSQL responding" pg_isready > /dev/null 2>&1
check "Redis responding" redis-cli PING > /dev/null 2>&1

# API checks
check "message-service /health" \
  curl -sf http://localhost:8080/health > /dev/null 2>&1
check "gateway /health" \
  curl -sf http://localhost:8081/health > /dev/null 2>&1

# Outbox backpressure check
PENDING=$(curl -sf http://localhost:8082/metrics 2>/dev/null | grep "outbox_pending_count" | awk '{print $2}' | cut -d. -f1)
if [ -n "$PENDING" ] && [ "$PENDING" -lt 10 ]; then
  echo "  ✓ outbox_pending_count=$PENDING (OK)"
else
  echo "  ✗ outbox_pending_count=${PENDING:-unknown} (may have backlog)"
fi

echo ""
if [ "$FAIL" -eq 0 ]; then
  echo "All checks passed"
else
  echo "$FAIL check(s) failed"
  exit 1
fi
