---
id: "0021"
title: "iOS 客户端架构骨架（P1 学习扩展）"
status: ready-for-agent
labels: ["ready-for-agent", "p1"]
parent: "0010"
blocked_by: []
created_at: 2026-07-21
---

# 0021 — iOS 客户端架构骨架（P1 学习扩展）

## Parent

Phase 3: 规模化与工程质量（P1 学习扩展），对应 Spec 13。

> **本票属于 P1 学习扩展，不在 Phase 3 的 P0 紧急范围内。P0 的 0016+0018+0019+0020 完成后再启动。**

## What to build

交付 iOS 项目的模块骨架：Xcode 项目 + Swift Package Manager 5 层模块划分、ChatDomain 纯 Swift 包（Entity + Repository 协议 + 消息状态机）、ChatInfrastructure 包（GRDB SQLite 本地 DB schema 初始化）、AppCore 包（DI Container + App Lifecycle 骨架）。不实现任何 UI——本票只交付编译通过的模块骨架和协议定义。

端到端行为：开发者 clone 仓库 → 打开 `ios/LiveChat.xcodeproj` → 编译通过 → 运行后 App 执行 AppCore 启动流程（初始化 DI、检查登录状态、打印模块加载日志）→ ChatDomain 的消息状态机枚举和 Entity 定义可被所有上层模块引用 → ChatInfrastructure 的 GRDB migrator 能在首次启动时创建本地 SQLite 数据库和 3 张核心表。

## Acceptance criteria

- [ ] `ios/` 目录结构按 Spec 13 §3 的 5 层模块划分：
  - `ChatDomain` — SPM package，纯 Swift，零外部依赖（Foundation 除外）
  - `ChatInfrastructure` — SPM package，依赖 ChatDomain + GRDB
  - `ChatApplication` — SPM package，依赖 ChatDomain + ChatInfrastructure
  - `ChatPresentation` — SPM package，依赖 ChatApplication（本票仅创建空壳，不实现 UI）
  - `AppCore` — 主 App target 的启动组装代码
- [ ] `ChatDomain` 包包含：
  - Entity struct：`Message`、`ConversationSummary`、`SyncCursor`、`Device`、`User`、`Attachment`（字段与 Spec 13 §4.1 DB Schema 对齐）
  - 消息状态机枚举：`enum MessageStatus: String { case draft, queued, sending, accepted, delivered, read, failed }`
  - Repository 协议清单：`MessageRepository`、`ConversationRepository`、`SyncRepository`、`PushRepository`、`WebSocketRepository`、`AuthRepository`、`MediaRepository`（接口签名与 Spec 13 §6 对齐，不实现方法体）
  - 远程事件枚举：`enum RemoteEvent`（case messageDelivery, messageStatusUpdate, syncTrigger, conversationUpdate, groupEvent）
- [ ] `ChatInfrastructure` 包包含：
  - GRDB/SQLite 数据库初始化（`DatabaseMigrator` 注册 3 张表的 migration）
  - 本地 DB schema 迁移：`messages`、`conversation_summaries`、`sync_cursors`（DDL 与 Spec 13 §4.1 一致）
  - Repository 协议的 stub 实现（方法体 `fatalError("Not implemented")`，用于编译验证协议签名）
- [ ] `AppCore` 包含：
  - 基础 DI Container（简单的服务注册/解析，不引入 Swinject 等重型框架）
  - `AppDelegate` 或 `@main App` 入口：`didFinishLaunching` 中按 Spec 13 §8.1 的顺序调用 init + check login + stub connect + stub sync
- [ ] 项目使用 Swift Package Manager 管理依赖（`GRDB.swift` 作为唯一第三方依赖）
- [ ] 编译通过（`xcodebuild -project ios/LiveChat.xcodeproj -scheme LiveChat build`），无编译错误
- [ ] 提供 `ios/README.md`：说明模块依赖关系、编译方式、后续实现的入口指引

## Blocked by

None — can start immediately（iOS 项目完全独立于服务端，P1 可择机启动）。

## 技术难点与注意事项

### 1. SPM 多模块项目的配置

**问题：** Xcode 项目 + SPM 多模块的配置比较繁琐。需要在 project.yml（如果用 XcodeGen）或手动在 Xcode GUI 中配置 target dependencies。

**方案（推荐）：** 使用 `xcodegen` 生成 `.xcodeproj`（不提交到 git），用 `project.yml` 描述 target 和依赖关系。仓库只提交 `project.yml` + Swift 源码。

