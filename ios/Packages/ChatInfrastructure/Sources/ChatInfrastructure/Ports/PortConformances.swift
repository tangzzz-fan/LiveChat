import Foundation
import ChatDomain

// MARK: - LocalDatabase → Store ports

extension LocalDatabase: MessageStore {
    public func fetchPendingSend(limit: Int) throws -> [Message] {
        try fetchPendingSendRecords(limit: limit).map { $0.toDomainMessage() }
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
        try upsertIncomingMessage(
            from: MessageCreatedPayload(
                serverMessageID: serverMessageID,
                conversationID: conversationID,
                conversationSeq: conversationSeq,
                senderUserID: senderUserID,
                messageType: messageType,
                content: content,
                serverReceivedAtMs: serverReceivedAtMs
            )
        )
    }

    public func markOwnMessagesRead(conversationID: String, upToSeq: Int64, myUserID: Int64) throws {
        try dbQueue.write { db in
            try db.execute(
                sql: """
                UPDATE messages SET status = ?
                WHERE conversation_id = ?
                  AND sender_user_id = ?
                  AND conversation_seq IS NOT NULL
                  AND conversation_seq <= ?
                  AND status IN (?, ?)
                """,
                arguments: [
                    MessageStatus.read.rawValue,
                    conversationID,
                    myUserID,
                    upToSeq,
                    MessageStatus.accepted.rawValue,
                    MessageStatus.delivered.rawValue,
                ]
            )
        }
    }
}

extension LocalDatabase: SyncCursorStore {}

extension LocalDatabase: ConversationStore {
    public func fetchConversationSummaries(userID: Int64) throws -> [ConversationSummary] {
        try fetchConversationSummaryRecords(userID: userID).map { $0.toDomain() }
    }

    public func clearUnread(userID: Int64, conversationID: String) throws {
        try dbQueue.write { db in
            try db.execute(
                sql: """
                UPDATE conversation_summaries
                SET unread_count = 0, updated_at = ?
                WHERE user_id = ? AND conversation_id = ?
                """,
                arguments: [
                    Int64(Date().timeIntervalSince1970 * 1000),
                    userID,
                    conversationID,
                ]
            )
        }
    }
}

// MARK: - API → Remote ports

extension MessageAPI: MessageRemote {
    public func sendMessage(_ request: SendMessageRequest) async throws -> SendMessageResponse {
        try await send(request)
    }
}

extension SyncAPI: SyncRemote {}

extension ConversationAPI: ConversationRemote {}

// MARK: - Record → Domain

extension MessageRecord {
    func toDomainMessage() -> Message {
        Message(
            serverMessageID: serverMessageID,
            clientMessageID: clientMessageID,
            conversationID: conversationID,
            conversationSeq: conversationSeq,
            senderUserID: senderUserID ?? 0,
            messageType: messageType,
            content: content ?? "",
            status: MessageStatus(rawValue: status) ?? .queued,
            serverReceivedAt: serverReceivedAt.map {
                Date(timeIntervalSince1970: Double($0) / 1000)
            },
            createdAt: Date(timeIntervalSince1970: Double(createdAt) / 1000)
        )
    }
}

extension ConversationSummaryRecord {
    func toDomain() -> ConversationSummary {
        ConversationSummary(
            userID: userID,
            conversationID: conversationID,
            type: type,
            title: title,
            lastMessagePreview: lastMessagePreview,
            lastMessageAt: lastMessageAt.map {
                Date(timeIntervalSince1970: Double($0) / 1000)
            },
            unreadCount: unreadCount,
            isPinned: isPinned,
            isMuted: isMuted
        )
    }
}
