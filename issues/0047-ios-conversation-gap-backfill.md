---
id: "0047"
title: "iOS 会话 seq 缺口探测 + 历史补拉"
status: complete
labels: ["p0"]
parent: "0043"
blocked_by: ["0044"]
created_at: 2026-07-27
---

# 0047 — 会话缺口补拉

## Parent

[0043](0043-ios-high-load-leftover.md)

## What to build

- 检测本地相邻 `conversation_seq` 缺口，或 `local_max_seq < latest_seq`（Spec 06 §4.4）
- 调用消息补拉 API，幂等写入 GRDB
- 与全局 sync cursor **分工清晰**（不混用一个序号）

## Acceptance criteria

- [x] 人为制造本地缺口后，补拉可填平且 UI 顺序正确
- [x] 与 WS/sync 入站仍 `server_message_id` 幂等
- [x] study-guide / high-load 文档标注缺口项已覆盖

## 实现备注

- `ConversationMessagesAPI` + `ConversationGapBackfill`
- 打开会话 / sync 后对活跃会话触发；不推进 sync cursor
- 落库复用 `IncomingMessageApplier`（server_message_id 幂等）

## Backend

**不阻塞。** 补拉 API 与 `latest_seq` 已就绪。

## Blocked by

- [0044](0044-ios-chat-list-seq-window.md)
