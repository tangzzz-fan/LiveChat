/// 消息状态机（Spec 13 §5.1）
public enum MessageStatus: String, CaseIterable, Codable, Sendable {
    case draft
    case queued
    case sending
    case accepted
    case delivered
    case read
    case failed

    public var allowedTransitions: Set<MessageStatus> {
        switch self {
        case .draft: return [.queued]
        case .queued: return [.sending]
        // queued：sending 超时 / 进程重启后收回，避免永久卡死（高负载 #5）
        case .sending: return [.accepted, .failed, .queued]
        case .accepted: return [.delivered]
        case .delivered: return [.read]
        case .read: return []
        case .failed: return [.queued]
        }
    }

    public func canTransition(to target: MessageStatus) -> Bool {
        allowedTransitions.contains(target)
    }
}
