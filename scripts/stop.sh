#!/bin/bash
# LiveChat Server — 一键停止所有服务
set -euo pipefail

echo "停止 LiveChat 服务..."
pkill -f "message-service" 2>/dev/null && echo "  ✓ message-service 已停止" || echo "  - message-service 未运行"
pkill -f "gateway" 2>/dev/null && echo "  ✓ gateway 已停止" || echo "  - gateway 未运行"
pkill -f "outbox-consumer" 2>/dev/null && echo "  ✓ outbox-consumer 已停止" || echo "  - outbox-consumer 未运行"

# Clean up port occupancy
lsof -ti:8080 | xargs kill -9 2>/dev/null || true
lsof -ti:8081 | xargs kill -9 2>/dev/null || true

echo "完成"
