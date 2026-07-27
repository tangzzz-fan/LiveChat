import Foundation
import ChatDomain

/// 把 sync / WS 投递统一落到本地 Store，避免双写竞态。
public enum IncomingMessageApplier {
    public static func applyMessageCreated(
        _ payload: MessageCreatedPayload,
        myUserID: Int64,
        messages: any MessageStore,
        conversations: any ConversationStore
    ) throws {
        try messages.upsertRemoteMessage(
            serverMessageID: payload.serverMessageID,
            conversationID: payload.conversationID,
            conversationSeq: payload.conversationSeq,
            senderUserID: payload.senderUserID,
            messageType: payload.messageType,
            content: payload.content,
            serverReceivedAtMs: payload.serverReceivedAtMs
        )
        let preview = previewText(from: payload.content)
        let at: Date? = payload.serverReceivedAtMs.map {
            Date(timeIntervalSince1970: Double($0) / 1000)
        }
        let existing = try conversations.fetchConversationSummaries(userID: myUserID)
            .first(where: { $0.conversationID == payload.conversationID })
        let unread: Int
        if payload.senderUserID == myUserID {
            unread = existing?.unreadCount ?? 0
        } else {
            unread = (existing?.unreadCount ?? 0) + 1
        }
        try conversations.upsertConversationSummary(
            ConversationSummary(
                userID: myUserID,
                conversationID: payload.conversationID,
                type: existing?.type ?? "direct",
                title: existing?.title ?? "user \(payload.senderUserID)",
                lastMessagePreview: preview,
                lastMessageAt: at ?? existing?.lastMessageAt,
                unreadCount: unread,
                isPinned: existing?.isPinned ?? false,
                isMuted: existing?.isMuted ?? false
            )
        )
    }

    public static func applyDelivery(
        _ delivery: Livechat_Ws_WsMessageDelivery,
        myUserID: Int64,
        messages: any MessageStore,
        conversations: any ConversationStore
    ) throws {
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
        try applyMessageCreated(
            payload,
            myUserID: myUserID,
            messages: messages,
            conversations: conversations
        )
    }

    public static func applyMessageRead(
        conversationID: String,
        lastReadSeq: Int64,
        myUserID: Int64,
        messages: any MessageStore
    ) throws {
        guard lastReadSeq > 0 else { return }
        try messages.markOwnMessagesRead(
            conversationID: conversationID,
            upToSeq: lastReadSeq,
            myUserID: myUserID
        )
    }

    public static func applyConversationUpdated(
        conversationID: String,
        unreadCount: Int?,
        myUserID: Int64,
        conversations: any ConversationStore
    ) throws {
        if unreadCount == 0 {
            try conversations.clearUnread(userID: myUserID, conversationID: conversationID)
            return
        }
        guard let unreadCount else { return }
        let existing = try conversations.fetchConversationSummaries(userID: myUserID)
            .first(where: { $0.conversationID == conversationID })
        guard let existing else { return }
        try conversations.upsertConversationSummary(
            ConversationSummary(
                userID: existing.userID,
                conversationID: existing.conversationID,
                type: existing.type,
                title: existing.title,
                lastMessagePreview: existing.lastMessagePreview,
                lastMessageAt: existing.lastMessageAt,
                unreadCount: unreadCount,
                isPinned: existing.isPinned,
                isMuted: existing.isMuted
            )
        )
    }

    private static func previewText(from content: String) -> String {
        let text = TextMessageContent.parseText(from: content)
        if text != content { return text }
        if ImageMessageContent.parseObjectKey(from: content) != nil { return "[图片]" }
        return content
    }
}
