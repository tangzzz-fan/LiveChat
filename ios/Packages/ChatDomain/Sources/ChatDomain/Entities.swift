import Foundation

public struct User: Equatable, Codable, Sendable {
    public let id: Int64
    public var phoneE164: String
    public var displayName: String

    public init(id: Int64, phoneE164: String, displayName: String) {
        self.id = id
        self.phoneE164 = phoneE164
        self.displayName = displayName
    }
}

public struct Device: Equatable, Codable, Sendable {
    public let id: String
    public let userID: Int64
    public var platform: String
    public var pushToken: String?
    public var sessionVersion: Int

    public init(
        id: String,
        userID: Int64,
        platform: String = "ios",
        pushToken: String? = nil,
        sessionVersion: Int = 1
    ) {
        self.id = id
        self.userID = userID
        self.platform = platform
        self.pushToken = pushToken
        self.sessionVersion = sessionVersion
    }
}

public struct Message: Equatable, Codable, Sendable {
    public let serverMessageID: String?
    public let clientMessageID: String
    public let conversationID: String
    public var conversationSeq: Int64?
    public let senderUserID: Int64
    public let messageType: String
    public let content: String
    public var status: MessageStatus
    public var serverReceivedAt: Date?
    public let createdAt: Date

    public init(
        serverMessageID: String? = nil,
        clientMessageID: String,
        conversationID: String,
        conversationSeq: Int64? = nil,
        senderUserID: Int64,
        messageType: String = "text",
        content: String,
        status: MessageStatus = .queued,
        serverReceivedAt: Date? = nil,
        createdAt: Date = Date()
    ) {
        self.serverMessageID = serverMessageID
        self.clientMessageID = clientMessageID
        self.conversationID = conversationID
        self.conversationSeq = conversationSeq
        self.senderUserID = senderUserID
        self.messageType = messageType
        self.content = content
        self.status = status
        self.serverReceivedAt = serverReceivedAt
        self.createdAt = createdAt
    }
}

public struct ConversationSummary: Equatable, Codable, Sendable {
    public let userID: Int64
    public let conversationID: String
    public var type: String
    public var title: String?
    public var lastMessagePreview: String?
    public var lastMessageAt: Date?
    public var unreadCount: Int
    public var isPinned: Bool
    public var isMuted: Bool

    public init(
        userID: Int64,
        conversationID: String,
        type: String = "direct",
        title: String? = nil,
        lastMessagePreview: String? = nil,
        lastMessageAt: Date? = nil,
        unreadCount: Int = 0,
        isPinned: Bool = false,
        isMuted: Bool = false
    ) {
        self.userID = userID
        self.conversationID = conversationID
        self.type = type
        self.title = title
        self.lastMessagePreview = lastMessagePreview
        self.lastMessageAt = lastMessageAt
        self.unreadCount = unreadCount
        self.isPinned = isPinned
        self.isMuted = isMuted
    }
}

public struct Attachment: Equatable, Codable, Sendable {
    public let objectKey: String
    public let mimeType: String
    public let sizeBytes: Int64
    public let width: Int?
    public let height: Int?

    public init(
        objectKey: String,
        mimeType: String,
        sizeBytes: Int64,
        width: Int? = nil,
        height: Int? = nil
    ) {
        self.objectKey = objectKey
        self.mimeType = mimeType
        self.sizeBytes = sizeBytes
        self.width = width
        self.height = height
    }
}
