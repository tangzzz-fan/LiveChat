import Foundation
import TGReduxKit

/// 聊天视图真相：列表摘要 + 当前会话可见窗口（不是 DB 全量）。
public struct ChatState: Equatable, Sendable {
    public var peerUserIDInput: String
    public var conversationRows: [ConversationRow]
    public var activeConversationID: String?
    public var visibleMessages: [MessageRow]
    /// 当前窗口内已加载的最小 `conversation_seq`（无 seq 的 pending 不计）。
    public var oldestLoadedSeq: Int64?
    public var hasMoreOlder: Bool
    public var composeDraft: String
    public var isBusy: Bool
    public var errorMessage: String?
    public var connectionBanner: String?
    public var syncBanner: String?
    public var isSyncing: Bool
    public var pushTokenBanner: String?

    public struct ConversationRow: Equatable, Sendable, Identifiable {
        public var id: String { conversationID }
        public let conversationID: String
        public let title: String
        public let preview: String
    }

    public struct MessageRow: Equatable, Sendable, Identifiable {
        public var id: String { clientMessageID }
        public let clientMessageID: String
        public let serverMessageID: String?
        public let text: String
        public let status: String
        public let isMine: Bool
    }

    public init(
        peerUserIDInput: String = "",
        conversationRows: [ConversationRow] = [],
        activeConversationID: String? = nil,
        visibleMessages: [MessageRow] = [],
        oldestLoadedSeq: Int64? = nil,
        hasMoreOlder: Bool = false,
        composeDraft: String = "",
        isBusy: Bool = false,
        errorMessage: String? = nil,
        connectionBanner: String? = nil,
        syncBanner: String? = nil,
        isSyncing: Bool = false,
        pushTokenBanner: String? = nil
    ) {
        self.peerUserIDInput = peerUserIDInput
        self.conversationRows = conversationRows
        self.activeConversationID = activeConversationID
        self.visibleMessages = visibleMessages
        self.oldestLoadedSeq = oldestLoadedSeq
        self.hasMoreOlder = hasMoreOlder
        self.composeDraft = composeDraft
        self.isBusy = isBusy
        self.errorMessage = errorMessage
        self.connectionBanner = connectionBanner
        self.syncBanner = syncBanner
        self.isSyncing = isSyncing
        self.pushTokenBanner = pushTokenBanner
    }
}

public enum ChatAction: Sendable {
    case setPeerUserIDInput(String)
    case openDirectTapped
    case refreshConversationsTapped
    case selectConversation(String)
    case leaveConversation
    case updateDraft(String)
    case sendTapped
    case loadOlderMessagesTapped
    case retryQueuedTapped
    case syncTapped
    case sceneBecameActive
    case sceneBecameBackground
    case realtimeEnsureStarted
    case realtimeStop
    case registerPushTokenTapped
    case pushTokenReceived(String)
    case silentPushWakeTapped
    case silentPushWake(reason: String)

    case busy(Bool)
    case failed(String)
    case conversationsLoaded([ChatState.ConversationRow])
    case conversationOpened(id: String, messages: [ChatState.MessageRow], oldestLoadedSeq: Int64?, hasMoreOlder: Bool)
    case visibleMessagesUpdated([ChatState.MessageRow], oldestLoadedSeq: Int64?, hasMoreOlder: Bool)
    case olderMessagesPrepended([ChatState.MessageRow], oldestLoadedSeq: Int64?, hasMoreOlder: Bool)
    case setConnectionBanner(String?)
    case setPushTokenBanner(String?)
    case syncStarted
    case syncFinished(applied: Int, cursor: Int64)
    case reset
}

@MainActor
public enum ChatFeature {
    public static let reducer: Reducer<ChatState, ChatAction> = { state, action in
        switch action {
        case .openDirectTapped, .refreshConversationsTapped, .sendTapped, .loadOlderMessagesTapped, .retryQueuedTapped,
             .syncTapped, .sceneBecameActive, .sceneBecameBackground,
             .realtimeEnsureStarted, .realtimeStop,
             .registerPushTokenTapped, .pushTokenReceived, .silentPushWakeTapped, .silentPushWake:
            break
        case .setPeerUserIDInput(let value):
            state.peerUserIDInput = value
            state.errorMessage = nil
        case .selectConversation(let id):
            state.activeConversationID = id
            state.errorMessage = nil
        case .leaveConversation:
            state.activeConversationID = nil
            state.visibleMessages = []
            state.oldestLoadedSeq = nil
            state.hasMoreOlder = false
            state.composeDraft = ""
        case .updateDraft(let text):
            state.composeDraft = text
        case .busy(let busy):
            state.isBusy = busy
        case .failed(let message):
            state.errorMessage = message
            state.isBusy = false
            state.isSyncing = false
        case .conversationsLoaded(let rows):
            state.conversationRows = rows
            state.isBusy = false
        case .conversationOpened(let id, let messages, let oldest, let hasMore):
            state.activeConversationID = id
            state.visibleMessages = messages
            state.oldestLoadedSeq = oldest
            state.hasMoreOlder = hasMore
            state.isBusy = false
            state.errorMessage = nil
        case .visibleMessagesUpdated(let messages, let oldest, let hasMore):
            state.visibleMessages = messages
            state.oldestLoadedSeq = oldest
            state.hasMoreOlder = hasMore
            state.isBusy = false
        case .olderMessagesPrepended(let older, let oldest, let hasMore):
            state.visibleMessages = older + state.visibleMessages
            state.oldestLoadedSeq = oldest
            state.hasMoreOlder = hasMore
            state.isBusy = false
        case .setConnectionBanner(let banner):
            state.connectionBanner = banner
        case .setPushTokenBanner(let banner):
            state.pushTokenBanner = banner
        case .syncStarted:
            state.isSyncing = true
            state.syncBanner = "同步中…"
            state.errorMessage = nil
        case .syncFinished(let applied, let cursor):
            state.isSyncing = false
            state.syncBanner = "已同步 +\(applied) · cursor \(cursor)"
        case .reset:
            state = ChatState()
        }
    }
}
