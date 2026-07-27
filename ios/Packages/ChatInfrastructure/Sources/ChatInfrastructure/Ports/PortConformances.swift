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
}

extension LocalDatabase: SyncCursorStore {}

extension LocalDatabase: ConversationStore {
    public func fetchConversationSummaries(userID: Int64) throws -> [ConversationSummary] {
        try fetchConversationSummaryRecords(userID: userID).map { $0.toDomain() }
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