```
ios/
├── project.yml          # XcodeGen spec
├── README.md
├── Packages/
│   ├── ChatDomain/
│   │   ├── Package.swift
│   │   └── Sources/ChatDomain/
│   ├── ChatInfrastructure/
│   ├── ChatApplication/
│   └── ChatPresentation/
└── AppCore/
    ├── Sources/
    └── Resources/
```

**替代方案：** 纯 SPM 项目（`Package.swift` 在根目录），不依赖 `.xcodeproj`。但 App target 通常还是需要 Xcode project。可以两者结合：SPM 管理 library targets，Xcode project 管理 App target。

**坑点：** XcodeGen 和 `xcodebuild` 的 CI 兼容性需要验证。P0 可以先用最简单的单一 Xcode project + 手动添加 SPM 依赖的方式，后续再优化。

### 2. GRDB vs CoreData 的选择

**问题：** Spec 13 提到了 GRDB / CoreData / SQLite。需要选一个。

**方案：** 选 **GRDB**（`github.com/groue/GRDB.swift`）。理由：
- Swift 原生、类型安全、async/await 一等公民
- 相比于 CoreData：更可控的 SQL、没有 `NSManagedObjectContext` 的线程陷阱
- 相比于纯 SQLite：自带的 `DatabaseMigrator` 和 `ValueObservation`（Combine publisher）非常好用

**坑点：** GRDB 的 `@Query` 属性包装器和 SwiftUI 集成很好，但本票不涉及 UI，不需要关心这个。

### 3. Repository 协议的 stub 实现

**问题：** 协议定义在 ChatDomain，实现在 ChatInfrastructure。Phase 3 只实现协议定义 + stub，真正的网络/DB 逻辑在后续 ticket。

**方案：** ChatInfrastructure 中的每个 Repository 实现类用 `fatalError("Not implemented — pending Phase 4")` 做 stub 方法体。这样项目可以编译通过（验证协议签名正确），但运行时会 crash。上层 ChatApplication 的 Use Case 可以先写，运行时测试时再替换真实实现。

**替代方案：** 使用 Swift 的 `#if DEBUG` + precondition 来区分 debug 和 release 行为。

### 4. 消息状态机的正确性

**问题：** 消息状态机（Spec 13 §5.1）定义了 7 个状态和它们的合法转移。直接写 `switch` 不能防止非法转移。

**方案：**
```swift
enum MessageStatus: String, CaseIterable {
    case draft, queued, sending, accepted, delivered, read, failed
    
    var allowedTransitions: Set<MessageStatus> {
        switch self {
        case .draft:     return [.queued]
        case .queued:    return [.sending]
        case .sending:   return [.accepted, .failed]
        case .accepted:  return [.delivered]
        case .delivered: return [.read]
        case .read:      return []
        case .failed:    return [.queued]  // retry
        }
    }
    
    func canTransition(to target: MessageStatus) -> Bool {
        allowedTransitions.contains(target)
    }
}
```

这个状态机不依赖任何框架，放在 ChatDomain 中。

### 5. 远程事件的统一入口

**问题：** WebSocket 帧、推送通知、Sync Event 三种来源都可能携带"新消息"事件。如果不同来源走不同处理路径，最终会在本地 DB 中产生重复。

**方案：** `RemoteEventProcessor`（Spec 13 §7.1）定义在 ChatApplication 层：
- 三种来源（WebSocket / Push / Sync）都先将原始数据转换为 `RemoteEvent` 枚举
- `RemoteEventProcessor.process(event)` 统一处理：去重（通过 server_message_id）+ 写入 DB
- 这是客户端的"单一事件入口"（类比服务端的 Outbox Consumer 是单一事件出口）

本票只定义协议和枚举，不实现具体 process 逻辑。

### 6. 涉及的关键文件

- `ios/project.yml` — XcodeGen 项目描述文件
- `ios/Packages/ChatDomain/Package.swift` + `Sources/` — 所有 Entity、Protocol、Enum
- `ios/Packages/ChatInfrastructure/Package.swift` + `Sources/` — GRDB migrator + Repository stubs
- `ios/Packages/ChatApplication/Sources/` — RemoteEventProcessor 协议 + Use Case 协议
- `ios/Packages/ChatPresentation/Sources/` — 空壳（后续填 ViewModel + View）
- `ios/AppCore/Sources/` — DI Container + AppDelegate/App entry
- `ios/README.md`
- `.gitignore` — 排除 `.xcodeproj`（如果使用 XcodeGen）
