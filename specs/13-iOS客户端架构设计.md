# Spec 13 - iOS 客户端架构设计

## 1. 目标

定义 iOS 客户端的模块边界、状态管理和本地数据策略，使客户端成为一个可恢复、可测试、可扩展的聊天终端，而不是仅靠页面状态拼装的演示程序。本 spec 重点学习如何在移动端实现“本地数据库为单一真相源”的架构。

## 2. 设计原则

- **UI 只消费状态，不直接拼装业务真相。** 页面从 ViewModel 读取，ViewModel 从 Repository 读取，Repository 从本地 DB 读取。
- **本地数据库是客户端单一可信状态源。** 网络结果先转为领域事件，再更新本地 DB 投影。
- **发送队列、同步队列和推送事件统一进入状态机。** 避免多入口并发修改同一份数据。
- **离线优先。** 断网时优先使用本地数据，恢复后执行增量同步。

## 3. 模块分层与依赖关系

```
┌─────────────────────────────────────────────┐
│           ChatPresentation                  │
│  SwiftUI/UIKit Views + ViewModels           │
│  依赖: ChatApplication                      │
└──────────────────┬──────────────────────────┘
                   │
┌──────────────────▼──────────────────────────┐
│           ChatApplication                   │
│  Use Cases: SendMessage, Sync, Login,       │
│  HandlePush, BackgroundRecovery             │
│  依赖: ChatDomain, ChatInfrastructure       │
└──────────────────┬──────────────────────────┘
                   │
┌──────────────────▼──────────────────────────┐
│              ChatDomain                     │
│  Entities, Repository Protocols,            │
│  State Machine Rules, Business Policies     │
│  不依赖任何具体框架                          │
└──────────────────┬──────────────────────────┘
                   │
┌──────────────────▼──────────────────────────┐
│           ChatInfrastructure                │
│  Network, WebSocket, Local DB (GRDB),       │
│  Keychain, Push, File Cache, Logging        │
│  实现 ChatDomain 的 Repository Protocols     │
└─────────────────────────────────────────────┘
                   │
┌──────────────────┴──────────────────────────┐
│               AppCore                       │
│  DI Container, Build Config,                │
│  Module Assembly, App Lifecycle             │
└─────────────────────────────────────────────┘
```

**依赖规则**：
- 上层依赖下层，下层不依赖上层。
- `ChatDomain` 不依赖 `ChatInfrastructure`，只依赖协议。
- `ChatPresentation` 不直接调用 `ChatInfrastructure`。

## 4. 本地数据策略

### 4.1 本地数据库（GRDB / CoreData / SQLite）

```sql
-- 消息表
CREATE TABLE messages (
    local_id          INTEGER PRIMARY KEY AUTOINCREMENT,
    server_message_id VARCHAR(64),
    client_message_id VARCHAR(64) NOT NULL,
    conversation_id   VARCHAR(64) NOT NULL,
    conversation_seq  BIGINT,
    sender_user_id    BIGINT,
    message_type      VARCHAR(20) NOT NULL,
    content           TEXT,
    status            VARCHAR(20) NOT NULL,  -- queued/sending/accepted/delivered/read/failed
    server_received_at INTEGER,
    created_at        INTEGER NOT NULL,
    
    UNIQUE(server_message_id),
    UNIQUE(client_message_id),
    INDEX idx_messages_conversation_seq (conversation_id, conversation_seq)
);

-- 会话摘要表
CREATE TABLE conversation_summaries (
    user_id           BIGINT NOT NULL,
    conversation_id   VARCHAR(64) NOT NULL,
    type              VARCHAR(20) NOT NULL,  -- private/group
    title             VARCHAR(255),
    last_message_id   VARCHAR(64),
    last_message_preview TEXT,
    last_message_at   INTEGER,
    unread_count      INTEGER NOT NULL DEFAULT 0,
    is_pinned         BOOLEAN NOT NULL DEFAULT FALSE,
    is_muted          BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at        INTEGER NOT NULL,
    
    PRIMARY KEY (user_id, conversation_id)
);

-- 同步游标表
CREATE TABLE sync_cursors (
    user_id        BIGINT NOT NULL PRIMARY KEY,
    device_id      VARCHAR(64) NOT NULL,
    last_event_seq BIGINT NOT NULL DEFAULT 0,
    last_sync_at   INTEGER
);
```

### 4.2 本地数据更新规则

