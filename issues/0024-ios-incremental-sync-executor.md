---
id: "0024"
title: "iOS 增量同步：SyncExecutor + 多端补拉"
status: open
labels: ["ready-for-agent", "p1"]
parent: "0021"
blocked_by: ["0023"]
created_at: 2026-07-21
---

# 0024 — iOS 增量同步：SyncExecutor + 多端补拉

## Parent

[0021 - iOS 客户端架构骨架](0021-ios-client-architecture-skeleton.md)

## What to build

实现 Spec 13 SyncExecutor：按本地 `sync_cursors` 调用 `GET /v1/sync/events`，将事件映射为领域事件并写入 GRDB，支持 `has_more` 连续拉取。端到端：设备 B 断开或杀进程期间 A 发消息，B 回前台后自动增量同步，本地消息/会话列表出现 A 的消息。

## Acceptance criteria

- [ ] `SyncRepository` 真实现：读/写本地 cursor + 拉取远端 events
- [ ] 启动与回前台触发增量同步（对齐 Spec 13 §8）
- [ ] `RemoteEventProcessor`（或等价）统一应用 sync 事件到本地 DB
- [ ] `has_more=true` 时继续拉取直至赶完或达安全上限
- [ ] 双设备演示：B 离线期间消息在 B 上线后可见
- [ ] 同步失败可重试，不损坏 cursor 单调性（失败不盲目推进）

## Blocked by

- [0023 - iOS 本地优先发消息：GRDB + HTTP send + 建群拿会话](0023-ios-local-first-send-grdb-http.md)

## 技术难点与注意事项

- Cursor 按 **device** 在服务端维护；换机/重装新 device_id 从 0 或策略性全量补拉需文档说明。
- 与后续 WS 投递去重：同一 `server_message_id` insert-if-needed。
