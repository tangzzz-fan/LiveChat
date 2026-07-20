# LiveChat Server — 操作说明

## 环境要求

| 组件 | 版本 | 说明 |
|------|------|------|
| Go | 1.22+ | 编译语言 |
| PostgreSQL | 16+ | 主数据库 |
| Redis | 7+ | 网关路由缓存 |
| protoc | 任意 | Protobuf 代码生成（可选，已生成 pb.go 文件） |

**macOS 快速安装：**
```bash
brew install go postgresql@16 redis protobuf
brew services start postgresql@16
brew services start redis
```

## 目录结构

```text
livechat-server/
├── cmd/
│   ├── message-service/    # HTTP API 服务（消息发送、认证、同步）
│   ├── gateway/            # WebSocket 长连接网关
│   ├── outbox-consumer/    # Outbox 消费者（投递 + 同步）
│   └── migrate/            # 数据库迁移工具
├── internal/
│   ├── api/                # HTTP 路由、handler、中间件
│   ├── auth/               # JWT 签发与验证
│   ├── conversations/      # 会话摘要投影
│   ├── domain/             # 共享领域类型
│   ├── fanout/             # 消息投递编排
│   ├── gateway/            # WebSocket 会话管理、帧协议
│   ├── infra/              # PostgreSQL、Redis 连接
│   ├── messages/           # 消息发送核心（幂等写入 + Outbox）
│   ├── metrics/            # Prometheus 指标
│   ├── outbox/             # Outbox 消费者（轮询、分发、重试）
│   └── sync/               # 增量同步事件、游标管理
├── proto/                  # Protobuf schema + 生成代码
├── migrations/             # PostgreSQL DDL
├── configs/                # 服务配置文件
├── docker-compose.yml      # 容器化开发环境
├── Makefile                # 常用命令入口
└── go.mod                  # Go module 定义
```

## 快速开始

### 1. 创建数据库

```bash
createuser livechat -s
psql postgres -c "ALTER USER livechat WITH PASSWORD 'livechat';"
createdb livechat -O livechat
```

### 2. 运行数据库迁移

```bash
go run ./cmd/migrate up
```

这会创建 8 张表：`users` → `devices` → `conversations` + `conversation_members` → `messages` → `outbox_events` → `sync_events` → `sync_cursors` → `conversation_summaries`

### 3. 启动服务（三个独立进程）

```bash
# 终端 1：HTTP API 服务（端口 8080）
go run ./cmd/message-service

# 终端 2：WebSocket 网关（端口 8081）
go run ./cmd/gateway

# 终端 3：Outbox 消费者（无端口，轮询 DB）
go run ./cmd/outbox-consumer
```

### 4. 验证服务是否正常

```bash
# 健康检查
curl http://localhost:8080/health

# Prometheus 指标
curl http://localhost:8080/debug/vars
```

## Makefile 常用命令

```bash
make dev              # 启动 PG + Redis（Docker 方式）
make migrate-up       # 执行数据库迁移
make migrate-down     # 回滚数据库迁移
make proto            # 从 .proto 生成 Go 代码
make build            # 编译所有二进制
make test             # 运行测试
make run-message-service  # 启动 Message Service
make run-gateway          # 启动 Gateway
make run-outbox-consumer  # 启动 Outbox Consumer
```

## API 文档

### 认证（无需 JWT）

#### POST /v1/auth/register — 注册

```bash
curl -s -X POST http://localhost:8080/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{
    "phone_e164": "+8613800000001",
    "verification_code": "123456",
    "device_id": "ios-dev-001",
    "platform": "ios"
  }'
```

响应：
```json
{
  "access_token": "eyJhbGci...",
  "refresh_token": "a1b2c3...",
  "expires_in": 3600,
  "user_id": 1
}
```

- 首次注册自动创建 user
- 已注册手机号重复注册返回已有 user
- P0 阶段：验证码为 mock，接受任意 6 位数字