- 所有 UI 展示数据来自本地 DB 查询（配合 `@Query` 或 Combine publisher）。
- 发送中的消息先以 `status = queued` 写入本地 DB，再发起网络请求。
- 服务端确认后，更新 `server_message_id`、`conversation_seq`、`status = accepted`。
- 收到 WebSocket / 推送 / 同步事件后，统一转换为领域事件并写入 DB。
- 数据库更新触发 UI 自动刷新。

## 5. 本地状态机

### 5.1 消息状态机

```
[draft]
  │
  ▼
[queued] ──(开始发送)──► [sending]
  │                          │
  │                          ├─(HTTP 200)──► [accepted]
  │                          │                    │
  │                          │                    ├─(投递 ACK)──► [delivered]
  │                          │                    │                    │
  │                          │                    │                    ├─(已读回执)──► [read]
  │                          │                    │                    │
  │                          └─(4xx/超时)───────► [failed]
  │
  └─(幂等命中/重复事件)──► 不更新状态，忽略
```

### 5.2 发送队列执行器

```swift
actor MessageSendExecutor {
    private var pendingMessages: [PendingMessage] = []
    private var isProcessing = false
    
    func enqueue(_ message: PendingMessage) {
        pendingMessages.append(message)
        Task { await process() }
    }
    
    private func process() async {
        guard !isProcessing else { return }
        isProcessing = true
        defer { isProcessing = false }
        
        while let message = pendingMessages.first {
            do {
                let result = try await networkService.sendMessage(message)
                await localDB.updateMessage(
                    clientMessageId: message.clientMessageId,
                    serverMessageId: result.serverMessageId,
                    conversationSeq: result.conversationSeq,
                    status: .accepted
                )
                pendingMessages.removeFirst()
            } catch {
                let shouldRetry = retryPolicy.shouldRetry(message.retryCount, error: error)
                if shouldRetry {
                    message.retryCount += 1
                    let delay = retryPolicy.delay(for: message.retryCount)
                    try? await Task.sleep(nanoseconds: UInt64(delay * 1_000_000_000))
                } else {
                    await localDB.updateMessageStatus(
                        clientMessageId: message.clientMessageId,
                        status: .failed
                    )
                    pendingMessages.removeFirst()
                }
            }
        }
    }
}
```

### 5.3 同步执行器

```swift
actor SyncExecutor {
    func syncIncremental() async throws {
        let localCursor = try localDB.getSyncCursor()
        let response = try await syncService.fetchEvents(cursor: localCursor)
        
        for event in response.events {
            let domainEvent = try eventMapper.map(event)
            try await apply(domainEvent)
            try localDB.updateSyncCursor(event.eventSeq)
        }
        
        if response.hasMore {
            try await syncIncremental()
        }
    }
    
    func apply(_ event: DomainEvent) async throws {
        switch event {
        case .messageCreated(let msg):
            try localDB.insertMessageIfNeeded(msg)
        case .conversationUpdated(let summary):
            try localDB.upsertConversationSummary(summary)
        case .messageRead(let conversationId, let seq):
            try localDB.updateLastReadSeq(conversationId, seq: seq)
        // ...
        }
    }
}
```

## 6. 仓储协议清单

```swift
// ChatDomain/RepositoryProtocols.swift

protocol MessageRepository {
    func getMessages(in conversationId: String, limit: Int) async throws -> [Message]
    func sendMessage(_ request: SendMessageRequest) async throws -> SendMessageResponse
    func insertMessage(_ message: Message) async throws
    func updateMessageStatus(clientMessageId: String, status: MessageStatus) async throws
}

protocol ConversationRepository {
    func getConversations() async throws -> [ConversationSummary]
    func getConversation(id: String) async throws -> ConversationSummary?
    func upsertConversation(_ conversation: ConversationSummary) async throws
}

protocol SyncRepository {
    func getSyncCursor() async throws -> Int64
    func updateSyncCursor(_ seq: Int64) async throws
    func syncEvents(from cursor: Int64) async throws -> SyncResponse
}

protocol PushRepository {
    func registerPushToken(_ token: Data) async throws
    func handleRemoteNotification(_ userInfo: [AnyHashable: Any]) async
}

protocol WebSocketRepository {
    func connect() async throws
    func disconnect() async
    func sendFrame(_ frame: WebSocketFrame) async throws
    var messageStream: AsyncStream<WebSocketFrame> { get }
}

protocol AuthRepository {
    func login(phone: String, code: String) async throws -> AuthTokens
    func refreshToken() async throws -> AuthTokens
    func logout() async throws
}

protocol MediaRepository {
    func uploadImage(_ data: Data, metadata: ImageMetadata) async throws -> Attachment
    func downloadImage(objectKey: String) async throws -> Data
}
```

