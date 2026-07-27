import Foundation
import TGReduxKit

/// 聊天视图真相：列表摘要 + 当前会话可见窗口（不是 DB 全量）。
public struct ChatState: Equatable, Sendable {
    public var peerUserIDInput: String
    public var conversationRows: [ConversationRow]
    public var activeConversationID: String?
    public var visibleMessages: [MessageRow]
    public var composeDraft: String
    public var isBusy: Bool
    public var errorMessage: String?
    public var connectionBanner: String?
    public var syncBanner: String?
    public var isSyncing: Bool

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
        composeDraft: String = "",
        isBusy: Bool = false,
        errorMessage: String? = nil,
        connectionBanner: String? = nil,
        syncBanner: String? = nil,
        isSyncing: Bool = false
    ) {
        self.peerUserIDInput = peerUserIDInput
        self.conversationRows = conversationRows
        self.activeConversationID = activeConversationID
        self.visibleMessages = visibleMessages
        self.composeDraft = composeDraft
        self.isBusy = isBusy
        self.errorMessage = errorMessage
        self.connectionBanner = connectionBanner
        self.syncBanner = syncBanner
        self.isSyncing = isSyncing
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
    case retryQueuedTapped
    case syncTapped
    case sceneBecameActive

    case busy(Bool)
    case failed(String)
    case conversationsLoaded([ChatState.ConversationRow])
    case conversationOpened(id: String, messages: [ChatState.MessageRow])
    case visibleMessagesUpdated([ChatState.MessageRow])
    case setConnectionBanner(String?)
    case syncStarted
    case syncFinished(applied: Int, cursor: Int64)
    case reset
}

@MainActor
public enum ChatFeature {
    public static let reducer: Reducer<ChatState, ChatAction> = { state, action in
        switch action {
        case .openDirectTapped, .refreshConversationsTapped, .sendTapped, .retryQueuedTapped,
             .syncTapped, .sceneBecameActive:
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
        case .conversationOpened(let id, let messages):
            state.activeConversationID = id
            state.visibleMessages = messages
            state.isBusy = false
            state.errorMessage = nil
        case .visibleMessagesUpdated(let messages):
            state.visibleMessages = messages
            state.isBusy = false
        case .setConnectionBanner(let banner):
            state.connectionBanner = banner
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
