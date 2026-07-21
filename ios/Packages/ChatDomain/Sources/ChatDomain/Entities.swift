import Foundation

// MARK: - Entities (Spec 13 §4.1)

/// 聊天系统中的账号主体
public struct User: Equatable, Codable {
    public let id: Int64
    public var phoneE164: String
    public var displayName: String

    public init(id: Int64, phoneE164: String, displayName: String) {
        self.id = id
        self.phoneE164 = phoneE164
        self.displayName = displayName
    }
}

/// 用户登录态承载单元
public struct Device: Equatable, Codable {
    public let id: String
    public let userID: Int64
    public var platform: String
    public var pushToken: String?
    public var sessionVersion: Int
    public var lastSeenAt: Date

    public init(id: String, userID: Int64, platform: String, pushToken: String? = nil, sessionVersion: Int = 1, lastSeenAt: Date = Date()) {
        self.id = id
        self.userID = userID
        self.platform = platform
        self.pushToken = pushToken
        self.sessionVersion = sessionVersion
        self.lastSeenAt = lastSeenAt
    }
}

/// 会话中的一条业务消息
public struct Message: Equatable, Codable {
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

    public init(serverMessageID: String? = nil, clientMessageID: String, conversationID: String, conversationSeq: Int64? = nil, senderUserID: Int64, messageType: String, content: String, status: MessageStatus = .queued, serverReceivedAt: Date? = nil, createdAt: Date = Date()) {
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

/// 会话摘要投影
public struct ConversationSummary: Equatable, Codable {
    public let userID: Int64
    public let conversationID: String
    public var type: String
    public var title: String?
    public var lastMessagePreview: String?
    public var lastMessageAt: Date?
    public var unreadCount: Int
    public var isPinned: Bool
    public var isMuted: Bool
    public var isHidden: Bool

    public init(userID: Int64, conversationID: String, type: String = "direct", title: String? = nil, lastMessagePreview: String? = nil, lastMessageAt: Date? = nil, unreadCount: Int = 0, isPinned: Bool = false, isMuted: Bool = false, isHidden: Bool = false) {
        self.userID = userID
        self.conversationID = conversationID
        self.type = type
        self.title = title
        self.lastMessagePreview = lastMessagePreview
        self.lastMessageAt = lastMessageAt
        self.unreadCount = unreadCount
        self.isPinned = isPinned
        self.isMuted = isMuted
        self.isHidden = isHidden
    }
}

/// 同步游标
public struct SyncCursor: Equatable, Codable {
    public let userID: Int64
    public let deviceID: String
    public var lastEventSeq: Int64
    public var lastSyncAt: Date?

    public init(userID: Int64, deviceID: String, lastEventSeq: Int64 = 0, lastSyncAt: Date? = nil) {
        self.userID = userID
        self.deviceID = deviceID
        self.lastEventSeq = lastEventSeq
        self.lastSyncAt = lastSyncAt
    }
}

/// 媒体附件元数据
public struct Attachment: Equatable, Codable {
    public let id: Int64?
    public var objectKey: String
    public var mimeType: String
    public var sizeBytes: Int64
    public var width: Int?
    public var height: Int?
    public var thumbnailKey: String?
    public var uploadStatus: UploadStatus

    public enum UploadStatus: String, Codable {
        case pending, processing, complete, failed, orphan
    }

    public init(id: Int64? = nil, objectKey: String, mimeType: String, sizeBytes: Int64, width: Int? = nil, height: Int? = nil, thumbnailKey: String? = nil, uploadStatus: UploadStatus = .pending) {
        self.id = id
        self.objectKey = objectKey
        self.mimeType = mimeType
        self.sizeBytes = sizeBytes
        self.width = width
        self.height = height
        self.thumbnailKey = thumbnailKey
        self.uploadStatus = uploadStatus
    }
}
