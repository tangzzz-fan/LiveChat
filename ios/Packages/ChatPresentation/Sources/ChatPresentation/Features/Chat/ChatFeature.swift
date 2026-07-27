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
    /// 0057：多选模式；选中集合为 client_message_id。
    public var isMultiSelecting: Bool
    public var selectedClientMessageIDs: Set<String>
    /// 0058：转发目标会话选择器。
    public var isForwardPickerPresented: Bool
    public var pendingForwardIDs: [String]
    /// 0058：系统分享（文本或临时图片路径）。
    public var sharePresentation: SharePresentation?

    public enum SharePresentation: Equatable, Sendable, Identifiable {
        case text(String)
        case imageFilePath(String)

        public var id: String {
            switch self {
            case .text(let value): return "text:\(value.prefix(64))"
            case .imageFilePath(let path): return "img:\(path)"
            }
        }
    }

    public struct ConversationRow: Equatable, Sendable, Identifiable {
        public var id: String { conversationID }
        public let conversationID: String
        public let title: String
        public let preview: String
        public let unreadCount: Int
        public let lastMessageAt: Date?

        public init(
            conversationID: String,
            title: String,
            preview: String,
            unreadCount: Int = 0,
            lastMessageAt: Date? = nil
        ) {
            self.conversationID = conversationID
            self.title = title
            self.preview = preview
            self.unreadCount = unreadCount
            self.lastMessageAt = lastMessageAt
        }
    }

    public struct MessageRow: Equatable, Sendable, Identifiable {
        public var id: String { clientMessageID }
        public let clientMessageID: String
        public let serverMessageID: String?
        public let text: String
        public let status: String
        public let isMine: Bool
        public let messageType: String
        public let imageObjectKey: String?
        public let imageWidth: Int?
        public let imageHeight: Int?

        public init(
            clientMessageID: String,
            serverMessageID: String?,
            text: String,
            status: String,
            isMine: Bool,
            messageType: String = "text",
            imageObjectKey: String? = nil,
            imageWidth: Int? = nil,
            imageHeight: Int? = nil
        ) {
            self.clientMessageID = clientMessageID
            self.serverMessageID = serverMessageID
            self.text = text
            self.status = status
            self.isMine = isMine
            self.messageType = messageType
            self.imageObjectKey = imageObjectKey
            self.imageWidth = imageWidth
            self.imageHeight = imageHeight
        }

        public var isImage: Bool { messageType == "image" }
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
        pushTokenBanner: String? = nil,
        isMultiSelecting: Bool = false,
        selectedClientMessageIDs: Set<String> = [],
        isForwardPickerPresented: Bool = false,
        pendingForwardIDs: [String] = [],
        sharePresentation: SharePresentation? = nil
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
        self.isMultiSelecting = isMultiSelecting
        self.selectedClientMessageIDs = selectedClientMessageIDs
        self.isForwardPickerPresented = isForwardPickerPresented
        self.pendingForwardIDs = pendingForwardIDs
        self.sharePresentation = sharePresentation
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
    case sendImageTapped(Data, width: Int, height: Int, mimeType: String, fileName: String)
    case loadOlderMessagesTapped
    case retryQueuedTapped
    case copyMessageTapped(String)
    case deleteLocalMessageTapped(String)
    case retryMessageTapped(String)
    case cancelSendTapped(String)
    case enterMultiSelect(String)
    case toggleMessageSelection(String)
    case exitMultiSelect
    case batchDeleteSelectedTapped
    case forwardSelectedTapped
    case forwardMessageTapped(String)
    case dismissForwardPicker
    case confirmForwardToConversation(String)
    case shareMessageTapped(String)
    case shareSelectedTapped
    case presentShare(ChatState.SharePresentation)
    case clearSharePresentation
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
        case .openDirectTapped, .refreshConversationsTapped, .sendTapped, .sendImageTapped,
             .loadOlderMessagesTapped, .retryQueuedTapped,
             .copyMessageTapped, .deleteLocalMessageTapped, .retryMessageTapped,
             .cancelSendTapped, .batchDeleteSelectedTapped,
             .confirmForwardToConversation, .shareMessageTapped, .shareSelectedTapped,
             .syncTapped, .sceneBecameActive, .sceneBecameBackground,
             .realtimeEnsureStarted, .realtimeStop,
             .registerPushTokenTapped, .pushTokenReceived, .silentPushWakeTapped, .silentPushWake:
            break
        case .setPeerUserIDInput(let value):
            state.peerUserIDInput = value
            state.errorMessage = nil
        case .selectConversation(let id):
            // 先清空窗口，避免 List onAppear 在脏数据/空数据上误滚，
            // 并保证后续 conversationOpened 的 last.id 变化一定触发 onChange。
            state.activeConversationID = id
            state.visibleMessages = []
            state.oldestLoadedSeq = nil
            state.hasMoreOlder = false
            state.errorMessage = nil
            ChatFeature.clearSelectionAndForward(&state)
        case .leaveConversation:
            state.activeConversationID = nil
            state.visibleMessages = []
            state.oldestLoadedSeq = nil
            state.hasMoreOlder = false
            state.composeDraft = ""
            ChatFeature.clearSelectionAndForward(&state)
        case .enterMultiSelect(let clientMessageID):
            state.isMultiSelecting = true
            state.selectedClientMessageIDs = [clientMessageID]
            state.errorMessage = nil
        case .toggleMessageSelection(let clientMessageID):
            guard state.isMultiSelecting else { return }
            if state.selectedClientMessageIDs.contains(clientMessageID) {
                state.selectedClientMessageIDs.remove(clientMessageID)
            } else {
                state.selectedClientMessageIDs.insert(clientMessageID)
            }
        case .exitMultiSelect:
            state.isMultiSelecting = false
            state.selectedClientMessageIDs = []
        case .forwardSelectedTapped:
            let ordered = state.visibleMessages
                .map(\.clientMessageID)
                .filter { state.selectedClientMessageIDs.contains($0) }
            guard !ordered.isEmpty else { return }
            state.pendingForwardIDs = ordered
            state.isForwardPickerPresented = true
            state.errorMessage = nil
        case .forwardMessageTapped(let clientMessageID):
            state.pendingForwardIDs = [clientMessageID]
            state.isForwardPickerPresented = true
            state.errorMessage = nil
        case .dismissForwardPicker:
            state.isForwardPickerPresented = false
            state.pendingForwardIDs = []
        case .presentShare(let payload):
            state.sharePresentation = payload
        case .clearSharePresentation:
            state.sharePresentation = nil
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
            ChatFeature.clearSelectionAndForward(&state)
        case .visibleMessagesUpdated(let messages, let oldest, let hasMore):
            state.visibleMessages = messages
            state.oldestLoadedSeq = oldest
            state.hasMoreOlder = hasMore
            state.isBusy = false
            // 多选中若消息被删，收敛选中集。
            if state.isMultiSelecting {
                let visible = Set(messages.map(\.clientMessageID))
                state.selectedClientMessageIDs = state.selectedClientMessageIDs.intersection(visible)
            }
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

    private static func clearSelectionAndForward(_ state: inout ChatState) {
        state.isMultiSelecting = false
        state.selectedClientMessageIDs = []
        state.isForwardPickerPresented = false
        state.pendingForwardIDs = []
    }
}
