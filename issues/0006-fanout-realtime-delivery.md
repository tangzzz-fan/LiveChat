---
id: "0006"
title: "实时投递（Fanout）：Outbox → Gateway → WebSocket 推送"
status: complete
labels: ["done"]
parent: "0001"
blocked_by: ["0004", "0005"]
created_at: 2026-07-20
---

## Parent

[0001 - 阶段一：消息正确性骨架](0001-phase-1-message-correctness-skeleton.md)

## What to build

将 Outbox Consumer 的 `message_created` handler 替换为真实投递逻辑：查目标设备的在线路由 → 构造 MESSAGE_DELIVERY 帧 → 通过 gRPC 转发到对应 Gateway 节点 → Gateway 写入 WebSocket 连接。同时将 `GET /v1/conversations/{cid}/messages?from_seq=N` 补拉端点实现（会话消息查询）。

端到端行为：User A 发送消息 → Outbox Consumer 消费 `message_created` 事件 → FanoutService 查出 conversation 的所有成员 → 对每个成员查 Redis 路由（哪些设备在线） → 对每个在线设备，向其所在 Gateway 发 gRPC `DeliverMessage` → Gateway 通过 WebSocket 发送 `MESSAGE_DELIVERY` 帧（opcode 0x1001）→ 对不在线的设备，写入 `sync_events` 表。

具体交付：

- `internal/fanout/service.go`：`Fanout(event)` 函数——根据 conversation_id 查 `conversation_members` 获取所有成员 user_id → 对每个非发送者，查 Redis `gateway:user:{uid}:*` 获取在线设备 → 对每个在线设备，查 `gateway:user:{uid}:{did}` 获取 gateway node → gRPC `DeliverMessage`。
- `internal/fanout/sync_backfill.go`：不在线的设备 → 调用 `sync.AppendEvent(user_id, event)` 写入 `sync_events`。
- `proto/message.proto`：定义 `FanoutService` gRPC service，含 `DeliverMessage(DeliverMessageRequest) returns (DeliverMessageResponse)`。
- Gateway 侧实现 gRPC server：接收 `DeliverMessage` 请求 → 根据 `device_id` 查找本地 WebSocket 连接 → 构造 `MESSAGE_DELIVERY` 帧（opcode 0x1001，payload = WsMessageDelivery protobuf）→ 写入 WebSocket。
- Gateway 侧：若设备不在本地节点（连接已迁移），返回 gRPC error code `NotFound`，Fanout 端捕获并回退到写 sync_events。
- `GET /v1/conversations/{cid}/messages?from_seq={seq}&limit={n}`：查询 messages 表，按 `conversation_seq ASC` 排序游标分页。若 `from_seq=0` 则从第一条开始。默认 limit=50。

## Acceptance criteria

- [ ] User A 发送消息后，User B 在同一 Gateway 节点的 WebSocket 连接上收到 `MESSAGE_DELIVERY` 帧（opcode 0x1001），payload 含 `server_message_id`、`conversation_id`、`conversation_seq`、`sender_user_id`、`message_type`、`content`
- [ ] User B 完全离线（无 WebSocket 连接）时，消息成功写入 `sync_events` 表，`event_type='message_created'`
- [ ] User B 部分在线（设备 1 离线、设备 2 在线）→ 设备 2 实时收到，设备 1 在 sync_events 中有事件
- [ ] 发送方自己不在投递目标列表中（不会给自己推送 MESSAGE_DELIVERY）
- [ ] `GET /v1/conversations/{cid}/messages?from_seq=1&limit=10` 按 conversation_seq ASC 返回最多 10 条消息
- [ ] 会话不存在 → 404；无权限访问 → 403

## Current implementation status

- 已实现：`internal/fanout/service.go` 已有 conversation member 查询、在线设备扫描、同步事件写入；`GET /v1/conversations/{cid}/messages` 已可工作并被 smoke 验证。
- 已实现：Gateway 已暴露 `GatewayDeliveryService.DeliverMessage` gRPC 服务，`outbox-consumer` 已改为通过 gRPC client 向目标 Gateway 节点投递 `WsMessageDelivery` protobuf；`DeliverToDevice()` 负责最终 WebSocket 下发。
- 已验证：`go test ./internal/gateway -run TestGatewayDeliversPublishedMessageToConnectedDevice -count=1` 覆盖 gRPC `DeliverMessage` → WebSocket `MESSAGE_DELIVERY` 路径；`./scripts/phase1-realtime-delivery.sh` 已固定验证 `Outbox -> Fanout -> gRPC Gateway -> WebSocket` 进程级链路、离线 `sync_events` 回退和 trace 透传。
- 结论：本票的实现与验收已经闭环，可视为完成态。

## Blocked by

- [0004 - Gateway：WebSocket 握手 + 心跳 + 用户路由注册](0004-gateway-websocket-handshake.md)
- [0005 - Outbox 消费者：事件拉取、重试、死信](0005-outbox-consumer.md)
