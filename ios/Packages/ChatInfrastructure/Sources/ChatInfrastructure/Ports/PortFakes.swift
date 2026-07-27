import Foundation
import ChatDomain

// MARK: - Port fakes（0053：测试 / 组合根 harness；非生产路径）

/// 内存 MessageStore：无 GRDB / 无 HTTP。
public final class FakeMessageStore: MessageStore, @unchecked Sendable {
    private let lock = NSLock()
    private var byClientID: [String: Message] = [:]
    private var byServerID: [String: Message] = [:]

    public init() {}

    public func insertMessage(_ message: Message) throws {
        lock.lock()
        defer { lock.unlock() }
        byClientID[message.clientMessageID] = message
        if let sid = message.serverMessageID {
            byServerID[sid] = message
        }
    }

    public func updateMessageStatus(clientMessageID: String, status: MessageStatus) throws {
        lock.lock()
        defer { lock.unlock() }
        guard var message = byClientID[clientMessageID] else { return }
        message.status = status
        byClientID[clientMessageID] = message
    }

    public func updateMessageAccepted(
        clientMessageID: String,
        serverMessageID: String,
        conversationSeq: Int64,
        serverReceivedAtMs: Int64
    ) throws {
        lock.lock()
        defer { lock.unlock() }
        guard var message = byClientID[clientMessageID] else { return }
        message = Message(
            serverMessageID: serverMessageID,
            clientMessageID: message.clientMessageID,
            conversationID: message.conversationID,
            conversationSeq: conversationSeq,
            senderUserID: message.senderUserID,
            messageType: message.messageType,
            content: message.content,
            status: .accepted,
            serverReceivedAt: Date(timeIntervalSince1970: Double(serverReceivedAtMs) / 1000),
            createdAt: message.createdAt
        )
        byClientID[clientMessageID] = message
        byServerID[serverMessageID] = message
    }

    public func fetchPendingSend(limit: Int) throws -> [Message] {
        lock.lock()
        defer { lock.unlock() }
        return byClientID.values
            .filter { $0.status == .queued || $0.status == .sending }
            .sorted { $0.createdAt < $1.createdAt }
            .prefix(limit)
            .map { $0 }
    }

    public func upsertRemoteMessage(
        serverMessageID: String,
        conversationID: String,
        conversationSeq: Int64,
        senderUserID: Int64,
        messageType: String,
        content: String,
        serverReceivedAtMs: Int64?
    ) throws {
        lock.lock()
        defer { lock.unlock() }
        if byServerID[serverMessageID] != nil { return }
        let message = Message(
            serverMessageID: serverMessageID,
            clientMessageID: "remote-\(serverMessageID)",
            conversationID: conversationID,
            conversationSeq: conversationSeq,
            senderUserID: senderUserID,
            messageType: messageType,
            content: content,
            status: .accepted,
            serverReceivedAt: serverReceivedAtMs.map {
                Date(timeIntervalSince1970: Double($0) / 1000)
            }
        )
        byServerID[serverMessageID] = message
        byClientID[message.clientMessageID] = message
    }

    public func markOwnMessagesRead(conversationID: String, upToSeq: Int64, myUserID: Int64) throws {
        lock.lock()
        defer { lock.unlock() }
        for (id, message) in byClientID {
            guard message.conversationID == conversationID,
                  message.senderUserID == myUserID,
                  let seq = message.conversationSeq,
                  seq <= upToSeq,
                  message.status == .accepted || message.status == .delivered
            else { continue }
            var updated = message
            updated.status = .read
            byClientID[id] = updated
            if let sid = updated.serverMessageID {
                byServerID[sid] = updated
            }
        }
    }

    public func message(clientMessageID: String) -> Message? {
        lock.lock()
        defer { lock.unlock() }
        return byClientID[clientMessageID]
    }
}

public final class FakeMessageRemote: MessageRemote, @unchecked Sendable {
    public enum Behavior: Sendable {
        case succeed(SendMessageResponse)
        case fail(Error)
        /// 首次挂起直到调用方超时；再次调用失败，结束 process 循环。
        case hangThenFail
    }

    private let behavior: Behavior
    private var _sendCount = 0

    public var sendCount: Int {
        _sendCount
    }

    public init(behavior: Behavior) {
        self.behavior = behavior
    }

    public func sendMessage(_ request: SendMessageRequest) async throws -> SendMessageResponse {
        _sendCount += 1
        let n = _sendCount
        switch behavior {
        case .succeed(let response):
            return response
        case .fail(let error):
            throw error
        case .hangThenFail:
            if n == 1 {
                try await Task.sleep(nanoseconds: 60_000_000_000)
                throw CancellationError()
            }
            throw HTTPClientError.status(code: 500, body: "fail after hang")
        }
    }
}

public final class FakeSyncRemote: SyncRemote, @unchecked Sendable {
    private let pages: [SyncResponse]
    private var index = 0

    public init(pages: [SyncResponse]) {
        self.pages = pages
    }

    public func fetchEvents(from cursor: Int64, limit: Int) async throws -> SyncResponse {
        if index >= pages.count {
            return SyncResponse(events: [], hasMore: false, latestEventSeq: cursor)
        }
        let page = pages[index]
        index += 1
        return page
    }
}

public final class FakeSyncCursorStore: SyncCursorStore, @unchecked Sendable {
    private let lock = NSLock()
    private var cursors: [String: Int64] = [:]

    public init() {}

    private func key(userID: Int64, deviceID: String) -> String {
        "\(userID):\(deviceID)"
    }

    public func getSyncCursor(userID: Int64, deviceID: String) throws -> Int64 {
        lock.lock()
        defer { lock.unlock() }
        return cursors[key(userID: userID, deviceID: deviceID)] ?? 0
    }

    public func updateSyncCursor(userID: Int64, deviceID: String, lastEventSeq: Int64) throws {
        lock.lock()
        defer { lock.unlock() }
        cursors[key(userID: userID, deviceID: deviceID)] = lastEventSeq
    }
}

public final class FakeConversationStore: ConversationStore, @unchecked Sendable {
    private let lock = NSLock()
    private var summaries: [String: ConversationSummary] = [:]

    public init() {}

    public func upsertConversationSummary(_ summary: ConversationSummary) throws {
        lock.lock()
        defer { lock.unlock() }
        summaries[summary.conversationID] = summary
    }

    public func fetchConversationSummaries(userID: Int64) throws -> [ConversationSummary] {
        lock.lock()
        defer { lock.unlock() }
        return summaries.values.filter { $0.userID == userID }
    }

    public func clearUnread(userID: Int64, conversationID: String) throws {
        lock.lock()
        defer { lock.unlock() }
        guard var summary = summaries[conversationID], summary.userID == userID else { return }
        summary = ConversationSummary(
            userID: summary.userID,
            conversationID: summary.conversationID,
            type: summary.type,
            title: summary.title,
            lastMessagePreview: summary.lastMessagePreview,
            lastMessageAt: summary.lastMessageAt,
            unreadCount: 0,
            isPinned: summary.isPinned,
            isMuted: summary.isMuted
        )
        summaries[conversationID] = summary
    }
}
