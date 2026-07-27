---
id: "0054"
title: "iOS 已读回执：进会话清未读 + WS ACK(read) + 气泡状态"
status: complete
labels: ["done", "p0"]
blocked_by: ["0041", "0044"]
created_at: 2026-07-27
---

# 0054 — 已读回执与未读收敛

## Context

服务端已读闭环已在 [0009](0009-read-receipts-observability.md) 完成（WS `ACK(read)` → outbox → `message_read` / `conversation_updated`）。iOS 补齐客户端一侧。

## What to build

- 打开会话：本地 `unread_count=0` + 若已连 WS 则发 `ACK(ack_type=read, last_read_seq=max(seq))`
- 接收方增量：`IncomingMessageApplier` 对他人消息正确累加未读
- Sync：处理 `message_read` / `conversation_updated`
- UI：己方气泡状态标记
- 对齐 `docs/ios-client-study-guide.md` 0049/0050/Media 漂移

## Acceptance criteria

- [x] 进会话后列表未读角标清零（本地）
- [x] 前台已连接时可发出 WS `ACK(read)`（有 max seq 时；水位去重）
- [x] sync `message_read` 可将己方消息推进到 `read`
- [x] 气泡对己方消息展示状态
- [x] study-guide 与 0049/0050/Media 现状一致

## Related

- Spec 05 ACK · Spec 13 §5.1 · `RealtimeSession.markConversationRead` · `SyncExecutor`