#### POST /v1/auth/login — 登录

```bash
curl -s -X POST http://localhost:8080/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{
    "phone_e164": "+8613800000001",
    "verification_code": "654321",
    "device_id": "ios-dev-001",
    "platform": "ios"
  }'
```

响应格式同 register。

---

### 消息发送（需要 JWT）

#### POST /v1/messages/send — 发送消息

```bash
curl -s -X POST http://localhost:8080/v1/messages/send \
  -H "Authorization: Bearer <token>" \
  -H 'Content-Type: application/json' \
  -d '{
    "client_message_id": "client-msg-001",
    "conversation_id": "conv-abc",
    "message_type": "text",
    "content": "{\"text\": \"Hello!\"}"
  }'
```

响应：
```json
{
  "server_message_id": "msg_conv-abc_000001",
  "conversation_seq": 1,
  "is_duplicate": false,
  "server_received_at_ms": 1752681600000
}
```

幂等重发（相同 `client_message_id`）：
```json
{
  "server_message_id": "msg_conv-abc_000001",
  "conversation_seq": 1,
  "is_duplicate": true,
  "server_received_at_ms": 1752681600000
}
```

错误码：
- `400` — 缺少必填字段
- `401` — 无 JWT 或 JWT 过期
- `403` — 发送者不是该会话的成员

---

### 同步（需要 JWT）

#### GET /v1/sync/events — 增量同步事件

```bash
curl -s "http://localhost:8080/v1/sync/events?cursor=0&limit=10" \
  -H "Authorization: Bearer <token>"
```

参数：
- `cursor` — 上次同步到的 event_seq（默认 0 = 从头开始）
- `limit` — 每页数量（默认 100，最大 200）

响应：
```json
{
  "events": [
    {
      "event_seq": 1,
      "user_id": 2,
      "event_type": "message_created",
      "payload": "{\"server_message_id\":\"...\",\"conversation_id\":\"...\",...}",
      "created_at": "2026-07-20T11:00:00+08:00"
    }
  ],
  "has_more": false,
  "latest_event_seq": 1,
  "server_time_ms": 1752681600000
}
```

每次同步完成后自动推进 cursor。

#### GET /v1/conversations/{cid}/messages — 会话消息补拉

```bash
curl -s "http://localhost:8080/v1/conversations/conv-abc/messages?from_seq=1&limit=50" \
  -H "Authorization: Bearer <token>"
```

参数：
- `from_seq` — conversation_seq 起点（默认 0）
- `limit` — 每页数量（默认 50，最大 100）

#### GET /v1/conversations — 会话列表

```bash
curl -s "http://localhost:8080/v1/conversations?limit=50&offset=0" \
  -H "Authorization: Bearer <token>"
```

响应按 `last_message_at DESC` 排序，含 `unread_count`。

---

### WebSocket 协议（Gateway 端口 8081）

#### 连接与握手

```
ws://localhost:8081/ws
```

1. 客户端连接 WebSocket
2. 发送 Binary 帧（Protobuf `WsFrame`，opcode `0x0001`）
   - payload: `HandshakeRequest { access_token, device_id, platform }`
3. 服务端验证 JWT，回复 `0x0002` `HandshakeResponse { session_id, heartbeat_interval_s: 30 }`

#### 心跳

- 客户端每 30s 发送 opcode `0x0003` (`HEARTBEAT`)
- 服务端回复 `0x0004` (`HEARTBEAT_ACK`)
- 90s 无帧 → 服务端发送 `0x0006` (`ERROR`, `should_reconnect=true`) 并断开

#### 消息投递（服务端 → 客户端）

- 新消息通过 opcode `0x1001` (`MESSAGE_DELIVERY`) 实时推送
- payload: `WsMessageDelivery { server_message_id, conversation_id, conversation_seq, ... }`

#### Opcode 速查

