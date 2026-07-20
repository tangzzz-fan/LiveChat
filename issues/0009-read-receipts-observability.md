---
id: "0009"
title: "已读回执 + 多端一致性收敛 + 可观测性"
status: in_progress
labels: ["in-progress"]
parent: "0001"
blocked_by: ["0006", "0007"]
created_at: 2026-07-20
---

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

## Current implementation status

- 已部分实现：`GET /metrics` 端点已存在，ConversationSummary 侧有 `MarkRead()`；Gateway 现在通过 `MessageAckService.ProcessAck` gRPC 将 `ACK(read)` 上送到 Message Service；Message Service 已能把 `ACK(read)` 写成 `read_receipt` outbox 事件；Outbox Consumer 已能消费 `read_receipt` 并写出 `message_read` / `conversation_updated` sync events；HTTP 入口已自动生成或透传 `trace_id`；`sync_events` 已支持按截止时间清理，并提供 `make cleanup-sync-events` 手动触发命令。
- 已验证：`TestGatewayForwardsReadAckToMessageService` 已改为覆盖 WebSocket ACK → gRPC `ProcessAck` 路径；`TestProcessReadAckCreatesOutboxAndProjectsReadState` 覆盖 `read_receipt` -> `unread_count=0` -> sync events 的最小业务闭环。
- 已新增验证：`internal/api/router_test.go` 覆盖 `trace_id` 生成与透传；`internal/sync/service_test.go` 覆盖 30 天清理接口只删除过期 `sync_events`。
- 未完成：仍缺真实进程级 runbook 证明 `WebSocket ACK -> Gateway -> gRPC Message Service -> Outbox Consumer -> sync_events` 整条链路；多端 `MAX(last_read_seq)` 收敛没有端到端验收；`trace_id` 还没有贯通到 gRPC / outbox / WebSocket 全链路，因此本票仍未关闭。
- 结论：本票已从“几乎纯占位”推进到“最小已读闭环已存在”，但还不能关闭，仍是 Phase 1 剩余的核心收口项之一。

## Blocked by

- [0006 - 实时投递（Fanout）](0006-fanout-realtime-delivery.md)
- [0007 - 离线同步：增量事件 API](0007-offline-sync-api.md)
