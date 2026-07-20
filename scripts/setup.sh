#!/bin/bash
# LiveChat Server — 一键环境搭建与启动脚本
# 用法: ./scripts/setup.sh [--skip-deps] [--start]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")/livechat-server"
PG_USER="livechat"
PG_PASSWORD="livechat"
PG_DB="livechat"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log()  { echo -e "${GREEN}[✓]${NC} $1"; }
warn() { echo -e "${YELLOW}[!]${NC} $1"; }
err()  { echo -e "${RED}[✗]${NC} $1"; exit 1; }

SKIP_DEPS=false
START_SERVICES=false
for arg in "$@"; do
    case "$arg" in
        --skip-deps) SKIP_DEPS=true ;;
        --start)     START_SERVICES=true ;;
    esac
done

echo "=============================================="
echo "  LiveChat Server — 环境搭建"
echo "=============================================="
echo ""

# ── Step 1: Check / Install Dependencies ──────────
if [ "$SKIP_DEPS" = false ]; then
    log "检查依赖..."

    # Go
    if ! command -v go &>/dev/null; then
        warn "Go 未安装，通过 Homebrew 安装..."
        brew install go || err "Go 安装失败"
    fi
    log "Go $(go version | awk '{print $3}')"

    # PostgreSQL
    if ! command -v psql &>/dev/null; then
        warn "PostgreSQL 未安装，通过 Homebrew 安装..."
        brew install postgresql@16 || err "PostgreSQL 安装失败"
    fi
    log "PostgreSQL $(psql --version | awk '{print $3}')"

    # Redis
    if ! command -v redis-cli &>/dev/null; then
        warn "Redis 未安装，通过 Homebrew 安装..."
        brew install redis || err "Redis 安装失败"
    fi
    log "Redis $(redis-cli --version | awk '{print $2}')"

    # protoc (optional)
    if command -v protoc &>/dev/null; then
        log "protoc $(protoc --version | awk '{print $2}')"
    else
        warn "protoc 未安装（Proto 文件已预生成，可跳过）"
    fi

    echo ""
fi

# ── Step 2: Start Services ────────────────────────
log "启动 PostgreSQL 和 Redis..."

# PostgreSQL
if brew services list | grep -q "postgresql@16.*started"; then
    log "PostgreSQL 已在运行"
else
    brew services start postgresql@16 2>/dev/null || \
        warn "PostgreSQL 启动失败，尝试手动启动..."
fi

# Redis
if brew services list | grep -q "redis.*started"; then
    log "Redis 已在运行"
else
    brew services start redis 2>/dev/null || \
        warn "Redis 启动失败，尝试手动启动..."
fi

sleep 2

# ── Step 3: Create Database & User ────────────────
log "配置数据库..."

# Create user if not exists
if psql postgres -tAc "SELECT 1 FROM pg_roles WHERE rolname='$PG_USER'" 2>/dev/null | grep -q 1; then
    log "数据库用户 '$PG_USER' 已存在"
else
    createuser "$PG_USER" -s 2>/dev/null || warn "创建用户失败（可能已存在）"
    log "创建数据库用户 '$PG_USER'"
fi

psql postgres -c "ALTER USER $PG_USER WITH PASSWORD '$PG_PASSWORD';" 2>/dev/null || true

# Create database if not exists
if psql -U "$PG_USER" -lqt 2>/dev/null | cut -d \| -f 1 | grep -qw "$PG_DB"; then
    log "数据库 '$PG_DB' 已存在"
else
    createdb "$PG_DB" -O "$PG_USER" 2>/dev/null || \
        warn "创建数据库失败（可能已存在）"
    log "创建数据库 '$PG_DB'"
fi

# ── Step 4: Run Migrations ────────────────────────
log "执行数据库迁移..."
cd "$PROJECT_DIR"
GOPROXY="${GOPROXY:-https://goproxy.cn,direct}" go run ./cmd/migrate up || \
    err "数据库迁移失败"
log "迁移完成"

# ── Step 5: Install Go Dependencies ───────────────
log "安装 Go 依赖..."
GOPROXY="${GOPROXY:-https://goproxy.cn,direct}" go mod download 2>/dev/null || true
GOPROXY="${GOPROXY:-https://goproxy.cn,direct}" go mod tidy 2>/dev/null || true
log "Go 依赖就绪"

# ── Step 6: Build ─────────────────────────────────
log "编译服务..."
go build ./cmd/message-service || err "message-service 编译失败"
go build ./cmd/gateway || err "gateway 编译失败"
go build ./cmd/outbox-consumer || err "outbox-consumer 编译失败"
log "编译完成"

echo ""
echo "=============================================="
echo -e "  ${GREEN}环境搭建完成！${NC}"
echo "=============================================="
echo ""
echo "启动服务："
echo ""
echo "  # 终端 1: HTTP API (端口 8080)"
echo "  cd livechat-server && go run ./cmd/message-service"
echo ""
echo "  # 终端 2: WebSocket 网关 (端口 8081)"
echo "  cd livechat-server && go run ./cmd/gateway"
echo ""
echo "  # 终端 3: Outbox 消费者"
echo "  cd livechat-server && go run ./cmd/outbox-consumer"
echo ""
echo "验证："
echo "  curl http://localhost:8080/health"
echo ""

# ── Step 7 (optional): Start Services ─────────────
if [ "$START_SERVICES" = true ]; then
    log "启动所有服务（后台运行）..."
    cd "$PROJECT_DIR"

    # Kill any existing instances
    pkill -f "message-service" 2>/dev/null || true
    pkill -f "gateway" 2>/dev/null || true
    pkill -f "outbox-consumer" 2>/dev/null || true
    sleep 1

    nohup go run ./cmd/message-service > /tmp/livechat-msg.log 2>&1 &
    echo "  message-service  PID: $!"
    nohup go run ./cmd/gateway > /tmp/livechat-gw.log 2>&1 &
    echo "  gateway          PID: $!"
    nohup go run ./cmd/outbox-consumer > /tmp/livechat-ob.log 2>&1 &
    echo "  outbox-consumer  PID: $!"

    sleep 3

    # Verify
    if curl -s http://localhost:8080/health > /dev/null 2>&1; then
        echo ""
        log "服务已启动并通过健康检查"
        curl -s http://localhost:8080/health | python3 -m json.tool 2>/dev/null || true
    else
        warn "服务启动中，请稍后检查..."
    fi

    echo ""
    echo "查看日志:"
    echo "  tail -f /tmp/livechat-msg.log"
    echo "  tail -f /tmp/livechat-gw.log"
    echo "  tail -f /tmp/livechat-ob.log"
    echo ""
    echo "停止服务:"
    echo "  pkill -f 'message-service|gateway|outbox-consumer'"
fi

echo "完成！ 🎉"