| Opcode | 方向 | 用途 |
|--------|------|------|
| 0x0001 | C→S | 握手 |
| 0x0002 | S→C | 握手响应 |
| 0x0003 | C→S | 心跳 |
| 0x0004 | S→C | 心跳应答 |
| 0x0005 | 双向 | 业务 ACK |
| 0x0006 | S→C | 错误 |
| 0x0007 | 双向 | 断开通知 |
| 0x1001 | S→C | 消息投递 |
| 0x2001 | S→C | 同步事件 |
| 0x2002 | S→C | 会话更新 |

完整定义见 `proto/ws_frame.proto`。

---

### 运维端点

```bash
# 健康检查
curl http://localhost:8080/health
# → {"status":"ok","details":{"postgres":"ok","redis":"ok"}}

# Metrics（Prometheus 格式）
curl http://localhost:8080/debug/vars

# Gateway 健康 + 活跃连接数
curl http://localhost:8081/health
# → {"status":"ok","active_sessions":3}
```

## 完整端到端验证流程

```bash
# 1. 注册两个用户
RESP_A=$(curl -s -X POST http://localhost:8080/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"phone_e164":"+8613800000001","verification_code":"123456","device_id":"a-ios","platform":"ios"}')
TOKEN_A=$(echo "$RESP_A" | python3 -c 'import sys,json; print(json.load(sys.stdin)["access_token"])')

RESP_B=$(curl -s -X POST http://localhost:8080/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"phone_e164":"+8613800000002","verification_code":"123456","device_id":"b-android","platform":"android"}')
TOKEN_B=$(echo "$RESP_B" | python3 -c 'import sys,json; print(json.load(sys.stdin)["access_token"])')

# 2. 创建会话并添加成员
psql -U livechat livechat -c "INSERT INTO conversations (id, type) VALUES ('conv-1', 'direct');"
psql -U livechat livechat -c "INSERT INTO conversation_members (conversation_id, user_id) VALUES ('conv-1', 1), ('conv-1', 2);"

# 3. A 发送消息
curl -s -X POST http://localhost:8080/v1/messages/send \
  -H "Authorization: Bearer $TOKEN_A" \
  -H 'Content-Type: application/json' \
  -d '{"client_message_id":"m1","conversation_id":"conv-1","message_type":"text","content":"{\"text\":\"Hello from A\"}"}'

# 4. B 查看会话列表
curl -s "http://localhost:8080/v1/conversations" -H "Authorization: Bearer $TOKEN_B"
# → 应显示 unread_count: 1

# 5. B 查看同步事件
curl -s "http://localhost:8080/v1/sync/events?cursor=0" -H "Authorization: Bearer $TOKEN_B"
# → 应返回 message_created 事件

# 6. B 读取具体消息
curl -s "http://localhost:8080/v1/conversations/conv-1/messages?from_seq=1" -H "Authorization: Bearer $TOKEN_B"
# → 应返回 A 发送的消息
```

## Phase 2 功能验证命令

以下命令用于验证 Phase 2 新增功能的端到端行为。执行前需确保：

```bash
# 一键启动所有服务
./scripts/setup.sh --start
```

### 快速环境初始化（所有测试共用）

```bash
RESP_A=$(curl -s -X POST http://localhost:8080/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"phone_e164":"+8613800000001","verification_code":"123456","device_id":"ios-A","platform":"ios"}')
TOKEN_A=$(echo "$RESP_A" | python3 -c 'import sys,json; print(json.load(sys.stdin)["access_token"])')
UID_A=$(echo "$RESP_A" | python3 -c 'import sys,json; print(json.load(sys.stdin)["user_id"])')

RESP_B=$(curl -s -X POST http://localhost:8080/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"phone_e164":"+8613800000002","verification_code":"123456","device_id":"android-B","platform":"android"}')
TOKEN_B=$(echo "$RESP_B" | python3 -c 'import sys,json; print(json.load(sys.stdin)["access_token"])')
UID_B=$(echo "$RESP_B" | python3 -c 'import sys,json; print(json.load(sys.stdin)["user_id"])')

psql -U livechat livechat -c "INSERT INTO conversations (id, type) VALUES ('conv-1', 'direct') ON CONFLICT DO NOTHING;"
psql -U livechat livechat -c "INSERT INTO conversation_members (conversation_id, user_id) VALUES ('conv-1', $UID_A), ('conv-1', $UID_B) ON CONFLICT DO NOTHING;"

echo "UID_A=$UID_A  UID_B=$UID_B"
```

