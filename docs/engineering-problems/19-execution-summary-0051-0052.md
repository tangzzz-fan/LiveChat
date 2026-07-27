# 执行总结：Domain 端口漂移修正（0051–0052）

> 对应工程问题 [19](19-domain-repository-ports-vs-concrete-executors.md)  
> 日期：2026-07-27  
> 范围：阶段 1 文档对齐 + 阶段 2 细粒度 Port；阶段 3（[0053](../../issues/0053-ios-appservices-port-injection-fakes.md)）未纳入本提交

## 1. 要解决什么

Spec 13 §6 粗粒度 `*Repository` 与主路径 Executor（`MessageSendExecutor` / `SyncExecutor` / `RealtimeSession`）长期漂移：协议像「待实现清单」，实际能力在具体类型上，易产生「还没做」幻觉，也难以用 Fake 测主路径。

## 2. 做了什么

### 阶段 1 — [0051](../../issues/0051-ios-domain-port-drift-docs.md)（文档降级）

| 交付 | 说明 |
|------|------|
| Spec 13 §6 | 粗 `MessageRepository` 等标为历史脚手架；正式保留 `AuthRepository` / `WebSocketTransport`；**禁止**空壳 Adapter |
| 工程问题 19 | 现状表 + 三阶段路线（0051→0052→0053） |
| study-guide §7 | 对照表与 Spec / 工程问题一致 |
| `RepositoryProtocols.swift` 文件头 | 标明阶段语义 |

零运行时行为变化。

### 阶段 2 — [0052](../../issues/0052-ios-fine-grained-ports-executor.md)（细粒度 Port）

| 交付 | 说明 |
|------|------|
| Domain Port | `MessageStore` / `MessageRemote`、`SyncCursorStore` / `SyncRemote`、`ConversationStore` / `ConversationRemote` |
| 删除粗协议 | `MessageRepository` / `ConversationRepository` / `SyncRepository` / `PushRepository` / `WebSocketRepository` |
| Conform | `PortConformances.swift`：`LocalDatabase` / `MessageAPI` / `SyncAPI` / `ConversationAPI` extension |
| Executor | `MessageSendExecutor` / `SyncExecutor` 依赖 `any` Port，不再写死 concrete 字段类型 |
| 测试 | `PortFakesTests`：`FakeMessageRemote`（hang→sending 超时）+ `FakeSyncRemote`（游标推进） |
| 组合根 | `AppServices.make()` 仍注入 Live，仅构造参数改为 Port 语义 |

生产路径行为意图不变；`MediaRepository` 仍留给 0049。

## 3. 未做 / 留给 0053

- `AppServices` 对外全面改为 Port 注入，以及 Fake 组装「enqueue → remote 成败 → 状态收敛」的 Application 层可测路径
- 工程问题 19 标记 `resolved`（等 0053 关闭）

## 4. 验收核对

| 票 | 准则 | 结果 |
|----|------|------|
| 0051 | Spec / 工程问题 19 / study-guide / 文件头一致；禁止空壳；无依赖图行为变更 | ✅ |
| 0052 | `AppServices.make()` 可组装；Executor 依赖协议；粗协议退出 Spec 待实现清单；Fake remote 测通过 | ✅ |

## 5. 关键文件

- `Specs/13-iOS客户端架构设计.md` §6
- `docs/engineering-problems/19-domain-repository-ports-vs-concrete-executors.md`
- `docs/ios-client-study-guide.md` §7
- `ios/Packages/ChatDomain/.../RepositoryProtocols.swift`
- `ios/Packages/ChatInfrastructure/.../Ports/PortConformances.swift`
- `MessageSendExecutor.swift` / `SyncExecutor.swift` / `AppServices.swift`
- `PortFakesTests.swift`
- `issues/0051` · `0052` · `0053` · `INDEX.md`

## 6. 结论

端口漂移的「文档真相」与「Executor 可替换缝」已对齐；下一步用 0053 把组合根与 Fake 主路径收口，再关闭工程问题 19。
