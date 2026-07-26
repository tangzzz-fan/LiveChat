import Foundation

public protocol MessageRepository: Sendable {
    func getMessages(in conversationID: String, afterSeq: Int64?, limit: Int) async throws -> [Message]
    func insertMessage(_ message: Message) async throws
    func updateMessageStatus(clientMessageID: String, status: MessageStatus) async throws
    func sendMessage(_ request: SendMessageRequest) async throws -> SendMessageResponse
}

public struct SendMessageRequest: Codable, Sendable {
    public let clientMessageID: String
    public let conversationID: String
    public let messageType: String
    public let content: String

    public init(
        clientMessageID: String,
        conversationID: String,
        messageType: String = "text",
        content: String
    ) {
        self.clientMessageID = clientMessageID
        self.conversationID = conversationID
        self.messageType = messageType
        self.content = content
    }
}

public struct SendMessageResponse: Codable, Sendable {
    public let serverMessageID: String
    public let conversationSeq: Int64
    public let isDuplicate: Bool
    public let serverReceivedAtMs: Int64

    public init(
        serverMessageID: String,
        conversationSeq: Int64,
        isDuplicate: Bool,
        serverReceivedAtMs: Int64
    ) {
        self.serverMessageID = serverMessageID
        self.conversationSeq = conversationSeq
        self.isDuplicate = isDuplicate
        self.serverReceivedAtMs = serverReceivedAtMs
    }
}

public protocol ConversationRepository: Sendable {
    func getConversations() async throws -> [ConversationSummary]
    func upsertConversation(_ conversation: ConversationSummary) async throws
}

public protocol SyncRepository: Sendable {
    func getSyncCursor() async throws -> Int64
    func updateSyncCursor(_ seq: Int64) async throws
    func fetchEvents(from cursor: Int64) async throws -> SyncResponse
}

public struct SyncEvent: Codable, Sendable {
    public let eventSeq: Int64
    public let userID: Int64
    public let conversationID: String?
    public let eventType: String
    public let payload: String
}

public struct SyncResponse: Codable, Sendable {
    public let events: [SyncEvent]
    public let hasMore: Bool
    public let latestEventSeq: Int64
}

public protocol AuthRepository: Sendable {
    func requestCode(phone: String) async throws -> CodeRequestResponse
    func verifyCode(
        phone: String,
        code: String,
        deviceID: String,
        platform: String
    ) async throws -> AuthTokens
    func refreshToken(_ token: String) async throws -> AuthTokens
    func logout() async throws
}

public struct CodeRequestResponse: Codable, Sendable {
    public let retryAfterSec: Int
    public let expiresInSec: Int

    public init(retryAfterSec: Int, expiresInSec: Int) {
        self.retryAfterSec = retryAfterSec
        self.expiresInSec = expiresInSec
    }
}

public struct AuthTokens: Codable, Sendable {
    public let accessToken: String
    public let refreshToken: String
    public let userID: Int64
    public let expiresIn: Int64

    public init(accessToken: String, refreshToken: String, userID: Int64, expiresIn: Int64) {
        self.accessToken = accessToken
        self.refreshToken = refreshToken
        self.userID = userID
        self.expiresIn = expiresIn
    }
}

public protocol PushRepository: Sendable {
    func registerPushToken(_ token: Data) async throws
    func handleRemoteNotification(_ userInfo: [AnyHashable: Any]) async
}

/// 应用层协议帧（protobuf 编解码在 Infrastructure）
public struct WebSocketFrame: Sendable {
    public let opcode: UInt32
    public let payload: Data

    public init(opcode: UInt32, payload: Data = Data()) {
        self.opcode = opcode
        self.payload = payload
    }
}

public protocol WebSocketRepository: Sendable {
    func connect() async throws
    func disconnect() async
    func sendFrame(_ frame: WebSocketFrame) async throws
    var messageStream: AsyncStream<WebSocketFrame> { get }
}

public protocol MediaRepository: Sendable {
    func uploadImage(_ data: Data, metadata: ImageMetadata) async throws -> Attachment
    func downloadImage(objectKey: String) async throws -> Data
}

public struct ImageMetadata: Codable, Sendable {
    public let mimeType: String
    public let sizeBytes: Int64
    public let fileName: String
    public let width: Int
    public let height: Int

    public init(mimeType: String, sizeBytes: Int64, fileName: String, width: Int, height: Int) {
        self.mimeType = mimeType
        self.sizeBytes = sizeBytes
        self.fileName = fileName
        self.width = width
        self.height = height
    }
}
