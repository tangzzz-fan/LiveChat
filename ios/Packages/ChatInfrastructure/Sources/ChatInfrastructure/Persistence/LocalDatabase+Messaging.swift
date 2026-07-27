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

    public func deleteLocalMessage(clientMessageID: String) throws {
        try dbQueue.write { db in
            try db.execute(
                sql: "DELETE FROM messages WHERE client_message_id = ?",
                arguments: [clientMessageID]
            )
        }
    }

    /// 按 Spec / 高负载 #10：同会话内以 `conversation_seq` 升序；无 seq 的 queued/sending 置底（`created_at` 次序）。
    public func fetchMessages(conversationID: String, limit: Int) throws -> [MessageRecord] {
        try fetchMessageWindow(
            conversationID: conversationID,
            mode: .latestPage,
            pageSize: limit
        ).records
    }

    /// 最新一页 + pending；`hasMoreOlder` 表示库中是否还有更早的 seq 消息。
    public func fetchLatestMessageWindow(
        conversationID: String,
        pageSize: Int = MessageWindow.defaultPageSize
    ) throws -> MessageWindowPage {
        try fetchMessageWindow(conversationID: conversationID, mode: .latestPage, pageSize: pageSize)
    }

    /// 向上翻页：`conversation_seq` 严格小于 `beforeSeq` 的最近一页（升序返回，便于 prepend）。
    public func fetchOlderMessages(
        conversationID: String,
        beforeSeq: Int64,
        pageSize: Int = MessageWindow.defaultPageSize
    ) throws -> MessageWindowPage {
        try dbQueue.read { db in
            let slice = try MessageRecord.fetchAll(
                db,
                sql: """
                SELECT * FROM messages
                WHERE conversation_id = ?
                  AND conversation_seq IS NOT NULL
                  AND conversation_seq < ?
                ORDER BY conversation_seq DESC
                LIMIT ?
                """,
                arguments: [conversationID, beforeSeq, pageSize]
            )
            let ordered = slice.reversed()
            let oldest = ordered.compactMap(\.conversationSeq).min()
            let hasMore: Bool
            if let oldest {
                hasMore = try Bool.fetchOne(
                    db,
                    sql: """
                    SELECT EXISTS(
                      SELECT 1 FROM messages
                      WHERE conversation_id = ?
                        AND conversation_seq IS NOT NULL
                        AND conversation_seq < ?
                    )
                    """,
                    arguments: [conversationID, oldest]
                ) ?? false
            } else {
                hasMore = false
            }
            return MessageWindowPage(records: Array(ordered), hasMoreOlder: hasMore, oldestSeq: oldest)
        }
    }

    public func fetchMessageWindow(
        conversationID: String,
        mode: MessageWindowLoadMode,
        pageSize: Int = MessageWindow.defaultPageSize
    ) throws -> MessageWindowPage {
        try dbQueue.read { db in
            try Self.messageWindow(
                db: db,
                conversationID: conversationID,
                mode: mode,
                pageSize: pageSize
            )
        }
    }

    /// 供 ValueObservation 在同一 `Database` 上读取消息窗（避免嵌套 `dbQueue.read`）。
    public static func messageWindow(
        db: Database,
        conversationID: String,
        mode: MessageWindowLoadMode,
        pageSize: Int = MessageWindow.defaultPageSize
    ) throws -> MessageWindowPage {
        let pending = try fetchPendingInConversation(db: db, conversationID: conversationID)
        let seqRows: [MessageRecord]
        let hasMoreOlder: Bool
        let oldestSeq: Int64?

        switch mode {
        case .latestPage:
            let slice = try MessageRecord.fetchAll(
                db,
                sql: """
                SELECT * FROM messages
                WHERE conversation_id = ?
                  AND conversation_seq IS NOT NULL
                ORDER BY conversation_seq DESC
                LIMIT ?
                """,
                arguments: [conversationID, pageSize]
            )
            seqRows = Array(slice.reversed())
            oldestSeq = seqRows.compactMap(\.conversationSeq).min()
            if let oldestSeq {
                hasMoreOlder = try Bool.fetchOne(
                    db,
                    sql: """
                    SELECT EXISTS(
                      SELECT 1 FROM messages
                      WHERE conversation_id = ?
                        AND conversation_seq IS NOT NULL
                        AND conversation_seq < ?
                    )
                    """,
                    arguments: [conversationID, oldestSeq]
                ) ?? false
            } else {
                hasMoreOlder = false
            }
        case .fromSeq(let anchor):
            seqRows = try MessageRecord.fetchAll(
                db,
                sql: """
                SELECT * FROM messages
                WHERE conversation_id = ?
                  AND conversation_seq IS NOT NULL
                  AND conversation_seq >= ?
                ORDER BY conversation_seq ASC
                """,
                arguments: [conversationID, anchor]
            )
            oldestSeq = anchor
            hasMoreOlder = try Bool.fetchOne(
                db,
                sql: """
                SELECT EXISTS(
                  SELECT 1 FROM messages
                  WHERE conversation_id = ?
                    AND conversation_seq IS NOT NULL
                    AND conversation_seq < ?
                )
                """,
                arguments: [conversationID, anchor]
            ) ?? false
        }

        return MessageWindowPage(
            records: mergeDisplayOrder(seqRows: seqRows, pending: pending),
            hasMoreOlder: hasMoreOlder,
            oldestSeq: oldestSeq
        )
    }

    public static func conversationSummaries(db: Database, userID: Int64) throws -> [ConversationSummaryRecord] {
        // 对齐服务端 List：pinned 优先，再按最近消息时间（SQLite DESC 下 NULL 自然靠后）。
        try ConversationSummaryRecord
            .filter(Column("user_id") == userID)
            .order(Column("is_pinned").desc, Column("last_message_at").desc)
            .fetchAll(db)
    }

    /// 本地已落库的最大 conversation_seq（无消息为 0）。
    public func maxConversationSeq(conversationID: String) throws -> Int64 {
        try dbQueue.read { db in
            try Int64.fetchOne(
                db,
                sql: """
                SELECT COALESCE(MAX(conversation_seq), 0) FROM messages
                WHERE conversation_id = ? AND conversation_seq IS NOT NULL
                """,
                arguments: [conversationID]
            ) ?? 0
        }
    }

    /// 相邻 seq 缺口起点列表（如本地有 1,2,5 → 返回 3）。
    public func conversationSeqGapStarts(conversationID: String) throws -> [Int64] {
        try dbQueue.read { db in
            let seqs = try Int64.fetchAll(
                db,
                sql: """
                SELECT conversation_seq FROM messages
                WHERE conversation_id = ? AND conversation_seq IS NOT NULL
                ORDER BY conversation_seq ASC
                """,
                arguments: [conversationID]
            )
            guard let first = seqs.first else { return [] }
            var gaps: [Int64] = []
            if first > 1 {
                gaps.append(1)
            }
            for i in 1..<seqs.count {
                let prev = seqs[i - 1]
                let next = seqs[i]
                if next > prev + 1 {
                    gaps.append(prev + 1)
                }
            }
            return gaps
        }
    }

    private static func fetchPendingInConversation(db: Database, conversationID: String) throws -> [MessageRecord] {
        try MessageRecord.fetchAll(
            db,
            sql: """
            SELECT * FROM messages
            WHERE conversation_id = ?
              AND conversation_seq IS NULL
              AND status IN (?, ?)
            ORDER BY created_at ASC
            """,
            arguments: [
                conversationID,
                MessageStatus.queued.rawValue,
                MessageStatus.sending.rawValue,
            ]
        )
    }

    /// 已排序的 seq 消息 + pending 置底。
    private static func mergeDisplayOrder(seqRows: [MessageRecord], pending: [MessageRecord]) -> [MessageRecord] {
        seqRows + pending
    }

    public func fetchPendingSendRecords(limit: Int) throws -> [MessageRecord] {
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

    public func fetchConversationSummaryRecords(userID: Int64) throws -> [ConversationSummaryRecord] {
        try dbQueue.read { db in
            try LocalDatabase.conversationSummaries(db: db, userID: userID)
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
            try Self.upsertIncomingMessage(db: db, payload: payload)
        }
    }

    /// 批量落库投递帧（单事务），供 WS 突发路径使用。
    public func applyIncomingDeliveries(
        _ deliveries: [Livechat_Ws_WsMessageDelivery],
        myUserID: Int64
    ) throws -> [String] {
        try dbQueue.write { db in
            var touched = Set<String>()
            for delivery in deliveries {
                let payload = MessageCreatedPayload(
                    serverMessageID: delivery.serverMessageID,
                    conversationID: delivery.conversationID,
                    conversationSeq: Int64(delivery.conversationSeq),
                    senderUserID: Int64(delivery.senderUserID),
                    senderDeviceID: delivery.senderDeviceID.isEmpty ? nil : delivery.senderDeviceID,
                    messageType: delivery.messageType.isEmpty ? "text" : delivery.messageType,
                    content: delivery.content,
                    serverReceivedAtMs: delivery.serverReceivedAtMs == 0 ? nil : delivery.serverReceivedAtMs
                )
                try Self.upsertIncomingMessage(db: db, payload: payload)
                let preview: String = {
                    guard let data = payload.content.data(using: .utf8) else { return payload.content }
                    struct Body: Decodable { let text: String? }
                    if let body = try? JSONDecoder().decode(Body.self, from: data), let text = body.text {
                        return text
                    }
                    return payload.content
                }()
                let now = Int64(Date().timeIntervalSince1970 * 1000)
                let lastAt = payload.serverReceivedAtMs ?? now
                try db.execute(
                    sql: """
                    INSERT INTO conversation_summaries (
                      user_id, conversation_id, type, title, last_message_preview,
                      last_message_at, unread_count, is_pinned, is_muted, updated_at
                    ) VALUES (?, ?, 'direct', ?, ?, ?, ?, 0, 0, ?)
                    ON CONFLICT(user_id, conversation_id) DO UPDATE SET
                      title = COALESCE(excluded.title, conversation_summaries.title),
                      last_message_preview = excluded.last_message_preview,
                      last_message_at = excluded.last_message_at,
                      unread_count = CASE
                        WHEN ? = 0 THEN conversation_summaries.unread_count
                        ELSE conversation_summaries.unread_count + 1
                      END,
                      updated_at = excluded.updated_at
                    """,
                    arguments: [
                        myUserID,
                        payload.conversationID,
                        "user \(payload.senderUserID)",
                        preview,
                        lastAt,
                        payload.senderUserID == myUserID ? 0 : 1,
                        now,
                        payload.senderUserID == myUserID ? 0 : 1,
                    ]
                )
                touched.insert(payload.conversationID)
            }
            return Array(touched).sorted()
        }
    }

    private static func upsertIncomingMessage(db: Database, payload: MessageCreatedPayload) throws {
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
