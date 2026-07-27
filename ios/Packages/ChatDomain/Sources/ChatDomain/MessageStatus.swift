/// 消息状态机（Spec 13 §5.1）
public enum MessageStatus: String, CaseIterable, Codable, Sendable {
    case draft
    case queued
    case sending
    case accepted
    case delivered
    case read
    case failed
    /// 用户主动取消（0055）；终态，不自动续跑。与 0046 超时回 `queued` 区分。
    case cancelled

    public var allowedTransitions: Set<MessageStatus> {
        switch self {
        case .draft: return [.queued]
        case .queued: return [.sending, .cancelled]
        // queued：sending 超时 / 进程重启后收回，避免永久卡死（高负载 #5）
        // cancelled：用户取消（0055）
        case .sending: return [.accepted, .failed, .queued, .cancelled]
        // 已读回执可从 accepted 直接收敛到 read（跳过 delivered）
        case .accepted: return [.delivered, .read]
        case .delivered: return [.read]
        case .read: return []
        case .failed: return [.queued]
        case .cancelled: return []
        }
    }

    public func canTransition(to target: MessageStatus) -> Bool {
        allowedTransitions.contains(target)
    }

    /// 气泡上需要「发送中」指示（spinner）。
    public var showsSendingIndicator: Bool {
        self == .queued || self == .sending
    }

    /// 允许用户取消（未 accepted）。
    public var canCancelSend: Bool {
        self == .queued || self == .sending
    }
}
