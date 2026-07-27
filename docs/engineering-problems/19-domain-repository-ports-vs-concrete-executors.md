# Domain 仓储协议与落地实现漂移

**状态：resolved（2026-07-27）** — 经 [0051](../../issues/0051-ios-domain-port-drift-docs.md) → [0052](../../issues/0052-ios-fine-grained-ports-executor.md) → [0053](../../issues/0053-ios-appservices-port-injection-fakes.md) 关闭。

## 问题是什么

Spec 13 §6 曾在 `ChatDomain` 定义一套粗粒度 Repository Protocols，期望 `ChatInfrastructure` 实现它们；实际主链路却用了**拆开的具体类型**（`MessageAPI` / `SyncExecutor` / `RealtimeSession` …），多数协议**无人 `conform`**，容易误以为「还没做」。

## 典型场景

- （历史）打开 `RepositoryProtocols.swift`：粗协议齐全，全局搜 `: MessageRepository` 却无结果。
- 写测试想 mock 粗 `MessageRepository`，发现 Production 路径根本不依赖该协议。
- 复习 Spec 「实现 ChatDomain 的 Repository Protocols」与代码对不上。

## 通用分析思路

1. **协议是否仍是端口？** 看 Application / Middleware 注入的是协议还是具体类型。
2. **一个协议是否混了多个职责？** 若 `getMessages`（本地）与 `sendMessage`（远程）绑在同一 Port，落地时往往会拆开。
3. **是否有更贴传输的替代抽象？** 例如 WS 用 `WebSocketTransport`（帧流）比 `WebSocketRepository`（业务帧流）更干净。

## 当前项目方案（已落地）

| Domain 协议 | 角色 | 实际落地 |
|-------------|------|----------|
| `AuthRepository` | ✅ 正式保留 | `AuthRepositoryLive` |
| `WebSocketTransport` | ✅ 正式保留（Infra） | `URLSessionWebSocketTransport`；编排 `RealtimeSession` |
| `MessageStore` / `MessageRemote` | ✅ | `LocalDatabase` / `MessageAPI`；编排 `MessageSendExecutor` |
| `SyncCursorStore` / `SyncRemote` | ✅ | `LocalDatabase` / `SyncAPI`；编排 `SyncExecutor` |
| `ConversationStore` / `ConversationRemote` | ✅ | `LocalDatabase` / `ConversationAPI` |
| `MediaRepository` | ❌ 未做 | 留给 [0049](../../issues/0049-ios-image-message.md) |
| 粗 `*Repository` | 🗑️ 已删除 | 勿再引入空壳 Adapter |

**组合根（0053）**：`AppServices.make()` 以 `any Port` 装配 Executor；对外暴露 `messageStore` / `messageRemote` / …。测试用 `AppServices.assembleSendExecutor` + `FakeMessageStore` / `FakeMessageRemote`（见 `PortFakes.swift`、`AppServicesTests`）。

**明确禁止**：写空壳 `MessageRepositoryLive` 只为凑 conform。

### 三阶段路线（已完成）

| 阶段 | Issue | 结果 |
|------|-------|------|
| 1 | [0051](../../issues/0051-ios-domain-port-drift-docs.md) | 粗 Repository 文档降级 |
| 2 | [0052](../../issues/0052-ios-fine-grained-ports-executor.md) | 细粒度 Port + Executor 依赖协议；粗协议删除 |
| 3 | [0053](../../issues/0053-ios-appservices-port-injection-fakes.md) | AppServices Port 组装 + Fake 可测主路径；本文 resolved |

## 替代方案及取舍

| 方案 | 好处 | 代价 | 结局 |
|------|------|------|------|
| A. 空壳 Adapter | 立刻满足旧 Spec 字面 | 假依赖 | **禁止** |
| B. 细粒度 Port | 与 Executor 对齐 | 改 Domain + 调用方 | **已采用** |
| C. 只删协议留 DTO | 减幻觉 | 短期与 Spec 不一致 | 经 0051 过渡后由 0052 完成 |

## 踩坑记录

- IDE「Find Usages」应搜 `MessageStore` / `MessageSendExecutor` / `SyncExecutor` / `RealtimeSession`，不要搜已删除的粗 `MessageRepository`。
- 投影分页等表面仍挂在 `LocalDatabase`；发送 / sync 主路径以 Port 为准。

## 相关

- Spec 13 §6 · [ios-client-study-guide.md](../ios-client-study-guide.md) §7 · `RepositoryProtocols.swift`
- Issues：0051 → 0052 → 0053
- 执行总结：[19-execution-summary-0051-0053.md](19-execution-summary-0051-0053.md)
