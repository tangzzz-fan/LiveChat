import Foundation
import ChatDomain

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

/// 有界发送队列：本地先落库，再 HTTP；429 按 Retry-After + jitter 退避；sending 超时回 queued。
public actor MessageSendExecutor {
    public static let maxQueueDepth = 100
    /// sending 卡住上限（弱网 / API 挂起）；超时回 queued，由 path 恢复或重试续跑。
    public static let sendingTimeoutNanoseconds: UInt64 = 30_000_000_000

    private let store: any MessageStore
    private let remote: any MessageRemote
    private let sendingTimeoutNanoseconds: UInt64
    private var isProcessing = false
    private var sendingStartedAt: [String: ContinuousClock.Instant] = [:]

    public init(
        store: any MessageStore,
        remote: any MessageRemote,
        sendingTimeoutNanoseconds: UInt64 = MessageSendExecutor.sendingTimeoutNanoseconds
    ) {
        self.store = store
        self.remote = remote
        self.sendingTimeoutNanoseconds = sendingTimeoutNanoseconds
    }

    public func enqueueLocalThenSend(_ message: Message) async throws {
        let pending = try store.fetchPendingSend(limit: Self.maxQueueDepth + 1)
        if pending.count >= Self.maxQueueDepth {
            throw SendQueueError.full
        }
        try store.insertMessage(message)
        await process()
    }

    public func processPending() async {
        await process()
    }

    /// 路径恢复入口：先收回超时/孤儿 sending，再续跑（不与 429 退避叠成风暴——仍单 actor 串行）。
    public func reclaimStaleSendingAndProcess() async {
        _ = try? reclaimStaleSending(now: .now)
        await process()
    }

    /// 测试钩子：强制把超时 sending 收回为 queued。
    public func reclaimStaleSendingForTests(now: ContinuousClock.Instant = .now) throws -> Int {
        try reclaimStaleSending(now: now)
    }

    private func reclaimStaleSending(now: ContinuousClock.Instant) throws -> Int {
        let pending = try store.fetchPendingSend(limit: Self.maxQueueDepth)
        var count = 0
        for item in pending where item.status == .sending {
            let stale: Bool
            if let started = sendingStartedAt[item.clientMessageID] {
                stale = now - started >= Duration.seconds(30)
            } else {
                // 进程重启后无内存计时 → 视为孤儿，收回避免永久卡 sending
                stale = true
            }
            if stale {
                try store.updateMessageStatus(
                    clientMessageID: item.clientMessageID,
                    status: .queued
                )
                sendingStartedAt.removeValue(forKey: item.clientMessageID)
                count += 1
            }
        }
        return count
    }

    private func process() async {
        guard !isProcessing else { return }
        isProcessing = true
        defer { isProcessing = false }

        while true {
            do {
                _ = try reclaimStaleSending(now: .now)
            } catch {
                return
            }

            let batch: [Message]
            do {
                batch = try store.fetchPendingSend(limit: 1)
            } catch {
                return
            }
            guard let item = batch.first else { return }

            do {
                try store.updateMessageStatus(
                    clientMessageID: item.clientMessageID,
                    status: .sending
                )
                sendingStartedAt[item.clientMessageID] = .now
                let response = try await sendWithTimeout(
                    SendMessageRequest(
                        clientMessageID: item.clientMessageID,
                        conversationID: item.conversationID,
                        messageType: item.messageType,
                        content: item.content
                    )
                )
                try store.updateMessageAccepted(
                    clientMessageID: item.clientMessageID,
                    serverMessageID: response.serverMessageID,
                    conversationSeq: response.conversationSeq,
                    serverReceivedAtMs: response.serverReceivedAtMs
                )
                sendingStartedAt.removeValue(forKey: item.clientMessageID)
            } catch let HTTPClientError.rateLimited(retryAfter, _) {
                // Stay in sending; wait then retry same client_message_id（刷新计时）。
                sendingStartedAt[item.clientMessageID] = .now
                let jitter = Double.random(in: 0...(retryAfter * 0.2))
                try? await Task.sleep(nanoseconds: UInt64((retryAfter + jitter) * 1_000_000_000))
            } catch is CancellationError {
                try? store.updateMessageStatus(
                    clientMessageID: item.clientMessageID,
                    status: .queued
                )
                sendingStartedAt.removeValue(forKey: item.clientMessageID)
            } catch {
                try? store.updateMessageStatus(
                    clientMessageID: item.clientMessageID,
                    status: .failed
                )
                sendingStartedAt.removeValue(forKey: item.clientMessageID)
            }
        }
    }

    private func sendWithTimeout(_ request: SendMessageRequest) async throws -> SendMessageResponse {
        try await withThrowingTaskGroup(of: SendMessageResponse.self) { group in
            group.addTask {
                try await self.remote.sendMessage(request)
            }
            group.addTask {
                try await Task.sleep(nanoseconds: self.sendingTimeoutNanoseconds)
                throw CancellationError()
            }
            let result = try await group.next()!
            group.cancelAll()
            return result
        }
    }
}

public enum SendQueueError: Error, Sendable {
    case full
}
