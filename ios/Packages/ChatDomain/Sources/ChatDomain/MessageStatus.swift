/// 消息状态机（Spec 13 §5.1）
public enum MessageStatus: String, CaseIterable, Codable {
    case draft
    case queued
    case sending
    case accepted
    case delivered
    case read
    case failed

    /// 每个状态允许转移到的目标状态集合
    public var allowedTransitions: Set<MessageStatus> {
        switch self {
        case .draft:     return [.queued]
        case .queued:    return [.sending]
        case .sending:   return [.accepted, .failed]
        case .accepted:  return [.delivered]
        case .delivered: return [.read]
        case .read:      return []
        case .failed:    return [.queued]  // retry
        }
    }

    /// 检查是否可以从当前状态转移到目标状态
    public func canTransition(to target: MessageStatus) -> Bool {
        allowedTransitions.contains(target)
    }
}
