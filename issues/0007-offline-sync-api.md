---
id: "0007"
title: "离线同步：增量事件 API + 游标管理 + 序号缺口检测"
status: in_progress
labels: ["in-progress"]
parent: "0001"
blocked_by: ["0005"]
created_at: 2026-07-20
---

## Parent

[0001 - 阶段一：消息正确性骨架](0001-phase-1-message-correctness-skeleton.md)

## What to build

实现增量同步 API 和客户端离线恢复所需的服务端支持：`GET /v1/sync/events` 按游标分页返回事件、SyncCursor 管理、客户端断线后的缺口恢复流程。

端到端行为：设备离线期间有新消息产生（已写入 sync_events）→ 设备重连 WebSocket，握手时带上 `last_event_seq` → 握手响应告知 `latest_event_seq` → 客户端发现 `local_cursor < latest_event_seq` → 调用 `GET /v1/sync/events?cursor=N&limit=100` 逐页拉取 → 拉完后本地 cursor 与 latest_event_seq 对齐 → 对每个活跃会话检查 `conversation_seq` 是否连续 → 若有缺口调用会话消息补拉。

具体交付：

- `internal/sync/service.go`：`AppendEvent(user_id, event_type, payload)` —— 写入 `sync_events` 表。`GetEvents(user_id, cursor, limit)` —— 查询 `WHERE user_id=$1 AND event_seq>$2 ORDER BY event_seq LIMIT $3`。
- `internal/sync/cursor.go`：`UpdateCursor(user_id, device_id, last_event_seq)` —— UPSERT sync_cursors；`GetCursor(user_id, device_id)` —— 取当前游标。
- `GET /v1/sync/events` handler：从 JWT 取 user_id/device_id，读取 cursor 参数（0 表示首次同步），调用 `GetEvents`，返回 `{events, has_more, latest_event_seq}`。latest_event_seq 从全局序列取（`SELECT MAX(event_seq) FROM sync_events WHERE user_id=$1`）。
- `handshake_response.latest_event_seq` 正确填充（Gateway 握手时查 sync_cursors 和全局最新 event_seq）。
- sync_events 写入时机：Outbox Consumer 的 `message_created` handler → Fanout 对不在线设备 → `sync.AppendEvent()`。
- 不做 sync_events TTL 清理和游标过期降级（那是运维面的工作，归属 Slice 8 的可观测性范畴）。

## Acceptance criteria

- [ ] 设备离线期间产生了 3 条 sync_events → `GET /v1/sync/events?cursor=0` 返回这 3 条事件，每条含 `event_seq`、`event_type`、`payload`
- [ ] `has_more=false` 且 `latest_event_seq` = 全局最新序号
- [ ] `GET /v1/sync/events?cursor=N` 只返回 `event_seq > N` 的事件
- [ ] 分页：总共 150 条事件，limit=100 → 第一页 `has_more=true`，第二页 `has_more=false`
- [ ] 手shake响应中 `latest_event_seq` 大于本地 cursor 时，系统识别到有离线消息待同步
- [ ] sync_cursors 表在每次同步完成后更新 `last_event_seq`
- [ ] 单端多次离线→上线→同步，cursor 持续正确前进

## Current implementation status

- 已实现：`sync_events` 写入、`GET /v1/sync/events`、`sync_cursors` 更新、基于 `cursor` 的增量查询、离线补拉基础路径，以及 Gateway 握手响应中的 `latest_event_seq` 联动。
- 已验证：`./scripts/phase1-smoke.sh` 已确认接收方可以通过同步 API 看到 `message_created` 事件。
- 已新增验证：Gateway 握手测试现在会断言 `HandshakeResponse.latest_event_seq` 来自同步事件提供者；`internal/sync/service_test.go` 已覆盖分页读取、`latest_event_seq` 返回、`cursor` 只前进不回退；`internal/api/router_integration_test.go` 已固定覆盖 `GET /v1/sync/events` 在 `cursor=0` 时返回事件 + `latest_event_seq`、分页 `has_more`、以及 handler 对 `sync_cursors` 的回写和单调前进。
- 未完成：序号缺口恢复仍未形成专门实现与固定验收，因此本票仍处于进行中。

## Blocked by

- [0005 - Outbox 消费者：事件拉取、重试、死信](0005-outbox-consumer.md)
