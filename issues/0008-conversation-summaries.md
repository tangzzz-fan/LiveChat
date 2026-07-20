---
id: "0008"
title: "会话摘要投影 + 会话列表 API"
status: complete
labels: ["done"]
parent: "0001"
blocked_by: ["0005"]
created_at: 2026-07-20
---

## Parent

[0001 - 阶段一：消息正确性骨架](0001-phase-1-message-correctness-skeleton.md)

## What to build

实现 ConversationSummary 投影维护和会话列表查询 API。每次消息写入时更新 `conversation_summaries` 表，让客户端可以高效拉取"会话列表页面"所需数据（最后一条消息预览、未读数、排序）。

端到端行为：User A 给 User B 发消息 → 消息写入 messages + outbox_events 后 → `conversation_summaries` 中 User B 的 `unread_count += 1`，`last_message_preview` 更新为新消息内容 → User B 调用 `GET /v1/conversations` → 返回按 `last_message_at DESC` 排序的会话列表（含预览和未读数）。

具体交付：

- `internal/conversations/summary.go`：`UpdateOnNewMessage(event)` —— 在 Outbox Consumer 的 `message_created` handler 中调用（或直接在 messages.Send 的同一事务中同步更新）→ `INSERT INTO conversation_summaries ... ON CONFLICT (user_id, conversation_id) DO UPDATE SET last_message_preview=..., last_message_at=..., unread_count = unread_count + 1, updated_at=NOW()`。
- 更新策略：对每个 conversation member（除发送者），unread_count + 1。发送者自己的 summary 只更新 `last_message_preview` 和 `last_message_at`，unread_count 不增加。
- `GET /v1/conversations` handler：从 JWT 取 user_id → `SELECT * FROM conversation_summaries WHERE user_id=$1 AND (is_hidden IS FALSE OR is_hidden IS NULL) ORDER BY is_pinned DESC, last_message_at DESC LIMIT $2 OFFSET $3`。默认 limit=50，支持 `?limit=&offset=` 分页。
- 响应中每条会话含：`conversation_id`、`conversation_type`、`last_message_preview`、`last_message_at`、`unread_count`、`is_pinned`、成员列表（从 conversation_members 联查）。
- 确保不依赖外部服务——ConversationSummary 维护逻辑在消息发送路径中，不引入额外异步链路（避免与 Outbox Consumer 的投递逻辑耦合过深）。

## Acceptance criteria

- [ ] User A 发送消息后，User A 的 `GET /v1/conversations` 返回该会话，`unread_count=0`
- [ ] User A 发送消息后，User B 的 `GET /v1/conversations` 返回该会话，`unread_count=1`
- [ ] 连续 3 条消息 → User B 的 `unread_count=3`
- [ ] 会话列表按 `last_message_at DESC` 排序（最近有消息的排最前）
- [ ] 空用户（没有任何会话）→ `GET /v1/conversations` 返回空数组 `[]` 和 HTTP 200
- [ ] 分页：`?limit=1&offset=0` 只返回 1 条，`offset=1` 返回第 2 条
- [ ] 无 JWT → 401

## Current implementation status

- 已实现：`conversation_summaries` 投影维护、`GET /v1/conversations`、未读数更新、排序与分页基础逻辑；响应现在包含从 `conversation_members` 联查得到的成员列表。
- 已新增验证：`internal/conversations/summary_test.go` 已覆盖连续 3 条消息后的未读累计、成员列表返回、按 `last_message_at DESC` 排序、分页和空数组场景；`internal/api/router_integration_test.go` 已固定覆盖 `GET /v1/conversations` 无 JWT 返回 `401`。
- 结论：本票的投影行为、列表返回行为与鉴权边界均已有固定自动化回归记录，可以关闭。

## Blocked by

- [0005 - Outbox 消费者：事件拉取、重试、死信](0005-outbox-consumer.md)
