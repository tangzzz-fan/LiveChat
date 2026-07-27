---
id: "0052"
title: "iOS 细粒度 Port：Store/Remote + Executor 依赖协议"
status: complete
labels: ["done", "p1"]
blocked_by: ["0051"]
created_at: 2026-07-27
---

# 0052 — 细粒度 Port + Executor 依赖协议

## Parent / blocked by

- [0051](0051-ios-domain-port-drift-docs.md)（文档对齐必须先完成）

## Context

工程问题 19 阶段 2：把粗 `*Repository` 换成细粒度 Store/Remote，让 Executor 依赖协议而非 concrete `LocalDatabase` / `MessageAPI`。**禁止**空壳 `MessageRepositoryLive`。

## What to build

在 `ChatDomain` 引入（命名可微调，职责锁定）：

- `MessageStore`：本地 insert / status / fetchPendingSend 等发送队列所需表面
- `MessageRemote`：`sendMessage(_:) async throws -> SendMessageResponse`
- `SyncCursorStore` + `SyncRemote`：游标读写 + `fetchEvents`
- `ConversationStore` / `ConversationRemote`：仅覆盖现有 `LocalDatabase` + `ConversationAPI` 已用表面

落地：

- `LocalDatabase` / `MessageAPI` / `SyncAPI` / `ConversationAPI` 等 **extension conform**（或极薄 adapter）
- `MessageSendExecutor` / `SyncExecutor` 构造改为依赖上述协议（`AppServices.make()` 仍注入 Live）
- **删除或 `@available(*, unavailable)`** 粗 `MessageRepository` / `SyncRepository` / `ConversationRepository` / `WebSocketRepository` / `PushRepository`
- DTO（`SendMessageRequest` 等）继续留在 Domain
- `MediaRepository` 留给 [0049](0049-ios-image-message.md)，可保留并注明未实现
- 不把 `RealtimeSession` 塞进粗 `WebSocketRepository`（继续 `WebSocketTransport`）

测试（证明 Port 可测）：

- ≥1 个 `FakeMessageRemote` 发送用例（429 退避或 hang→sending 超时）
- ≥1 个 `FakeSyncRemote` 游标推进用例

## Acceptance criteria

- [x] 生产路径行为不变（`AppServices.make()` 仍可组装）
- [x] Executor 源码不再直接写死 `LocalDatabase` / `MessageAPI` 类型名作为依赖字段类型（改为协议）
- [x] 粗协议不再作为 Spec §6「待实现清单」
- [x] Fake remote 发送测 + Fake remote sync 测通过

## 实现备注

- 不要把整个 `LocalDatabase` 打成单一巨 Port
- 参考已有细粒度范例：`WebSocketTransport`
- 现状测试基线：`LocalDatabaseTests.orphanSendingIsReclaimedToQueued`（真 API 但不打网）

## Blocked by

- [0051](0051-ios-domain-port-drift-docs.md)

## Related

- [0053](0053-ios-appservices-port-injection-fakes.md)（下一步：组合根接线）
- [19-domain-repository-ports-vs-concrete-executors.md](../docs/engineering-problems/19-domain-repository-ports-vs-concrete-executors.md)
