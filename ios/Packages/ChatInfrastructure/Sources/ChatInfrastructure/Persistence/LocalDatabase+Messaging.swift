import Foundation
import GRDB
import ChatDomain

public struct MessageRecord: Codable, FetchableRecord, PersistableRecord, Sendable {
    public var localID: Int64?
    public var serverMessageID: String?
    public var clientMessageID: String
    public var conversationID: String
    public var conversationSeq: Int64?
    public var senderUserID: Int64?
    public var messageType: String
    public var content: String?
    public var status: String
    public var serverReceivedAt: Int64?
    public var createdAt: Int64

    public static let databaseTableName = "messages"

    enum CodingKeys: String, CodingKey {
        case localID = "local_id"
        case serverMessageID = "server_message_id"
        case clientMessageID = "client_message_id"
        case conversationID = "conversation_id"
        case conversationSeq = "conversation_seq"
        case senderUserID = "sender_user_id"
        case messageType = "message_type"
        case content
        case status
        case serverReceivedAt = "server_received_at"
        case createdAt = "created_at"
    }
}

public struct ConversationSummaryRecord: Codable, FetchableRecord, PersistableRecord, Sendable {
    public var userID: Int64
    public var conversationID: String
    public var type: String
    public var title: String?
    public var lastMessagePreview: String?
    public var lastMessageAt: Int64?
    public var unreadCount: Int
    public var isPinned: Bool
    public var isMuted: Bool
    public var updatedAt: Int64

    public static let databaseTableName = "conversation_summaries"

    enum CodingKeys: String, CodingKey {
        case userID = "user_id"
        case conversationID = "conversation_id"
        case type
        case title
        case lastMessagePreview = "last_message_preview"
        case lastMessageAt = "last_message_at"
        case unreadCount = "unread_count"
        case isPinned = "is_pinned"
        case isMuted = "is_muted"
        case updatedAt = "updated_at"
    }
}

extension LocalDatabase {
    public func insertMessage(_ message: Message) throws {
        let record = MessageRecord(
            localID: nil,
            serverMessageID: message.serverMessageID,
            clientMessageID: message.clientMessageID,
            conversationID: message.conversationID,
            conversationSeq: message.conversationSeq,
            senderUserID: message.senderUserID,
            messageType: message.messageType,
            content: message.content,
            status: message.status.rawValue,
            serverReceivedAt: message.serverReceivedAt.map { Int64($0.timeIntervalSince1970 * 1000) },
            createdAt: Int64(message.createdAt.timeIntervalSince1970 * 1000)
        )
        try dbQueue.write { db in
            try record.insert(db, onConflict: .ignore)
        }
    }

    public func updateMessageAccepted(
        clientMessageID: String,
        serverMessageID: String,
        conversationSeq: Int64,
        serverReceivedAtMs: Int64
    ) throws {
        try dbQueue.write { db in
            try db.execute(
                sql: """
                UPDATE messages SET
                  server_message_id = ?,
                  conversation_seq = ?,
                  status = ?,
                  server_received_at = ?
                WHERE client_message_id = ?
                """,
                arguments: [
                    serverMessageID,
                    conversationSeq,
                    MessageStatus.accepted.rawValue,
                    serverReceivedAtMs,
                    clientMessageID,
                ]
            )
        }
    }

    public func updateMessageStatus(clientMessageID: String, status: MessageStatus) throws {
        try dbQueue.write { db in
            try db.execute(
                sql: "UPDATE messages SET status = ? WHERE client_message_id = ?",
                arguments: [status.rawValue, clientMessageID]
            )
        }
    }

    public func fetchMessages(conversationID: String, limit: Int) throws -> [MessageRecord] {
        try dbQueue.read { db in
            try MessageRecord
                .filter(Column("conversation_id") == conversationID)
                .order(Column("created_at").asc)
                .limit(limit)
                .fetchAll(db)
        }
    }

    public func fetchPendingSend(limit: Int) throws -> [MessageRecord] {
        try dbQueue.read { db in
            try MessageRecord.fetchAll(
                db,
                sql: """
                SELECT * FROM messages
                WHERE status IN (?, ?)
                ORDER BY created_at ASC
                LIMIT ?
                """,
                arguments: [
                    MessageStatus.queued.rawValue,
                    MessageStatus.sending.rawValue,
                    limit,
                ]
            )
        }
    }

    public func upsertConversationSummary(_ summary: ConversationSummary) throws {
        let now = Int64(Date().timeIntervalSince1970 * 1000)
        let record = ConversationSummaryRecord(
            userID: summary.userID,
            conversationID: summary.conversationID,
            type: summary.type,
            title: summary.title,
            lastMessagePreview: summary.lastMessagePreview,
            lastMessageAt: summary.lastMessageAt.map { Int64($0.timeIntervalSince1970 * 1000) },
            unreadCount: summary.unreadCount,
            isPinned: summary.isPinned,
            isMuted: summary.isMuted,
            updatedAt: now
        )
        try dbQueue.write { db in
            try record.save(db)
        }
    }

    public func fetchConversationSummaries(userID: Int64) throws -> [ConversationSummaryRecord] {
        try dbQueue.read { db in
            try ConversationSummaryRecord
                .filter(Column("user_id") == userID)
                .order(Column("updated_at").desc)
                .fetchAll(db)
        }
    }

    public func getSyncCursor(userID: Int64, deviceID: String) throws -> Int64 {
        try dbQueue.read { db in
            let value = try Int64.fetchOne(
                db,
                sql: """
                SELECT last_event_seq FROM sync_cursors
                WHERE user_id = ? AND device_id = ?
                """,
                arguments: [userID, deviceID]
            )
            return value ?? 0
        }
    }

    public func updateSyncCursor(userID: Int64, deviceID: String, lastEventSeq: Int64) throws {
        let now = Int64(Date().timeIntervalSince1970 * 1000)
        try dbQueue.write { db in
            try db.execute(
                sql: """
                INSERT INTO sync_cursors (user_id, device_id, last_event_seq, last_sync_at)
                VALUES (?, ?, ?, ?)
                ON CONFLICT(user_id, device_id) DO UPDATE SET
                  last_event_seq = MAX(sync_cursors.last_event_seq, excluded.last_event_seq),
                  last_sync_at = excluded.last_sync_at
                """,
                arguments: [userID, deviceID, lastEventSeq, now]
            )
        }
    }

    /// 幂等写入对端消息：按 server_message_id 去重。
    public func upsertIncomingMessage(from payload: MessageCreatedPayload) throws {
        try dbQueue.write { db in
            if let existing = try Int64.fetchOne(
                db,
                sql: "SELECT local_id FROM messages WHERE server_message_id = ?",
                arguments: [payload.serverMessageID]
            ), existing > 0 {
                return
            }
            let clientID = "remote-\(payload.serverMessageID)"
            let createdAt = payload.serverReceivedAtMs
                ?? Int64(Date().timeIntervalSince1970 * 1000)
            try db.execute(
                sql: """
                INSERT INTO messages (
                  server_message_id, client_message_id, conversation_id, conversation_seq,
                  sender_user_id, message_type, content, status, server_received_at, created_at
                ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                ON CONFLICT(client_message_id) DO NOTHING
                """,
                arguments: [
                    payload.serverMessageID,
                    clientID,
                    payload.conversationID,
                    payload.conversationSeq,
                    payload.senderUserID,
                    payload.messageType,
                    payload.content,
                    MessageStatus.accepted.rawValue,
                    payload.serverReceivedAtMs,
                    createdAt,
                ]
            )
        }
    }
}
