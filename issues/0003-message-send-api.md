---
id: "0003"
title: "消息发送 API + 幂等写入 + Outbox 事件"
status: ready-for-agent
labels: ["ready-for-agent"]
parent: "0001"
blocked_by: ["0002"]
created_at: 2026-07-20
---

## Parent

[0001 - 阶段一：消息正确性骨架](0001-phase-1-message-correctness-skeleton.md)

## What to build

实现消息发送 HTTP API 的服务端写入路径：鉴权 → 校验 → 分配 conversation_seq → 在同一数据库事务中写入 messages 表和 outbox_events 表。

端到端行为：发送方调用 `POST /v1/messages/send` → 服务端校验发送者属于该会话 → 分配单调递增的 `conversation_seq` → `BEGIN; INSERT INTO messages ...; INSERT INTO outbox_events ...; COMMIT;` → 返回 `{server_message_id, conversation_seq, is_duplicate, server_received_at_ms}`。若 `client_message_id` 已存在则幂等返回，不重复写入。

具体交付：

- `internal/domain/` 定义共享类型：`Message`（含 `server_message_id`、`conversation_id`、`conversation_seq`、`sender_user_id`、`sender_device_id`、`client_message_id`、`message_type`、`content`、`server_received_at`），`OutboxEvent`（含 `aggregate_type`、`aggregate_id`、`event_type`、`payload`、`status`）。
- `internal/messages/` 实现 `Send()` 函数：校验 conversation membership（从 `conversation_members` 表查），调用 `nextval('conversation_seq_' || cid)` 分配序号，在同一事务中执行 INSERT messages + INSERT outbox_events，利用 `ON CONFLICT (sender_user_id, client_message_id) DO NOTHING` 实现幂等。
- Conversation seq 使用 PostgreSQL sequence（每会话一个 sequence，动态创建），确保单会话内严格递增。
- `POST /v1/messages/send` handler：从 JWT 提取 `user_id`/`device_id`，校验 request body（`client_message_id`、`conversation_id`、`message_type` 必填），调用 `messages.Send()`，返回结果。
- 幂等命中时返回 HTTP 200 + `is_duplicate: true` + 原始消息的 `server_message_id` 和 `conversation_seq`。
- 校验失败（非会话成员、会话不存在等）返回 4xx + 明确错误信息。

## Acceptance criteria

- [ ] `POST /v1/messages/send` 成功写入一条消息，messages 表和 outbox_events 表各有一行
- [ ] outbox_events 行：`aggregate_type='message'`, `event_type='message_created'`, `status='pending'`, payload 含 `server_message_id` 和 `conversation_seq`
- [ ] 发送后 `conversation_seq` 严格递增（同一会话连续发送 3 条 → seq 分别为 1, 2, 3）
- [ ] 相同 `(sender_user_id, client_message_id)` 发送两次 → 第二次返回 HTTP 200, `is_duplicate: true`，DB 中只有 1 条消息记录
- [ ] 同一 `client_message_id` 由不同 user 发送 → 各自独立写入不冲突
- [ ] 发送者不是 conversation member → 返回 403
- [ ] 缺少必填字段（`client_message_id`、`conversation_id`） → 返回 400
- [ ] 无 JWT 访问 → 返回 401

## Blocked by

- [0002 - 项目脚手架 + DB 迁移 + Mock Auth](0002-scaffold-migrations-auth.md)
