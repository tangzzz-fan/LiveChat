---
id: "0053"
title: "iOS AppServices 协议注入 + Fake 可测主路径"
status: open
labels: ["ready-for-agent", "p1"]
blocked_by: ["0052"]
created_at: 2026-07-27
---

# 0053 — AppServices 协议注入 + Fake 可测主路径

## Parent / blocked by

- [0052](0052-ios-fine-grained-ports-executor.md)（细粒度 Port 与 Executor 重构必须先完成）

## Context

工程问题 19 阶段 3：组合根以 Port 组装；测试可用 Fake 跑通「enqueue → remote 成败 → 状态收敛」，无需真 HTTP。完成后关闭工程问题 19。

## What to build

- `AppServices`：对外暴露/注入改为细粒度 Port，或继续持有 Executor，但其依赖在 `make` 内以 `any Port` 组装；组合根是唯一 Live 装配点
- 测试目标增加 `FakeMessageStore` / `FakeMessageRemote` / `FakeSyncRemote`（Test support 或 Infrastructure test target）
- `AppServicesTests`（或新建）用 Fake 组装一条「enqueue → remote 失败/成功 → 状态收敛」路径，无需真 HTTP
- 更新 `docs/engineering-problems/19-domain-repository-ports-vs-concrete-executors.md` 为 **resolved**，并更新 `docs/engineering-problems/INDEX.md`

## Acceptance criteria

- [ ] IDE Find Usages 以 Port / Executor 为准（不再依赖粗 `MessageRepository` 幻觉）
- [ ] Fake 组装路径可跑通（无真 HTTP）
- [ ] 工程问题 19 标记 resolved；INDEX 同步

## 实现备注

- 不改 UI / Redux 业务语义
- 不强制 mock `RealtimeSession` 全链路（WS 仍以 `WebSocketTransport` 为缝）
- 与 [0052](0052-ios-fine-grained-ports-executor.md) 的 Fake 可复用，本票重点是组合根与 Application 层可测性

## Blocked by

- [0052](0052-ios-fine-grained-ports-executor.md)

## Related

- [0051](0051-ios-domain-port-drift-docs.md)
- [19-domain-repository-ports-vs-concrete-executors.md](../docs/engineering-problems/19-domain-repository-ports-vs-concrete-executors.md)
- [`AppServices.swift`](../ios/Packages/ChatApplication/Sources/ChatApplication/AppServices.swift)
