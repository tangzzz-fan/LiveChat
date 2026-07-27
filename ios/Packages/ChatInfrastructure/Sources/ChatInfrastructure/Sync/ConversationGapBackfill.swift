import Foundation
import ChatDomain

public struct ConversationMessagesPage: Sendable {
    public let messages: [RemoteConversationMessage]
    public let latestSeq: Int64
    public let hasMore: Bool
}

public struct RemoteConversationMessage: Sendable {
    public let serverMessageID: String
    public let conversationID: String
    public let conversationSeq: Int64
    public let senderUserID: Int64
    public let senderDeviceID: String?
    public let clientMessageID: String?
    public let messageType: String
    public let content: String
    public let serverReceivedAtMs: Int64?
}

/// `GET /v1/conversations/{cid}/messages` — 会话缺口补拉（与全局 sync cursor 分工）。
public final class ConversationMessagesAPI: Sendable {
    private let http: HTTPClient
    private let session: SessionStore

    public init(http: HTTPClient, session: SessionStore) {
        self.http = http
        self.session = session
    }

    public func fetchMessages(
        conversationID: String,
        fromSeq: Int64,
        limit: Int = 50
    ) async throws -> ConversationMessagesPage {
        guard let creds = try session.load() else {
            throw HTTPClientError.status(code: 401, body: "not logged in")
        }
        struct Row: Decodable {
            let server_message_id: String
            let conversation_id: String
            let conversation_seq: Int64
            let sender_user_id: Int64
            let sender_device_id: String?
            let client_message_id: String?
            let message_type: String
            let content: String
            let server_received_at: String?
        }
        struct Resp: Decodable {
            let messages: [Row]
            let latest_seq: Int64
            let has_more: Bool
        }
        let resp: Resp = try await http.getJSON(
            path: "/v1/conversations/\(conversationID)/messages",
            bearerToken: creds.accessToken,
            query: [
                URLQueryItem(name: "from_seq", value: String(fromSeq)),
                URLQueryItem(name: "limit", value: String(limit)),
            ]
        )
        let messages = resp.messages.map { row in
            RemoteConversationMessage(
                serverMessageID: row.server_message_id,
                conversationID: row.conversation_id,
                conversationSeq: row.conversation_seq,
                senderUserID: row.sender_user_id,
                senderDeviceID: row.sender_device_id,
                clientMessageID: row.client_message_id,
                messageType: row.message_type,
                content: row.content,
                serverReceivedAtMs: Self.parseMs(row.server_received_at)
            )
        }
        return ConversationMessagesPage(
            messages: messages,
            latestSeq: resp.latest_seq,
            hasMore: resp.has_more
        )
    }

    private static func parseMs(_ rfc3339: String?) -> Int64? {
        guard let rfc3339 else { return nil }
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        if let date = formatter.date(from: rfc3339) {
            return Int64(date.timeIntervalSince1970 * 1000)
        }
        formatter.formatOptions = [.withInternetDateTime]
        guard let date = formatter.date(from: rfc3339) else { return nil }
        return Int64(date.timeIntervalSince1970 * 1000)
    }
}

public struct GapBackfillResult: Sendable, Equatable {
    public let appliedCount: Int
    public let localMaxSeq: Int64
    public let latestSeq: Int64

    public init(appliedCount: Int, localMaxSeq: Int64, latestSeq: Int64) {
        self.appliedCount = appliedCount
        self.localMaxSeq = localMaxSeq
        self.latestSeq = latestSeq
    }
}

/// Spec 06 §4.4：探测会话 `conversation_seq` 缺口并用消息补拉 API 填平（不推进 sync cursor）。
public actor ConversationGapBackfill {
    public static let pageSize = 50

    private let database: LocalDatabase
    private let api: ConversationMessagesAPI
    private let session: SessionStore

    public init(database: LocalDatabase, api: ConversationMessagesAPI, session: SessionStore) {
        self.database = database
        self.api = api
        self.session = session
    }

    public func backfillIfNeeded(conversationID: String) async throws -> GapBackfillResult {
        guard let creds = try session.load() else {
            throw HTTPClientError.status(code: 401, body: "not logged in")
        }

        var applied = 0
        let gapStarts = try database.conversationSeqGapStarts(conversationID: conversationID)
        for start in gapStarts.prefix(5) {
            applied += try await pullPages(
                conversationID: conversationID,
                fromSeq: start,
                myUserID: creds.userID,
                stopWhenCaughtUpToLocal: true
            )
        }

        var localMax = try database.maxConversationSeq(conversationID: conversationID)
        let probe = try await api.fetchMessages(
            conversationID: conversationID,
            fromSeq: max(localMax, 0),
            limit: 1
        )
        let latest = probe.latestSeq
        if localMax < latest {
            let from = localMax + 1
            applied += try await pullPages(
                conversationID: conversationID,
                fromSeq: from,
                myUserID: creds.userID,
                stopWhenCaughtUpToLocal: false
            )
            localMax = try database.maxConversationSeq(conversationID: conversationID)
        }

        return GapBackfillResult(
            appliedCount: applied,
            localMaxSeq: localMax,
            latestSeq: latest
        )
    }

    private func pullPages(
        conversationID: String,
        fromSeq: Int64,
        myUserID: Int64,
        stopWhenCaughtUpToLocal: Bool
    ) async throws -> Int {
        var cursor = fromSeq
        var applied = 0
        while true {
            try Task.checkCancellation()
            let page = try await api.fetchMessages(
                conversationID: conversationID,
                fromSeq: cursor,
                limit: Self.pageSize
            )
            if page.messages.isEmpty {
                break
            }
            for remote in page.messages {
                let payload = MessageCreatedPayload(
                    serverMessageID: remote.serverMessageID,
                    conversationID: remote.conversationID,
                    conversationSeq: remote.conversationSeq,
                    senderUserID: remote.senderUserID,
                    senderDeviceID: remote.senderDeviceID,
                    messageType: remote.messageType,
                    content: remote.content,
                    serverReceivedAtMs: remote.serverReceivedAtMs
                )
                try IncomingMessageApplier.applyMessageCreated(
                    payload,
                    myUserID: myUserID,
                    database: database
                )
                applied += 1
                cursor = remote.conversationSeq + 1
            }
            if stopWhenCaughtUpToLocal {
                // 填相邻缺口：本页后若已连上本地更高 seq，可停。
                let gaps = try database.conversationSeqGapStarts(conversationID: conversationID)
                if !gaps.contains(where: { $0 >= fromSeq && $0 < cursor }) {
                    break
                }
            }
            if !page.hasMore || cursor > page.latestSeq {
                break
            }
            await Task.yield()
        }
        return applied
    }
}
