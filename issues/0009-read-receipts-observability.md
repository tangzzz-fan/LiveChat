---
id: "0009"
title: "已读回执 + 多端一致性收敛 + 可观测性"
status: ready-for-agent
labels: ["ready-for-agent"]
parent: "0001"
blocked_by: ["0006", "0007"]
created_at: 2026-07-20

## Parent

[0001 - 阶段一：消息正确性骨架](0001-phase-1-message-correctness-skeleton.md)

## What to build

实现已读回执的发送与消费、多端已读状态的 MAX 收敛规则、以及全部 Prometheus 可观测性指标。

端到端行为：User B 打开与 A 的会话 → WebSocket 发送 `ACK` 帧（ack_type="read", event_seq=..., last_read_seq=N）→ Gateway 转发到 MessageService → 生成 `read_receipt` Outbox 事件 → Outbox Consumer 消费 → 更新 ConversationSummary 中 B 的 `unread_count=0` → 生成 `conversation_updated` sync_event 给 B 的其他设备 → User A 离线，通过 sync_events 收到 `read_receipt` 事件 → 各方 `last_read_seq = MAX(本地值, 事件值)`。

具体交付：

- ACK 帧处理：Gateway 收到 opcode 0x0005（ACK）→ gRPC 调用 MessageService `ProcessAck(user_id, device_id, ack_type, event_seq, acked_at_ms)` → MessageService 校验 `event_seq` 合法性 → 生成 Outbox 事件 `delivery_acked` 或 `read_receipt`。
- `read_receipt` 消费：更新 B 自己的 `conversation_summaries.unread_count=0` → 通知 A（发送方）消息被读了（sync_event `message_read`）→ 通知 B 的其他设备已读状态更新（sync_event `conversation_updated`）。
- 已读收敛规则：`last_read_seq = MAX(local, remote)`——在 `POST /v1/sync/events` 的 payload 中携带 `last_read_seq`，客户端应用时取 MAX。
- Prometheus metrics 端点：`GET /metrics`（所有服务暴露）。Message Service 指标：`http_requests_total{method,path,status}`、`http_request_duration_seconds{method,path,quantile}`、`messages_sent_total`、`outbox_events_created_total`。Outbox Consumer 指标：`outbox_pending_count`、`outbox_processing_count`、`outbox_failed_count`、`outbox_consumer_lag_seconds`。Gateway 指标：`ws_connections_active`、`ws_connections_total`、`ws_heartbeat_timeouts_total`。
- 结构化日志补充：所有 HTTP/gRPC handler 自动注入 `trace_id`（从 request header 取或生成 UUID），slog 输出 JSON 格式。
- Sync events TTL 清理：定时任务（每天凌晨）`DELETE FROM sync_events WHERE created_at < NOW() - INTERVAL '30 days'`。

## Acceptance criteria

- [ ] User B 通过 WebSocket 发送 ACK（ack_type="read"）→ Outbox 生成 `read_receipt` 事件 → 被消费
- [ ] 消费 `read_receipt` 后，B 的 `unread_count` 重置为 0
- [ ] B 的其他设备通过 sync_events 收到 `conversation_updated`（unread_count=0）
- [ ] A 通过 sync_events 收到 `message_read` 事件（last_read_seq 对应被读的消息）
- [ ] 设备 1 `last_read_seq=100`，设备 2 `last_read_seq=50` → sync_event 下发 seq=100 → 设备 2 应用 MAX(50,100)=100
- [ ] `GET /metrics` 返回 Prometheus 格式指标，包含上述所有指标名
- [ ] 每条 HTTP 请求日志含 `trace_id`
- [ ] Sync events 30 天清理任务可被手动触发（如通过 admin 端点或 make 命令）

## Blocked by

- [0006 - 实时投递（Fanout）](0006-fanout-realtime-delivery.md)
- [0007 - 离线同步：增量事件 API](0007-offline-sync-api.md)
