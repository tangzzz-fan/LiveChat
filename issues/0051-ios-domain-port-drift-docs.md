---
id: "0051"
title: "iOS Domain 端口漂移：Spec/文档对齐（粗 Repository 降级）"
status: complete
labels: ["done", "p1"]
blocked_by: []
created_at: 2026-07-27
---

# 0051 — Domain 端口漂移：文档对齐

## Context

工程问题 [19](../docs/engineering-problems/19-domain-repository-ports-vs-concrete-executors.md)：Spec 13 §6 粗粒度 `*Repository` 与主路径 Executor（`MessageSendExecutor` / `SyncExecutor` / `RealtimeSession`）漂移。本票为三阶段修正的**阶段 1**，只改文档与注释，零运行时行为变化。

后续：[0052](0052-ios-fine-grained-ports-executor.md)（细粒度 Port）→ [0053](0053-ios-appservices-port-injection-fakes.md)（AppServices + Fake）。

## What to build

- 改写 `Specs/13-iOS客户端架构设计.md` §6：声明粗 `MessageRepository` / `ConversationRepository` / `SyncRepository` / `WebSocketRepository` / `PushRepository` 为历史脚手架；正式演进为细粒度 Store/Remote；保留已验证的 `AuthRepository`、`WebSocketTransport`
- 更新 `docs/engineering-problems/19-domain-repository-ports-vs-concrete-executors.md`：现状 → 三阶段路线，指向 0051–0053
- 更新 `docs/ios-client-study-guide.md` §7 对照表，与上述表述一致
- 更新 `ios/Packages/ChatDomain/.../RepositoryProtocols.swift` 文件头注释：「阶段 1 已文档降级；阶段 2 将拆细」

## Acceptance criteria

- [x] Spec §6 / 工程问题 19 / study-guide §7 / `RepositoryProtocols.swift` 文件头四者表述一致
- [x] 明确禁止空壳 `MessageRepositoryLive` 凑 conform
- [x] 无代码依赖图变更（`AppServices` / Executor 构造不变）

## 实现备注

- 不写空壳 Adapter；不删粗协议源码（留给 0052）
- `MediaRepository` 继续注明留给 [0049](0049-ios-image-message.md)
- 现状测试真相：`LocalDatabase.inMemory()` + Executor 测试钩子，非 `any MessageRepository`

## Blocked by

无

## Related

- [19-domain-repository-ports-vs-concrete-executors.md](../docs/engineering-problems/19-domain-repository-ports-vs-concrete-executors.md)
- [0049](0049-ios-image-message.md)（Media 仍独立）
