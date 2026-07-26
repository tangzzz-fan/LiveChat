import Foundation
import GRDB
import ChatDomain

/// 本地 SQLite（Spec 13 §4.1）：单 DatabaseQueue + WAL。
public final class LocalDatabase: Sendable {
    public let dbQueue: DatabaseQueue

    public init(path: String) throws {
        var config = Configuration()
        config.prepareDatabase { db in
            try db.execute(sql: "PRAGMA foreign_keys = ON")
        }
        dbQueue = try DatabaseQueue(path: path, configuration: config)
        try dbQueue.write { db in
            try db.execute(sql: "PRAGMA journal_mode = WAL")
        }
        try migrate()
    }

    public static func inMemory() throws -> LocalDatabase {
        try LocalDatabase(path: ":memory:")
    }

    public static func applicationDefault() throws -> LocalDatabase {
        let fm = FileManager.default
        let base = try fm.url(
            for: .applicationSupportDirectory,
            in: .userDomainMask,
            appropriateFor: nil,
            create: true
        ).appendingPathComponent("LiveChat", isDirectory: true)
        try fm.createDirectory(at: base, withIntermediateDirectories: true)
        return try LocalDatabase(path: base.appendingPathComponent("chat.sqlite").path)
    }

    private func migrate() throws {
        var migrator = DatabaseMigrator()

        migrator.registerMigration("v1_scaffold") { db in
            try db.create(table: "messages") { t in
                t.autoIncrementedPrimaryKey("local_id")
                t.column("server_message_id", .text)
                t.column("client_message_id", .text).notNull().unique()
                t.column("conversation_id", .text).notNull()
                t.column("conversation_seq", .integer)
                t.column("sender_user_id", .integer)
                t.column("message_type", .text).notNull()
                t.column("content", .text)
                t.column("status", .text).notNull().defaults(to: MessageStatus.queued.rawValue)
                t.column("server_received_at", .integer)
                t.column("created_at", .integer).notNull()
            }
            // UNIQUE(server_message_id) where not null is handled in app logic for scaffold
            try db.create(
                index: "idx_messages_conversation_seq",
                on: "messages",
                columns: ["conversation_id", "conversation_seq"]
            )

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

            try db.create(table: "sync_cursors") { t in
                t.column("user_id", .integer).notNull()
                t.column("device_id", .text).notNull()
                t.column("last_event_seq", .integer).notNull().defaults(to: 0)
                t.column("last_sync_at", .integer)
                t.primaryKey(["user_id", "device_id"])
            }
        }

        try migrator.migrate(dbQueue)
    }
}
