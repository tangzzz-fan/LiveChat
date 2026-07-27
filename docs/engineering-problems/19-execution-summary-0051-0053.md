# 执行总结：Domain 端口漂移修正（0051–0053）

> 对应工程问题 [19](19-domain-repository-ports-vs-concrete-executors.md)（**resolved**）  
> 日期：2026-07-27

## 1. 要解决什么

Spec 13 §6 粗粒度 `*Repository` 与主路径 Executor 漂移：协议像「待实现清单」，实际能力在具体类型上，难以用 Fake 测主路径。

## 2. 做了什么

### 0051 — 文档降级

粗 Repository 标为历史脚手架；保留 `AuthRepository` / `WebSocketTransport`；禁止空壳 Adapter。零运行时变化。

### 0052 — 细粒度 Port

引入 `MessageStore` / `MessageRemote`、`SyncCursorStore` / `SyncRemote`、`ConversationStore` / `ConversationRemote`；删除粗协议；Executor 依赖 `any` Port；`PortConformances` + `PortFakesTests`。

### 0053 — 组合根 + Fake 主路径

| 交付 | 说明 |
|------|------|
| `AppServices` | 暴露 `messageStore` / `messageRemote` 等 Port；`make()` 以 `any Port` 装配 |
| `assembleSendExecutor` / `assembleSyncExecutor` | 唯一装配缝（Live 与 Fake 共用） |
| `PortFakes.swift` | `FakeMessageStore` / `FakeMessageRemote` / `FakeSyncRemote` 等 |
| `AppServicesTests` | Fake 组装：enqueue → success→accepted / fail→failed（无 HTTP） |
| 工程问题 19 | 标记 **resolved** |

## 3. 验收

| 票 | 结果 |
|----|------|
| 0051 | ✅ |
| 0052 | ✅（`swift test` PortFakes） |
| 0053 | ✅（`swift test` ChatApplication） |

## 4. 结论

端口漂移已闭环。投影分页仍可挂在 `LocalDatabase`；发送 / sync 以 Port + Executor 为准。Media 留给 0049。
