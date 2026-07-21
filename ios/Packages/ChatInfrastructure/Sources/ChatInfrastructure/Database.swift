import Foundation
import GRDB
import ChatDomain

/// 本地 SQLite 数据库初始化（Spec 13 §4.1）
public final class Database {
    public let dbWriter: any DatabaseWriter

    public init(path: String) throws {
        dbWriter = try DatabaseQueue(path: path)
        try migrate()
    }

    private func migrate() throws {
        var migrator = DatabaseMigrator()

        migrator.registerMigration("v1_create_messages") { db in
            try db.create(table: "messages") { t in
                t.autoIncrementedPrimaryKey("local_id")
                t.column("server_message_id", .text)
                t.column("client_message_id", .text).notNull()
                t.column("conversation_id", .text).notNull()
                t.column("conversation_seq", .integer)
                t.column("sender_user_id", .integer)
                t.column("message_type", .text).notNull()
                t.column("content", .text)
                t.column("status", .text).notNull().defaults(to: "queued")
                t.column("server_received_at", .integer)
                t.column("created_at", .integer).notNull()
                t.uniqueKey(["server_message_id"])
                t.uniqueKey(["client_message_id"])
                t.index(["conversation_id", "conversation_seq"])
            }
        }

        migrator.registerMigration("v1_create_conversation_summaries") { db in
            try db.create(table: "conversation_summaries") { t in
                t.column("user_id", .integer).notNull()
                t.column("conversation_id", .text).notNull()
                t.column("type", .text).notNull()
                t.column("title", .text)
                t.column("last_message_preview", .text)
                t.column("last_message_at", .integer)
                t.column("unread_count", .integer).notNull().defaults(to: 0)
                t.column("is_pinned", .boolean).notNull().defaults(to: false)
                t.column("is_muted", .boolean).notNull().defaults(to: false)
                t.column("updated_at", .integer).notNull()
                t.primaryKey(["user_id", "conversation_id"])
            }
        }

        migrator.registerMigration("v1_create_sync_cursors") { db in
            try db.create(table: "sync_cursors") { t in
                t.column("user_id", .integer).notNull()
                t.column("device_id", .text).notNull()
                t.column("last_event_seq", .integer).notNull().defaults(to: 0)
                t.column("last_sync_at", .integer)
                t.primaryKey(["user_id", "device_id"])
            }
        }

        try migrator.migrate(dbWriter)
    }
}

// MARK: - GRDB Record types

/// GRDB record for messages table
struct MessageRecord: Codable, FetchableRecord, PersistableRecord {
    var localID: Int64?
    var serverMessageID: String?
    var clientMessageID: String
    var conversationID: String
    var conversationSeq: Int64?
    var senderUserID: Int64?
    var messageType: String
    var content: String?
    var status: String
    var serverReceivedAt: Int?
    var createdAt: Int

    static let databaseTableName = "messages"
}

/// GRDB record for conversation_summaries table
struct ConversationSummaryRecord: Codable, FetchableRecord, PersistableRecord {
    var userID: Int64
    var conversationID: String
    var type: String
    var title: String?
    var lastMessagePreview: String?
    var lastMessageAt: Int?
    var unreadCount: Int
    var isPinned: Bool
    var isMuted: Bool
    var updatedAt: Int

    static let databaseTableName = "conversation_summaries"
}

/// GRDB record for sync_cursors table
struct SyncCursorRecord: Codable, FetchableRecord, PersistableRecord {
    var userID: Int64
    var deviceID: String
    var lastEventSeq: Int64
    var lastSyncAt: Int?

    static let databaseTableName = "sync_cursors"
}
