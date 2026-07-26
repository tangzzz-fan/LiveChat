import Foundation
import ChatDomain

public struct DirectConversationResult: Sendable, Equatable {
    public let conversationID: String
    public let type: String
    public let peerUserID: Int64
    public let created: Bool
}

public final class ConversationAPI: Sendable {
    private let http: HTTPClient
    private let session: SessionStore

    public init(http: HTTPClient, session: SessionStore) {
        self.http = http
        self.session = session
    }

    public func ensureDirect(peerUserID: Int64) async throws -> DirectConversationResult {
        guard let creds = try session.load() else {
            throw HTTPClientError.status(code: 401, body: "not logged in")
        }
        struct Body: Encodable { let peer_user_id: Int64 }
        struct Resp: Decodable {
            let conversation_id: String
            let type: String
            let peer_user_id: Int64
            let created: Bool
        }
        let resp: Resp = try await http.postJSON(
            path: "/v1/conversations/direct",
            body: Body(peer_user_id: peerUserID),
            bearerToken: creds.accessToken
        )
        return DirectConversationResult(
            conversationID: resp.conversation_id,
            type: resp.type,
            peerUserID: resp.peer_user_id,
            created: resp.created
        )
    }

    public func listRemoteSummaries() async throws -> [ConversationSummary] {
        guard let creds = try session.load() else { return [] }
        struct Member: Decodable {
            let user_id: Int64
            let display_name: String?
        }
        struct Row: Decodable {
            let user_id: Int64
            let conversation_id: String
            let conversation_type: String
            let last_message_preview: String?
            let last_message_at: String?
            let unread_count: Int
            let is_pinned: Bool
            let members: [Member]?
        }
        struct Resp: Decodable { let conversations: [Row] }
        let resp: Resp = try await http.getJSON(
            path: "/v1/conversations",
            bearerToken: creds.accessToken
        )
        return resp.conversations.map { row in
            let title = row.members?
                .first(where: { $0.user_id != creds.userID })?
                .display_name
            return ConversationSummary(
                userID: row.user_id,
                conversationID: row.conversation_id,
                type: row.conversation_type,
                title: title ?? row.conversation_id,
                lastMessagePreview: row.last_message_preview,
                lastMessageAt: nil,
                unreadCount: row.unread_count,
                isPinned: row.is_pinned,
                isMuted: false
            )
        }
    }
}

public final class MessageAPI: Sendable {
    private let http: HTTPClient
    private let session: SessionStore

    public init(http: HTTPClient, session: SessionStore) {
        self.http = http
        self.session = session
    }

    public func send(_ request: SendMessageRequest) async throws -> SendMessageResponse {
        guard let creds = try session.load() else {
            throw HTTPClientError.status(code: 401, body: "not logged in")
        }
        struct Body: Encodable {
            let client_message_id: String
            let conversation_id: String
            let message_type: String
            let content: String
        }
        struct Resp: Decodable {
            let server_message_id: String
            let conversation_seq: Int64
            let is_duplicate: Bool
            let server_received_at_ms: Int64
        }
        let resp: Resp = try await http.postJSON(
            path: "/v1/messages/send",
            body: Body(
                client_message_id: request.clientMessageID,
                conversation_id: request.conversationID,
                message_type: request.messageType,
                content: request.content
            ),
            bearerToken: creds.accessToken
        )
        return SendMessageResponse(
            serverMessageID: resp.server_message_id,
            conversationSeq: resp.conversation_seq,
            isDuplicate: resp.is_duplicate,
            serverReceivedAtMs: resp.server_received_at_ms
        )
    }
}

/// 有界发送队列：本地先落库，再 HTTP；429 按 Retry-After + jitter 退避。
public actor MessageSendExecutor {
    public static let maxQueueDepth = 100

    private let database: LocalDatabase
    private let api: MessageAPI
    private var isProcessing = false

    public init(database: LocalDatabase, api: MessageAPI) {
        self.database = database
        self.api = api
    }

    public func enqueueLocalThenSend(_ message: Message) async throws {
        let pending = try database.fetchPendingSend(limit: Self.maxQueueDepth + 1)
        if pending.count >= Self.maxQueueDepth {
            throw SendQueueError.full
        }
        try database.insertMessage(message)
        await process()
    }

    public func processPending() async {
        await process()
    }

    private func process() async {
        guard !isProcessing else { return }
        isProcessing = true
        defer { isProcessing = false }

        while true {
            let batch: [MessageRecord]
            do {
                batch = try database.fetchPendingSend(limit: 1)
            } catch {
                return
            }
            guard let item = batch.first else { return }

            do {
                try database.updateMessageStatus(
                    clientMessageID: item.clientMessageID,
                    status: .sending
                )
                let response = try await api.send(
                    SendMessageRequest(
                        clientMessageID: item.clientMessageID,
                        conversationID: item.conversationID,
                        messageType: item.messageType,
                        content: item.content ?? ""
                    )
                )
                try database.updateMessageAccepted(
                    clientMessageID: item.clientMessageID,
                    serverMessageID: response.serverMessageID,
                    conversationSeq: response.conversationSeq,
                    serverReceivedAtMs: response.serverReceivedAtMs
                )
            } catch let HTTPClientError.rateLimited(retryAfter, _) {
                // Stay in sending; wait then retry same client_message_id.
                let jitter = Double.random(in: 0...(retryAfter * 0.2))
                try? await Task.sleep(nanoseconds: UInt64((retryAfter + jitter) * 1_000_000_000))
            } catch {
                try? database.updateMessageStatus(
                    clientMessageID: item.clientMessageID,
                    status: .failed
                )
            }
        }
    }
}

public enum SendQueueError: Error, Sendable {
    case full
}
