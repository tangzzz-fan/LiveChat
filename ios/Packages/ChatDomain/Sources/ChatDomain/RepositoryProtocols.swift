import Foundation

// MARK: - Repository Protocols (Spec 13 §6)

public protocol MessageRepository {
    func getMessages(in conversationID: String, limit: Int) async throws -> [Message]
    func sendMessage(_ request: SendMessageRequest) async throws -> SendMessageResponse
    func insertMessage(_ message: Message) async throws
    func updateMessageStatus(clientMessageID: String, status: MessageStatus) async throws
}

public struct SendMessageRequest: Codable {
    public let clientMessageID: String
    public let conversationID: String
    public let messageType: String
    public let content: String

    public init(clientMessageID: String, conversationID: String, messageType: String = "text", content: String) {
        self.clientMessageID = clientMessageID
        self.conversationID = conversationID
        self.messageType = messageType
        self.content = content
    }
}

public struct SendMessageResponse: Codable {
    public let serverMessageID: String
    public let conversationSeq: Int64
    public let isDuplicate: Bool
    public let serverReceivedAtMs: Int64
}

public protocol ConversationRepository {
    func getConversations() async throws -> [ConversationSummary]
    func getConversation(id: String) async throws -> ConversationSummary?
    func upsertConversation(_ conversation: ConversationSummary) async throws
}

public protocol SyncRepository {
    func getSyncCursor() async throws -> Int64
    func updateSyncCursor(_ seq: Int64) async throws
    func fetchEvents(from cursor: Int64) async throws -> SyncResponse
}

public struct SyncEvent: Codable {
    public let eventSeq: Int64
    public let userID: Int64
    public let conversationID: String?
    public let eventType: String
    public let payload: String  // JSON-encoded
    public let createdAt: String
}

public struct SyncResponse: Codable {
    public let events: [SyncEvent]
    public let hasMore: Bool
    public let latestEventSeq: Int64
}

public protocol PushRepository {
    func registerPushToken(_ token: Data) async throws
    func handleRemoteNotification(_ userInfo: [AnyHashable: Any]) async
}

public protocol WebSocketRepository {
    func connect() async throws
    func disconnect() async
    func sendFrame(_ frame: WebSocketFrame) async throws
    var messageStream: AsyncStream<WebSocketFrame> { get }
}

public struct WebSocketFrame: Codable {
    public let opcode: Int
    public let payload: Data?

    public init(opcode: Int, payload: Data? = nil) {
        self.opcode = opcode
        self.payload = payload
    }
}

public protocol AuthRepository {
    func requestCode(phone: String) async throws -> CodeRequestResponse
    func verifyCode(phone: String, code: String, deviceID: String, platform: String) async throws -> AuthTokens
    func refreshToken(_ token: String) async throws -> AuthTokens
    func logout() async throws
}

public struct CodeRequestResponse: Codable {
    public let retryAfterSec: Int
    public let expiresInSec: Int
}

public struct AuthTokens: Codable {
    public let accessToken: String
    public let refreshToken: String
    public let userID: Int64
    public let expiresIn: Int64
}

public protocol MediaRepository {
    func uploadImage(_ data: Data, metadata: ImageMetadata) async throws -> Attachment
    func downloadImage(objectKey: String) async throws -> Data
}

public struct ImageMetadata: Codable {
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
