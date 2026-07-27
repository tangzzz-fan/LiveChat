# Domain 仓储协议与落地实现漂移

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

## 当前项目方案

### 现状（阶段 2：细粒度 Port 已落地）

| Domain 协议 | 角色 | 实际落地（2026-07） |
|-------------|------|---------------------|
| `AuthRepository` | ✅ **正式保留** | `AuthRepositoryLive` |
| `WebSocketTransport` | ✅ **正式保留**（Infra） | `URLSessionWebSocketTransport`；编排 `RealtimeSession` |
| `MessageStore` / `MessageRemote` | ✅ **正式** | `LocalDatabase` / `MessageAPI`；编排 `MessageSendExecutor` |
| `SyncCursorStore` / `SyncRemote` | ✅ **正式** | `LocalDatabase` / `SyncAPI`；编排 `SyncExecutor` |
| `ConversationStore` / `ConversationRemote` | ✅ **正式** | `LocalDatabase` / `ConversationAPI` |
| `MediaRepository` | ❌ 未做 | 留给 [0049](../../issues/0049-ios-image-message.md) |
| 粗 `*Repository` | 🗑️ **已删除**（0052） | 勿再引入空壳 Adapter |

**DTO 仍在用**：`SendMessageRequest` / `SendMessageResponse` / `SyncEvent` / `SyncResponse` / `AuthTokens` 等。

**为何拆开算合理**：发送要「本地优先 + 有界队列 + 429 退避」，不宜塞进单薄的粗 `MessageRepository.sendMessage`；实时通道要握手/心跳/批量落库，也不适合直接暴露粗 `WebSocketRepository.messageStream` 给 UI。

**明确禁止**：写空壳 `MessageRepositoryLive` 只为凑 conform。

**测试真相（0052）**：`FakeMessageRemote`（hang→sending 超时）+ `FakeSyncRemote`（游标推进）；见 `PortFakesTests`。`AppServices.make()` 仍注入 Live；组合根全面 Port 化留给 0053。

### 三阶段修正路线

| 阶段 | Issue | 做什么 | 运行时行为 |
|------|-------|--------|------------|
| **1** | [0051](../../issues/0051-ios-domain-port-drift-docs.md) | Spec §6 / 本文 / study-guide §7 / 协议文件头：粗 Repository **文档降级** | **零变化** |
| **2** | [0052](../../issues/0052-ios-fine-grained-ports-executor.md) | 细粒度 Port；Executor 依赖协议；粗协议删除 | 行为不变 |
| **3** | [0053](../../issues/0053-ios-appservices-port-injection-fakes.md) | `AppServices` 以 Port 组装；Fake 可测主路径；本文标记 resolved | 行为不变 |

当前进度：**阶段 2（0052）完成**；阶段 3 待做。

## 替代方案及取舍

| 方案 | 好处 | 代价 |
|------|------|------|
| A. 空壳 Adapter 包装现有类型 | 立刻满足旧 Spec 字面要求 | 假依赖；测试仍测不到真路径 — **禁止** |
| B. 改 Spec/协议为细粒度 Port（推荐演进） | 与 Executor 对齐 | 要改 Domain + 调用方 — **0052 已落地；0053 收尾** |
| C. 删除未用协议，只留 DTO | 减少幻觉 | 与旧 Spec §6 清单短期不一致 — **0051 文档化 → 0052 删除粗协议** |

当前：**B（0052 完成）→ 0053 组合根**。

## 踩坑记录

- 用 IDE「Find Usages」只搜粗协议名会漏掉真实能力；应搜 `MessageSendExecutor` / `SyncExecutor` / `RealtimeSession` / `MessageStore`。
- `AppServices` 对外仍暴露具体 `LocalDatabase` / `*API`；Executor **内部**已依赖 `any MessageStore` 等——这是 0052 真相源。0053 再把组合根对外也改成 Port。

## 相关

- Spec 13 §6 · [ios-client-study-guide.md](../ios-client-study-guide.md) §7 · `ios/Packages/ChatDomain/.../RepositoryProtocols.swift`
- Issues：[0051](../../issues/0051-ios-domain-port-drift-docs.md) → [0052](../../issues/0052-ios-fine-grained-ports-executor.md) → [0053](../../issues/0053-ios-appservices-port-injection-fakes.md)
- 执行总结：[19-execution-summary-0051-0052.md](19-execution-summary-0051-0052.md)
