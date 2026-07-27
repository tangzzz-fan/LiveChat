# Domain 仓储协议与落地实现漂移

## 问题是什么

Spec 13 §6 在 `ChatDomain` 定义了一套 Repository Protocols，期望 `ChatInfrastructure` 实现它们；实际主链路却用了**拆开的具体类型**（`MessageAPI` / `SyncExecutor` / `RealtimeSession` …），多数协议**无人 `conform`**，容易误以为「还没做」。

## 典型场景

- 打开 `RepositoryProtocols.swift`：协议齐全，全局搜 `: MessageRepository` 却无结果。
- 写测试想 mock `MessageRepository`，发现 Production 路径根本不依赖该协议。
- 复习 Spec 「实现 ChatDomain 的 Repository Protocols」与代码对不上。

## 通用分析思路

1. **协议是否仍是端口？** 看 Application / Middleware 注入的是协议还是具体类型。
2. **一个协议是否混了多个职责？** 若 `getMessages`（本地）与 `sendMessage`（远程）绑在同一 Port，落地时往往会拆开。
3. **是否有更贴传输的替代抽象？** 例如 WS 用 `WebSocketTransport`（帧流）比 `WebSocketRepository`（业务帧流）更干净。

## 当前项目方案

| Domain 协议 | 状态 | 实际落地（2026-07） |
|-------------|------|---------------------|
| `AuthRepository` | ✅ 已实现 | `AuthRepositoryLive` |
| `MessageRepository` | ⚠️ 未 conform | 本地：`LocalDatabase`；远程发送：`MessageAPI` + `MessageSendExecutor` |
| `ConversationRepository` | ⚠️ 未 conform | `ConversationAPI` + `LocalDatabase` |
| `SyncRepository` | ⚠️ 未 conform | `SyncAPI` + `LocalDatabase`；编排：`SyncExecutor` / `ConversationGapBackfill` |
| `PushRepository` | ⚠️ 未 conform | `PushTokenAPI` + `SilentSyncWakeHandler`（签名也不完全对齐） |
| `WebSocketRepository` | ⚠️ 未 conform | Spec 更贴地的是 `WebSocketTransport` → `URLSessionWebSocketTransport`；会话编排在 `RealtimeSession` |
| `MediaRepository` | ❌ 未做 | 留给 [0049](../../issues/0049-ios-image-message.md) |

**DTO 仍在用**：`SendMessageRequest` / `SendMessageResponse` / `SyncEvent` / `SyncResponse` / `AuthTokens` 等被 Infrastructure 直接依赖——文件不只是空协议。

**为何拆开算合理**：发送要「本地优先 + 有界队列 + 429 退避」，不宜塞进单薄的 `MessageRepository.sendMessage`；实时通道要握手/心跳/批量落库，也不适合直接暴露 `messageStream: AsyncStream<WebSocketFrame>` 给 UI。

**刻意不做的事（本轮）**：强行写一层空壳 `MessageRepositoryLive: MessageRepository` 只为凑 conform——会误导调用方以为那是主路径。后续若要可测替换，再引入**细粒度** Port（如 `MessageStore` / `MessageRemote`），并由 Executor 组合。

## 替代方案及取舍

| 方案 | 好处 | 代价 |
|------|------|------|
| A. 空壳 Adapter 包装现有类型 | 立刻满足 Spec 字面要求 | 假依赖；测试仍测不到真路径 |
| B. 改 Spec/协议为细粒度 Port（推荐演进） | 与 Executor 对齐 | 要改 Domain + 调用方 |
| C. 删除未用协议，只留 DTO | 减少幻觉 | 与 Spec §6 清单短期不一致 |

当前：**C 的文档化 + 协议文件内标注**；代码保留协议作演进锚点（B）。

## 踩坑记录

- 用 IDE「Find Usages」只搜协议名会漏掉真实能力；应同时搜 `MessageSendExecutor` / `SyncExecutor` / `RealtimeSession`。
- `AppServices` 注入的是具体类型，不是 `any MessageRepository`——这是现状真相源。

## 相关

- Spec 13 §6 · [ios-client-study-guide.md](../ios-client-study-guide.md) §7 · `ios/Packages/ChatDomain/.../RepositoryProtocols.swift`
