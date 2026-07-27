---
id: "0061"
title: "iOS 会话列表按 last_message_at 排序（非 updated_at）"
status: complete
labels: ["done", "p0", "bug"]
blocked_by: []
created_at: 2026-07-27
---

# 0061 — 会话列表最新消息排序

## Notes

**现象**：会话列表不按最新发送/收到的消息置顶。

**根因**：
1. `ConversationAPI.listRemoteSummaries` 把服务端 `last_message_at` 丢成 `nil`
2. 本地投影 `ORDER BY updated_at DESC`；进会话 `clearUnread` 会 bump `updated_at`，与「最近消息」语义不符
3. 服务端已是 `ORDER BY is_pinned DESC, last_message_at DESC NULLS LAST`

## What to build

- [x] 解析 API `last_message_at`（ISO8601）
- [x] 本地 `conversationSummaries` / fetch 按 `is_pinned` + `last_message_at` 排序
- [x] `clearUnread` 不再改写 `updated_at`
- [x] 回归测试

## Acceptance criteria

- [x] 有更新的消息时间的会话排在更前面（即使另一会话刚被打开清未读）
- [x] 刷新远程列表后 `lastMessageAt` 非空且顺序与服务端一致

## Related

- `ConversationAPI` · `LocalDatabase+Messaging` · `PortConformances.clearUnread`