### 0010: Refresh Token 轮换

```bash
REFRESH=$(curl -s -X POST http://localhost:8080/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"phone_e164":"+8613800000003","verification_code":"123456","device_id":"dev-refresh","platform":"ios"}' \
  | python3 -c 'import sys,json; print(json.load(sys.stdin)["refresh_token"])')

echo "refresh_token: $REFRESH"

curl -s -X POST http://localhost:8080/v1/auth/refresh \
  -H 'Content-Type: application/json' \
  -d "{\"refresh_token\": \"$REFRESH\"}" | python3 -m json.tool
# 应返回新的 access_token + refresh_token（轮换后旧 token 失效）
```

### 0011: 设备列表与吊销

```bash
# 查看设备列表
curl -s "http://localhost:8080/v1/devices" \
  -H "Authorization: Bearer $TOKEN_A" | python3 -m json.tool
# 应返回 devices 数组，is_current=true

# 吊销设备
curl -s -X POST "http://localhost:8080/v1/devices/ios-A/revoke" \
  -H "Authorization: Bearer $TOKEN_A" | python3 -m json.tool
# 应返回 {"revoked":"ios-A"}
```

### 0012/0013: 群聊创建与成员管理

```bash
# 创建群
GRESP=$(curl -s -X POST http://localhost:8080/v1/groups \
  -H "Authorization: Bearer $TOKEN_A" \
  -H 'Content-Type: application/json' \
  -d '{"name":"测试群聊","description":"Phase 2 验证"}')
echo "$GRESP" | python3 -m json.tool

GID=$(echo "$GRESP" | python3 -c 'import sys,json; print(json.load(sys.stdin)["group"]["id"])')
CID=$(echo "$GRESP" | python3 -c 'import sys,json; print(json.load(sys.stdin)["conversation_id"])')
echo "Group: $GID, Conv: $CID"

# 加人
curl -s -X POST "http://localhost:8080/v1/groups/$GID/members" \
  -H "Authorization: Bearer $TOKEN_A" \
  -H 'Content-Type: application/json' \
  -d "{\"user_ids\": [$UID_B]}" | python3 -m json.tool
# 应返回 {"added":1}

# 查看成员
curl -s "http://localhost:8080/v1/groups/$GID/members" \
  -H "Authorization: Bearer $TOKEN_A" | python3 -m json.tool
# 应含 owner (UID_A) 和 member (UID_B)

# 踢人
curl -s -X DELETE "http://localhost:8080/v1/groups/$GID/members/$UID_B" \
  -H "Authorization: Bearer $TOKEN_A" | python3 -m json.tool
# 应返回 {"removed":<UID_B>}
```

### 0006/0016: 群聊消息 + 同步验证

```bash
# 重新加 B 入群（如果上面已踢出）
curl -s -X POST "http://localhost:8080/v1/groups/$GID/members" \
  -H "Authorization: Bearer $TOKEN_A" \
  -H 'Content-Type: application/json' \
  -d "{\"user_ids\": [$UID_B]}" > /dev/null

# A 在群聊中发消息
curl -s -X POST http://localhost:8080/v1/messages/send \
  -H "Authorization: Bearer $TOKEN_A" \
  -H 'Content-Type: application/json' \
  -d "{\"client_message_id\":\"group-msg-1\",\"conversation_id\":\"$CID\",\"message_type\":\"text\",\"content\":\"{\\\"text\\\":\\\"大家好\\\"}\"}" \
  | python3 -m json.tool
# 应返回 server_message_id + conversation_seq

# B 拉取群聊消息
curl -s "http://localhost:8080/v1/conversations/$CID/messages?from_seq=1" \
  -H "Authorization: Bearer $TOKEN_B" | python3 -m json.tool
# 应看到 A 的消息

# B 查看同步事件
curl -s "http://localhost:8080/v1/sync/events?cursor=0" \
  -H "Authorization: Bearer $TOKEN_B" | python3 -m json.tool
# 应含 message_created 事件
```