## 7. 推送与同步统一入口

### 7.1 统一事件总线

```swift
enum RemoteEvent {
    case messageDelivery(Message)
    case messageStatusUpdate(MessageStatusUpdate)
    case syncTrigger(latestEventSeq: Int64)
    case conversationUpdate(ConversationSummary)
    case groupEvent(GroupEvent)
}

actor RemoteEventProcessor {
    private let eventBus: EventBus<RemoteEvent>
    
    func handleWebSocketFrame(_ frame: WebSocketFrame) async {
        guard let event = mapFrameToEvent(frame) else { return }
        await process(event)
    }
    
    func handleRemoteNotification(_ userInfo: [AnyHashable: Any]) async {
        guard let event = mapNotificationToEvent(userInfo) else { return }
        await process(event)
    }
    
    func handleSyncResponse(_ response: SyncResponse) async {
        for event in response.events {
            guard let domainEvent = mapSyncEventToEvent(event) else { continue }
            await process(domainEvent)
        }
    }
    
    private func process(_ event: RemoteEvent) async {
        switch event {
        case .messageDelivery(let msg):
            try? await localDB.insertMessageIfNeeded(msg)
        case .syncTrigger(let latestSeq):
            try? await syncExecutor.syncIncremental()
        // ...
        }
    }
}
```

### 7.2 处理优先级

1. **WebSocket 实时事件**：最高优先级，直接应用。
2. **Silent Push 唤醒**：触发后台同步，同步完成后应用事件。
3. **Visible Push 点击**：用户启动 App，执行全量增量同步。
4. **BGTaskScheduler 后台刷新**：最低优先级， opportunistic 同步。

## 8. 生命周期与恢复

### 8.1 App 启动恢复

```swift
func applicationDidFinishLaunching(_ application: UIApplication) {
    // 1. 初始化 DI 与本地 DB
    AppContainer.shared.initialize()
    
    // 2. 检查登录状态
    guard authRepository.isLoggedIn else { return }
    
    // 3. 建立 WebSocket 连接（携带 last_event_seq）
    Task {
        try? await webSocketRepository.connect()
    }
    
    // 4. 触发增量同步
    Task {
        try? await syncExecutor.syncIncremental()
    }
    
    // 5. 注册推送 Token
    Task {
        if let token = pushToken {
            try? await pushRepository.registerPushToken(token)
        }
    }
}
```

### 8.2 前后台切换

- **进入后台**：
  - 保持 WebSocket 连接，心跳间隔延长至 120s。
  - 注册 Silent Push，确保新消息能唤醒。
- **回到前台**：
  - 恢复 30s 心跳。
  - 立即检查 `last_event_seq` 是否落后，触发增量同步。

### 8.3 断网恢复

- 发送队列暂停，消息状态保持 `sending` 或 `queued`。
- 网络恢复后，发送队列继续执行；已处于 `sending` 状态的消息通过幂等键重试。
- 同步执行器优先拉取离线期间事件，再处理新到达事件。

## 9. P0 客户端能力边界

| 能力 | P0 | P1 |
|------|----|----|
| 本地消息存储与展示 | ✓ | — |
| 发送队列与重试 | ✓ | — |
| WebSocket 实时投递 | ✓ | — |
| 离线同步与增量拉取 | ✓ | — |
| 会话摘要与未读数 | ✓ | — |
| 推送唤醒与去重 | ✓ | — |
| 图片上传/下载/缩略图 | ✓ | — |
| Notification Service Extension 解密 | — | ✓ |
| E2EE 密钥管理 | — | ✓ |
| 视频转码/内容审核 | — | ✓ |

## 10. 流程描述

用户发送消息时，应用层先生成本地消息实体并写入数据库，再通过网络服务发起消息接收请求。服务端确认后，本地消息收敛到正式消息记录。推送、同步和长连接事件都通过统一入口转化为领域事件，最终更新数据库投影，页面只监听投影变化。

## 11. 交付物

- [ ] 模块依赖图（本 spec §3）
- [ ] 本地数据库 Schema（本 spec §4.1）
- [ ] 消息状态机图（本 spec §5.1）
- [ ] 发送队列与同步执行器设计（本 spec §5.2–5.3）
- [ ] 仓储协议清单（本 spec §6）
- [ ] 推送与同步统一入口设计（本 spec §7）
- [ ] App 生命周期恢复流程（本 spec §8）
- [ ] P0 客户端能力边界（本 spec §9）
