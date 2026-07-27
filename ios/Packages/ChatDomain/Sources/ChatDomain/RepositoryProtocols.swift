import Foundation

// MARK: - 落地说明（Spec 13 §6）
//
// 本文件是 Domain「端口」清单 + 共享 DTO。
// 除 AuthRepository 外，多数协议尚未被 Infrastructure `conform`；
// 主链路用拆开的具体类型（Executor / API / LocalDatabase）。
// 完整对照表：docs/engineering-problems/19-domain-repository-ports-vs-concrete-executors.md

/// 本地消息读写 + 远程发送的粗粒度端口（脚手架）。
/// 落地：`LocalDatabase` + `MessageAPI` + `MessageSendExecutor`（未直接 conform）。
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

/// 落地：`ConversationAPI` + `LocalDatabase`（未直接 conform）。
public protocol ConversationRepository: Sendable {
    func getConversations() async throws -> [ConversationSummary]
    func upsertConversation(_ conversation: ConversationSummary) async throws
}

/// 落地：`SyncAPI` + `LocalDatabase`；编排见 `SyncExecutor`（未直接 conform）。
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

    public init(
        eventSeq: Int64,
        userID: Int64,
        conversationID: String? = nil,
        eventType: String,
        payload: String
    ) {
        self.eventSeq = eventSeq
        self.userID = userID
        self.conversationID = conversationID
        self.eventType = eventType
        self.payload = payload
    }

    enum CodingKeys: String, CodingKey {
        case eventSeq = "event_seq"
        case userID = "user_id"
        case conversationID = "conversation_id"
        case eventType = "event_type"
        case payload
    }
}

public struct SyncResponse: Codable, Sendable {
    public let events: [SyncEvent]
    public let hasMore: Bool
    public let latestEventSeq: Int64

    public init(events: [SyncEvent], hasMore: Bool, latestEventSeq: Int64) {
        self.events = events
        self.hasMore = hasMore
        self.latestEventSeq = latestEventSeq
    }

    enum CodingKeys: String, CodingKey {
        case events
        case hasMore = "has_more"
        case latestEventSeq = "latest_event_seq"
    }
}

/// ✅ 已实现：`AuthRepositoryLive`。
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

/// 落地：`PushTokenAPI` + `SilentSyncWakeHandler`（签名不完全对齐，未 conform）。
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

/// 脚手架端口；更贴地的传输抽象是 Infrastructure 的 `WebSocketTransport`。
/// 落地编排：`RealtimeSession` + `URLSessionWebSocketTransport`（未 conform 本协议）。
public protocol WebSocketRepository: Sendable {
    func connect() async throws
    func disconnect() async
    func sendFrame(_ frame: WebSocketFrame) async throws
    var messageStream: AsyncStream<WebSocketFrame> { get }
}

/// 留给 0049；服务端媒体 API 已就绪（0014）。
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
