import Foundation
import ChatDomain

/// Fanout `message_created` payload（与服务端 outbox / sync 对齐）。
public struct MessageCreatedPayload: Decodable, Sendable {
    public let serverMessageID: String
    public let conversationID: String
    public let conversationSeq: Int64
    public let senderUserID: Int64
    public let senderDeviceID: String?
    public let messageType: String
    public let content: String
    public let serverReceivedAtMs: Int64?

    public init(
        serverMessageID: String,
        conversationID: String,
        conversationSeq: Int64,
        senderUserID: Int64,
        senderDeviceID: String? = nil,
        messageType: String,
        content: String,
        serverReceivedAtMs: Int64? = nil
    ) {
        self.serverMessageID = serverMessageID
        self.conversationID = conversationID
        self.conversationSeq = conversationSeq
        self.senderUserID = senderUserID
        self.senderDeviceID = senderDeviceID
        self.messageType = messageType
        self.content = content
        self.serverReceivedAtMs = serverReceivedAtMs
    }

    enum CodingKeys: String, CodingKey {
        case serverMessageID = "server_message_id"
        case conversationID = "conversation_id"
        case conversationSeq = "conversation_seq"
        case senderUserID = "sender_user_id"
        case senderDeviceID = "sender_device_id"
        case messageType = "message_type"
        case content
        case serverReceivedAtMs = "server_received_at_ms"
    }
}

public struct SyncRunResult: Sendable, Equatable {
    public let appliedCount: Int
    public let cursor: Int64
    public let touchedConversationIDs: [String]

    public init(appliedCount: Int, cursor: Int64, touchedConversationIDs: [String]) {
        self.appliedCount = appliedCount
        self.cursor = cursor
        self.touchedConversationIDs = touchedConversationIDs
    }
}

/// 增量同步：拉事件 → 分批 apply → 仅成功后推进本地游标。
public actor SyncExecutor {
    public static let pageSize = 50

    private let database: LocalDatabase
    private let api: SyncAPI
    private let session: SessionStore
    private var isRunning = false

    public init(database: LocalDatabase, api: SyncAPI, session: SessionStore) {
        self.database = database
        self.api = api
        self.session = session
    }

    public func syncIncremental() async throws -> SyncRunResult {
        guard !isRunning else {
            // Single-flight：已有一轮在跑则直接返回当前游标。
            let creds = try session.load()
            let cursor = try database.getSyncCursor(
                userID: creds?.userID ?? 0,
                deviceID: creds?.deviceID ?? ""
            )
            return SyncRunResult(appliedCount: 0, cursor: cursor, touchedConversationIDs: [])
        }
        isRunning = true
        defer { isRunning = false }

        guard let creds = try session.load() else {
            throw HTTPClientError.status(code: 401, body: "not logged in")
        }

        var cursor = try database.getSyncCursor(userID: creds.userID, deviceID: creds.deviceID)
        var applied = 0
        var touched = Set<String>()

        while true {
            let page = try await api.fetchEvents(from: cursor, limit: Self.pageSize)
            if page.events.isEmpty {
                break
            }

            for event in page.events {
                let conversationID = try apply(event: event, myUserID: creds.userID)
                if let conversationID {
                    touched.insert(conversationID)
                }
                // 单事件成功后再推进，失败中断不丢后续重拉。
                try database.updateSyncCursor(
                    userID: creds.userID,
                    deviceID: creds.deviceID,
                    lastEventSeq: event.eventSeq
                )
                cursor = event.eventSeq
                applied += 1
            }

            await Task.yield()

            if !page.hasMore {
                break
            }
        }

        return SyncRunResult(
            appliedCount: applied,
            cursor: cursor,
            touchedConversationIDs: Array(touched).sorted()
        )
    }

    private func apply(event: SyncEvent, myUserID: Int64) throws -> String? {
        switch event.eventType {
        case "message_created":
            guard let data = event.payload.data(using: .utf8) else { return nil }
            let payload = try JSONDecoder().decode(MessageCreatedPayload.self, from: data)
            try IncomingMessageApplier.applyMessageCreated(payload, myUserID: myUserID, database: database)
            return payload.conversationID
        default:
            // 未知类型跳过但仍推进游标，避免卡死；后续票可扩展。
            return event.conversationID
        }
    }
}
