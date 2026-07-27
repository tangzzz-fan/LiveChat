import Testing
import Foundation
import ChatDomain
@testable import ChatInfrastructure

// MARK: - Fakes

private final class FakeMessageRemote: MessageRemote, @unchecked Sendable {
    enum Behavior: Sendable {
        /// 首次挂起直到超时；再次调用直接失败，结束 process 循环。
        case hangThenFail
        case succeed(SendMessageResponse)
    }

    private let behavior: Behavior
    private var _sendCount = 0

    var sendCount: Int { _sendCount }

    init(behavior: Behavior) {
        self.behavior = behavior
    }

    func sendMessage(_ request: SendMessageRequest) async throws -> SendMessageResponse {
        _sendCount += 1
        let n = _sendCount

        switch behavior {
        case .hangThenFail:
            if n == 1 {
                try await Task.sleep(nanoseconds: 60_000_000_000)
                throw CancellationError()
            }
            throw HTTPClientError.status(code: 500, body: "fail after hang")
        case .succeed(let response):
            return response
        }
    }
}

private final class FakeSyncRemote: SyncRemote, @unchecked Sendable {
    private let pages: [SyncResponse]
    private var index = 0

    init(pages: [SyncResponse]) {
        self.pages = pages
    }

    func fetchEvents(from cursor: Int64, limit: Int) async throws -> SyncResponse {
        if index >= pages.count {
            return SyncResponse(events: [], hasMore: false, latestEventSeq: cursor)
        }
        let page = pages[index]
        index += 1
        return page
    }
}

// MARK: - Tests

@Test
func fakeMessageRemoteHangTimesOutBackToQueued() async throws {
    let db = try LocalDatabase.inMemory()
    let remote = FakeMessageRemote(behavior: .hangThenFail)
    let executor = MessageSendExecutor(
        store: db,
        remote: remote,
        sendingTimeoutNanoseconds: 40_000_000 // 40ms
    )
    try await executor.enqueueLocalThenSend(
        Message(
            clientMessageID: "hang-1",
            conversationID: "c-hang",
            senderUserID: 1,
            messageType: "text",
            content: #"{"text":"hang"}"#,
            status: .queued
        )
    )
    // 首次 hang→超时回 queued；再次 fail→failed。证明 Port 可替换且超时路径生效。
    #expect(remote.sendCount >= 1)
    let status = try await db.dbQueue.read { database in
        try String.fetchOne(
            database,
            sql: "SELECT status FROM messages WHERE client_message_id = ?",
            arguments: ["hang-1"]
        )
    }
    #expect(status == MessageStatus.failed.rawValue)
}

@Test
func fakeSyncRemoteAdvancesCursorAfterApply() async throws {
    let db = try LocalDatabase.inMemory()
    let keychain = KeychainStore(service: "com.tango.LiveChat.test.sync.\(UUID().uuidString)")
    let session = SessionStore(keychain: keychain)
    try session.save(
        SessionCredentials(
            accessToken: "tok",
            refreshToken: "ref",
            userID: 42,
            deviceID: "ios-test"
        )
    )

    // 与服务端 fanout JSON 对齐（snake_case）。
    let payloadJSON = """
    {"server_message_id":"msg_sync_1","conversation_id":"conv_sync","conversation_seq":1,"sender_user_id":7,"message_type":"text","content":"{\\"text\\":\\"from sync\\"}","server_received_at_ms":1700000000000}
    """

    let remote = FakeSyncRemote(pages: [
        SyncResponse(
            events: [
                SyncEvent(
                    eventSeq: 10,
                    userID: 42,
                    conversationID: "conv_sync",
                    eventType: "message_created",
                    payload: payloadJSON
                ),
            ],
            hasMore: false,
            latestEventSeq: 10
        ),
    ])

    let executor = SyncExecutor(
        cursorStore: db,
        remote: remote,
        messageStore: db,
        conversationStore: db,
        session: session
    )

    #expect(try db.getSyncCursor(userID: 42, deviceID: "ios-test") == 0)
    let result = try await executor.syncIncremental()
    #expect(result.appliedCount == 1)
    #expect(result.cursor == 10)
    #expect(try db.getSyncCursor(userID: 42, deviceID: "ios-test") == 10)

    let rows = try db.fetchMessages(conversationID: "conv_sync", limit: 10)
    #expect(rows.count == 1)
    #expect(rows[0].serverMessageID == "msg_sync_1")
}