### 权限拒绝验证

```bash
# 第三人非群成员
TOKEN_C=$(curl -s -X POST http://localhost:8080/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"phone_e164":"+8613800000005","verification_code":"123456","device_id":"ios-C","platform":"ios"}' \
  | python3 -c 'import sys,json; print(json.load(sys.stdin)["access_token"])')

# 非成员发消息 → 403
curl -s -X POST http://localhost:8080/v1/messages/send \
  -H "Authorization: Bearer $TOKEN_C" \
  -H 'Content-Type: application/json' \
  -d "{\"client_message_id\":\"bad\",\"conversation_id\":\"$CID\",\"message_type\":\"text\",\"content\":\"{}\"}" \
  | python3 -m json.tool
# 应返回 403

# 非成员读消息 → 403
curl -s "http://localhost:8080/v1/conversations/$CID/messages?from_seq=1" \
  -H "Authorization: Bearer $TOKEN_C" | python3 -m json.tool
# 应返回 403
```

### 幂等性验证

```bash
for i in 1 2; do
  echo "=== 第 $i 次 ==="
  curl -s -X POST http://localhost:8080/v1/messages/send \
    -H "Authorization: Bearer $TOKEN_A" \
    -H 'Content-Type: application/json' \
    -d "{\"client_message_id\":\"idem-test\",\"conversation_id\":\"conv-1\",\"message_type\":\"text\",\"content\":\"{\\\"text\\\":\\\"幂等\\\"}\"}" \
    | python3 -m json.tool
done
# 第 1 次: is_duplicate=false
# 第 2 次: is_duplicate=true
```

## 数据库表速查

| 表 | 用途 | 关键约束 |
|----|------|----------|
| `users` | 用户账号 | `phone_e164 UNIQUE` |
| `devices` | 设备会话 | FK → users |
| `conversations` | 会话 | `type IN ('direct','group')` |
| `conversation_members` | 会话成员 | PK(`conversation_id`, `user_id`) |
| `messages` | 消息记录 | `UNIQUE(sender_user_id, client_message_id)` 幂等 |
| `outbox_events` | 事务性事件 | `status IN ('pending','processing','retry','done','failed')` |
| `sync_events` | 增量同步流 | PK(`user_id`, `event_seq`)，按 user 分片 |
| `sync_cursors` | 设备游标 | PK(`user_id`, `device_id`) |
| `conversation_summaries` | 会话列表投影 | PK(`user_id`, `conversation_id`) |

## 配置

服务配置见 `configs/` 目录。主要配置项：

| 配置 | 默认值 | 说明 |
|------|--------|------|
| `database.host:port` | `localhost:5432` | PostgreSQL 地址 |
| `redis.host:port` | `localhost:6379` | Redis 地址 |
| `auth.jwt_secret` | `livechat-dev-...` | JWT 签名密钥（生产需更换） |
| `auth.access_token_ttl` | `1h` | Token 有效期 |
| `outbox.poll_interval_ms` | `100` | Consumer 轮询间隔 |
| `outbox.max_retries` | `10` | 最大重试次数 |
| `gateway.heartbeat_interval` | `30s` | 心跳间隔 |
| `gateway.read_timeout` | `90s` | 读超时 |
| `server.port` | `8080` / `8081` | HTTP / WS 监听端口 |
