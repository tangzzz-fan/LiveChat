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
        try conversations.upsertConversationSummary(
            ConversationSummary(
                userID: myUserID,
                conversationID: payload.conversationID,
                type: "direct",
                title: "user \(payload.senderUserID)",
                lastMessagePreview: preview,
                lastMessageAt: at,
                unreadCount: payload.senderUserID == myUserID ? 0 : 1
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

    private static func previewText(from content: String) -> String {
        guard let data = content.data(using: .utf8) else { return content }
        struct Payload: Decodable { let text: String? }
        if let payload = try? JSONDecoder().decode(Payload.self, from: data), let text = payload.text {
            return text
        }
        return content
    }
}
