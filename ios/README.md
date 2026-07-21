# LiveChat iOS

iOS 客户端架构骨架，按 Spec 13 的 5 层模块划分。

## 模块依赖关系

```
ChatPresentation (SwiftUI Views + ViewModels)
    └── ChatApplication (Use Cases: SendMessage, Sync, Login)
        ├── ChatDomain (Entities, Repository Protocols, State Machine)
        └── ChatInfrastructure (GRDB, Network, WebSocket, Keychain)
            └── ChatDomain (implements Repository protocols)

AppCore (DI Container, App Lifecycle)
```

- `ChatDomain` — 纯 Swift，零外部依赖（Foundation 除外）
- `ChatInfrastructure` — 依赖 ChatDomain + GRDB
- `ChatApplication` — 依赖 ChatDomain + ChatInfrastructure
- `ChatPresentation` — 依赖 ChatApplication
- `AppCore` — 依赖所有模块

## 编译

```bash
xcodegen generate
xcodebuild -project LiveChat.xcodeproj -scheme LiveChat build
```

## 状态

P1 学习扩展，当前仅骨架（Entity + Repository 协议 + 消息状态机 + GRDB Schema）。
UI 实现在后续 Phase。

## 关键文件

| 文件 | 内容 |
|------|------|
| `Packages/ChatDomain/Sources/Entities.swift` | User, Message, ConversationSummary, Device, SyncCursor |
| `Packages/ChatDomain/Sources/MessageStatus.swift` | 消息状态机枚举（7 状态 + 合法转移规则） |
| `Packages/ChatDomain/Sources/RepositoryProtocols.swift` | 7 个 Repository 协议 |
| `Packages/ChatDomain/Sources/RemoteEvent.swift` | 统一远程事件枚举 |
| `Packages/ChatInfrastructure/Sources/Database.swift` | GRDB migrator + 表定义 |
| `Packages/ChatInfrastructure/Sources/Repositories.swift` | Repository stub 实现 |
| `AppCore/Sources/AppContainer.swift` | DI Container |
| `AppCore/Sources/LiveChatApp.swift` | @main App 入口 |
