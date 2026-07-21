import Foundation

/// 统一远程事件入口（Spec 13 §7.1）
/// WebSocket / Push / Sync 三种来源都先转为 RemoteEvent，再统一处理
public enum RemoteEvent {
    case messageDelivery(Message)
    case messageStatusUpdate(clientMessageID: String, status: MessageStatus)
    case syncTrigger(latestEventSeq: Int64)
    case conversationUpdate(ConversationSummary)
    case groupEvent(groupID: String, eventType: String, actorUserID: Int64, targetUserID: Int64?)
}
