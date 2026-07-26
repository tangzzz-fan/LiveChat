#!/usr/bin/env bash
# health-check.sh — 恢复后系统状态校验
set -uo pipefail

FAIL=0

check() {
  local label="$1"
  shift
  if "$@" >/dev/null 2>&1; then
    echo "  ✓ $label"
  else
    echo "  ✗ $label"
    FAIL=$((FAIL + 1))
  fi
}

# 优先匹配编译后的二进制；`go run` 启动时进程名是临时文件，回退到命令行匹配。
process_running() {
  local name="$1"
  pgrep -x "$name" >/dev/null 2>&1 || pgrep -f "cmd/${name}\b" >/dev/null 2>&1
}

echo "=== System Health Check ==="

# Process checks
check "message-service running" process_running message-service
check "gateway running" process_running gateway
check "outbox-consumer running" process_running outbox-consumer

# DB checks
check "PostgreSQL responding" pg_isready
check "Redis responding" redis-cli PING

# API checks
check "message-service /health" curl -sf http://localhost:8080/health
check "gateway /metrics" curl -sf http://localhost:8081/metrics

# Outbox backlog check
PENDING=$(curl -sf http://localhost:8082/metrics 2>/dev/null | grep "^outbox_pending_count" | awk '{print $2}' | cut -d. -f1)
if [ -n "${PENDING:-}" ] && [ "$PENDING" -lt 10 ]; then
  echo "  ✓ outbox_pending_count=$PENDING (OK)"
else
  echo "  ✗ outbox_pending_count=${PENDING:-unknown} (may have backlog)"
  FAIL=$((FAIL + 1))
fi

# Send-side backpressure (ticket 0032): informational, not a failure signal.
BP=$(curl -sf http://localhost:8080/metrics 2>/dev/null | grep "^send_backpressure_" | awk '{printf "%s=%d ", $1, $2}')
if [ -n "${BP:-}" ]; then
  echo "  · ${BP}"
fi

echo ""
if [ "$FAIL" -eq 0 ]; then
  echo "All checks passed"
else
  echo "$FAIL check(s) failed"
  exit 1
fi
