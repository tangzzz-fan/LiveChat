---
id: "0004"
title: "Gateway：WebSocket 握手 + 心跳 + 用户路由注册"
status: ready-for-agent
labels: ["ready-for-agent"]
parent: "0001"
blocked_by: ["0002"]
created_at: 2026-07-20
---

## Parent

[0001 - 阶段一：消息正确性骨架](0001-phase-1-message-correctness-skeleton.md)

## What to build

实现 Gateway 的连接管理基础能力：WebSocket 升级 → 握手鉴权 → 会话建立 → 心跳保活 → 用户路由注册/注销。

端到端行为：客户端通过 WebSocket 连接 Gateway，发送 `HANDSHAKE_REQ`（含 JWT）→ Gateway 验证 JWT，创建 session，注册 Redis 路由（`gateway:user:{uid}:{device_id}` = `{node_id}:{conn_id}`）→ 返回 `HANDSHAKE_RESP`（含 session_id、心跳间隔、server_time）→ 客户端每 30s 发 `HEARTBEAT`，Gateway 回复 `HEARTBEAT_ACK` 并续期 Redis TTL → 90s 无帧则服务端断开。

具体交付：

- Protobuf schema（`proto/ws_frame.proto`）：定义 `WsFrame`、`HandshakeRequest`、`HandshakeResponse`、`Heartbeat`、`HeartbeatAck`、`MessageAck`、`ErrorFrame`，实现 Spec 05 §3.1 的全部 message 类型和 §3.2 的系统级 opcode（0x0001–0x0008）。
- `internal/gateway/session.go`：Session 结构体（`session_id`、`user_id`、`device_id`、`ws_conn`、`last_read_at`、`ctx`），NewSession 创建，Close 清理。
- `internal/gateway/handler.go`：opcode 分发路由——`HANDSHAKE_REQ` → 鉴权 + 握手；`HEARTBEAT` → 回复 ACK + 续期 Redis TTL；`DISCONNECT` → 清理 session + Redis 路由。
- `internal/gateway/heartbeat.go`：Watchdog——每 10s 扫描所有连接，`now - last_read_at > 90s` 则发送 `DISCONNECT` 帧并清理。
- `internal/gateway/router.go`：Redis 路由注册/注销函数——`Register(user_id, device_id, node_id, conn_id)` 写入 `gateway:user:{uid}:{did}` 和 `gateway:node:{nid}:connections`；`Unregister(user_id, device_id)` 删除两条记录。
- WebSocket 使用 Protobuf binary frames（非 JSON text frames）。
- `make proto` 命令使用 `buf generate` 生成 Go 代码。

## Acceptance criteria

- [ ] WebSocket 连接 `/ws` → 发送 HANDSHAKE_REQ（含 JWT）→ 收到 HANDSHAKE_RESP（`success=true`，含 `session_id`、`heartbeat_interval_s=30`）
- [ ] HANDSHAKE_REQ 含无效 JWT → 收到 ERROR frame（`should_reconnect=false`），连接随后关闭
- [ ] 握手成功后，Redis 中存在 key `gateway:user:{uid}:{did}`，值为 `{node_id}:{conn_id}`
- [ ] 客户端每 30s 发送 HEARTBEAT → 服务端回复 HEARTBEAT_ACK 并续期 Redis TTL
- [ ] 客户端 90s 不发送任何帧 → 服务端发送 DISCONNECT（code=timeout）→ Redis 中路由记录被清理
- [ ] 客户端发送 DISCONNECT → 服务端清理 session + Redis 路由
- [ ] 同一 user_id + device_id 的旧连接被新连接替换时，旧连接被踢出（ERROR frame `should_reconnect=true`），Redis 路由指向新连接
- [ ] `buf lint` 对 proto 文件无错误

## Blocked by

- [0002 - 项目脚手架 + DB 迁移 + Mock Auth](0002-scaffold-migrations-auth.md)
