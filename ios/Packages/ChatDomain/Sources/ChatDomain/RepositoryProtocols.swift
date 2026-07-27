import Foundation

// MARK: - 落地说明（Spec 13 §6）
//
// 阶段 1（0051）：粗 *Repository 文档降级。
// 阶段 2（0052）：细粒度 Store/Remote；Executor 依赖协议；粗协议已删除。
// 阶段 3（0053）：AppServices 以 any Port 组装；FakeMessageStore/Remote 可测主路径。
// 工程问题 19：resolved。
//
// 正式保留：AuthRepository；传输缝 WebSocketTransport（Infra）。
// 禁止空壳 MessageRepositoryLive 等 Adapter 凑 conform。
// MediaRepository：0049 已由 MediaAPI 实现（upload + download/auth）。
// 对照：docs/engineering-problems/19-domain-repository-ports-vs-concrete-executors.md

// MARK: - Fine-grained ports（0052）

/// 本地发送队列所需表面（勿把整个 LocalDatabase 打成巨 Port）。
public protocol MessageStore: Sendable {
    func insertMessage(_ message: Message) throws
    func updateMessageStatus(clientMessageID: String, status: MessageStatus) throws
    func updateMessageAccepted(
        clientMessageID: String,
        serverMessageID: String,
        conversationSeq: Int64,
        serverReceivedAtMs: Int64
    ) throws
    func fetchPendingSend(limit: Int) throws -> [Message]
    /// Sync / 投递落库：按 server_message_id 幂等写入已接受消息。
    func upsertRemoteMessage(
        serverMessageID: String,
        conversationID: String,
        conversationSeq: Int64,
        senderUserID: Int64,
        messageType: String,
        content: String,
        serverReceivedAtMs: Int64?
    ) throws
    /// 己方消息：`conversation_seq <= upToSeq` 推进为 read（MAX 收敛）。
    func markOwnMessagesRead(conversationID: String, upToSeq: Int64, myUserID: Int64) throws
    /// 仅删本机投影（0056）；不声称服务端撤回。
    func deleteLocalMessage(clientMessageID: String) throws
    /// 按 client_message_id 读取（转发 / 分享用）。
    func fetchMessage(clientMessageID: String) throws -> Message?
}

public protocol MessageRemote: Sendable {
    func sendMessage(_ request: SendMessageRequest) async throws -> SendMessageResponse
}

public protocol SyncCursorStore: Sendable {
    func getSyncCursor(userID: Int64, deviceID: String) throws -> Int64
    func updateSyncCursor(userID: Int64, deviceID: String, lastEventSeq: Int64) throws
}

public protocol SyncRemote: Sendable {
    func fetchEvents(from cursor: Int64, limit: Int) async throws -> SyncResponse
}

public protocol ConversationStore: Sendable {
    func upsertConversationSummary(_ summary: ConversationSummary) throws
    func fetchConversationSummaries(userID: Int64) throws -> [ConversationSummary]
    func clearUnread(userID: Int64, conversationID: String) throws
}

public struct DirectConversationResult: Sendable, Equatable {
    public let conversationID: String
    public let type: String
    public let peerUserID: Int64
    public let created: Bool

    public init(conversationID: String, type: String, peerUserID: Int64, created: Bool) {
        self.conversationID = conversationID
        self.type = type
        self.peerUserID = peerUserID
        self.created = created
    }
}

public protocol ConversationRemote: Sendable {
    func ensureDirect(peerUserID: Int64) async throws -> DirectConversationResult
    func listRemoteSummaries() async throws -> [ConversationSummary]
}

// MARK: - Shared DTOs

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

/// 应用层协议帧（protobuf 编解码在 Infrastructure）
public struct WebSocketFrame: Sendable {
    public let opcode: UInt32
    public let payload: Data

    public init(opcode: UInt32, payload: Data = Data()) {
        self.opcode = opcode
        self.payload = payload
    }
}

/// 图片媒体端口（0049）。上传走 initiate → PUT parts → complete；下载走 download/auth。
public protocol MediaRepository: Sendable {
    func uploadImage(
        _ data: Data,
        metadata: ImageMetadata,
        conversationID: String
    ) async throws -> Attachment
    func downloadImage(objectKey: String, conversationID: String) async throws -> Data
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
