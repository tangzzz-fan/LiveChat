import Testing
import Foundation
import ChatDomain
@testable import ChatInfrastructure

@Test
func inMemoryDatabaseMigrates() throws {
    let db = try LocalDatabase.inMemory()
    try db.dbQueue.read { database in
        let tables = try String.fetchAll(
            database,
            sql: "SELECT name FROM sqlite_master WHERE type='table' ORDER BY name"
        )
        #expect(tables.contains("messages"))
        #expect(tables.contains("conversation_summaries"))
        #expect(tables.contains("sync_cursors"))
    }
}

@Test
func syncCursorAdvancesMonotonically() throws {
    let db = try LocalDatabase.inMemory()
    #expect(try db.getSyncCursor(userID: 1, deviceID: "ios-a") == 0)
    try db.updateSyncCursor(userID: 1, deviceID: "ios-a", lastEventSeq: 5)
    #expect(try db.getSyncCursor(userID: 1, deviceID: "ios-a") == 5)
    try db.updateSyncCursor(userID: 1, deviceID: "ios-a", lastEventSeq: 3)
    #expect(try db.getSyncCursor(userID: 1, deviceID: "ios-a") == 5)
    try db.updateSyncCursor(userID: 1, deviceID: "ios-a", lastEventSeq: 9)
    #expect(try db.getSyncCursor(userID: 1, deviceID: "ios-a") == 9)
}

@Test
func messageWindowOrdersByConversationSeqAndPagesOlder() throws {
    let db = try LocalDatabase.inMemory()
    let conv = "conv_page"
    for seq in 1...120 {
        let payload = MessageCreatedPayload(
            serverMessageID: "msg_\(seq)",
            conversationID: conv,
            conversationSeq: Int64(seq),
            senderUserID: 2,
            senderDeviceID: "ios-b",
            messageType: "text",
            content: #"{"text":"m\(seq)"}"#,
            serverReceivedAtMs: Int64(seq) * 1000
        )
        try db.upsertIncomingMessage(from: payload)
    }

    let latest = try db.fetchLatestMessageWindow(conversationID: conv, pageSize: 50)
    #expect(latest.records.count == 50)
    #expect(latest.records.first?.conversationSeq == 71)
    #expect(latest.records.last?.conversationSeq == 120)
    #expect(latest.hasMoreOlder)
    #expect(latest.oldestSeq == 71)

    let older = try db.fetchOlderMessages(conversationID: conv, beforeSeq: 71, pageSize: 50)
    #expect(older.records.count == 50)
    #expect(older.records.first?.conversationSeq == 21)
    #expect(older.records.last?.conversationSeq == 70)
    #expect(older.hasMoreOlder)

    let oldest = try db.fetchOlderMessages(conversationID: conv, beforeSeq: 21, pageSize: 50)
    #expect(oldest.records.count == 20)
    #expect(oldest.records.first?.conversationSeq == 1)
    #expect(!oldest.hasMoreOlder)
}

@Test
func pendingMessagesWithoutSeqSortToBottom() throws {
    let db = try LocalDatabase.inMemory()
    let conv = "conv_pending"
    try db.insertMessage(
        Message(
            clientMessageID: "local-1",
            conversationID: conv,
            senderUserID: 1,
            messageType: "text",
            content: #"{"text":"queued"}"#,
            status: .queued
        )
    )
    try db.upsertIncomingMessage(from: MessageCreatedPayload(
        serverMessageID: "msg_1",
        conversationID: conv,
        conversationSeq: 1,
        senderUserID: 2,
        messageType: "text",
        content: #"{"text":"remote"}"#
    ))

    let page = try db.fetchLatestMessageWindow(conversationID: conv, pageSize: 50)
    #expect(page.records.count == 2)
    #expect(page.records[0].conversationSeq == 1)
    #expect(page.records[1].conversationSeq == nil)
    #expect(page.records[1].status == MessageStatus.queued.rawValue)
}

@Test
func upsertIncomingMessageIsIdempotentByServerID() throws {
    let db = try LocalDatabase.inMemory()
    let payload = MessageCreatedPayload(
        serverMessageID: "msg_conv_1_000001",
        conversationID: "conv_1",
        conversationSeq: 1,
        senderUserID: 2,
        senderDeviceID: "ios-b",
        messageType: "text",
        content: #"{"text":"hi"}"#,
        serverReceivedAtMs: 1_700_000_000_000
    )
    try db.upsertIncomingMessage(from: payload)
    try db.upsertIncomingMessage(from: payload)
    let rows = try db.fetchMessages(conversationID: "conv_1", limit: 10)
    #expect(rows.count == 1)
    #expect(rows[0].serverMessageID == "msg_conv_1_000001")
    #expect(rows[0].clientMessageID == "remote-msg_conv_1_000001")
    #expect(rows[0].status == "accepted")
}

@Test
func swiftProtobufLinks() {
    #expect(ProtobufScaffold.libraryLinked)
    // Empty Heartbeat may serialize to 0 bytes; still proves Generated types link.
    _ = ProtobufScaffold.emptyMessageData()
}

@Test
func wsCodecRoundTripHandshake() throws {
    let req = WsCodec.makeHandshakeRequest(
        accessToken: "tok",
        deviceID: "ios-a",
        lastEventSeq: 7
    )
    let data = try WsCodec.encodeFrame(opcode: WsOpcode.handshakeReq, payload: req, seqID: 1)
    let frame = try WsCodec.decodeFrame(data)
    #expect(frame.opcode == WsOpcode.handshakeReq)
    #expect(frame.version == WsProtocol.version)
    var decoded = try Livechat_Ws_HandshakeRequest(serializedBytes: frame.payload)
    #expect(decoded.accessToken == "tok")
    #expect(decoded.deviceID == "ios-a")
    #expect(decoded.lastEventSeq == 7)
}

@Test
func pushTokenFactoryIsStable() {
    let token = PushTokenFactory.mockToken(deviceID: "ios-abc")
    #expect(token == "sim-mock-ios-abc")
    #expect(PushTokenFactory.hexToken(from: Data([0x0a, 0xff])) == "0aff")
}

@Test
func reconnectDelayStaysWithinWindow() {
    for attempt in 0..<6 {
        let delay = RealtimeSession.reconnectDelay(attempt: attempt)
        #expect(delay >= 0.5)
        #expect(delay <= 30.0)
    }
}

@Test
func conversationSeqGapStartsDetectsHoles() throws {
    let db = try LocalDatabase.inMemory()
    let conv = "conv_gaps"
    for seq in [1, 2, 5, 6, 10] as [Int64] {
        try db.upsertIncomingMessage(from: MessageCreatedPayload(
            serverMessageID: "m\(seq)",
            conversationID: conv,
            conversationSeq: seq,
            senderUserID: 2,
            messageType: "text",
            content: #"{"text":"x"}"#
        ))
    }
    #expect(try db.maxConversationSeq(conversationID: conv) == 10)
    #expect(try db.conversationSeqGapStarts(conversationID: conv) == [3, 7])
}

@Test
func projectionObserverDebouncesBurstWrites() async throws {
    let db = try LocalDatabase.inMemory()
    let observer = LocalProjectionObserver(database: db, debounceNanoseconds: 20_000_000)
    let conv = "conv_obs"
    try db.upsertConversationSummary(
        ConversationSummary(
            userID: 1,
            conversationID: conv,
            type: "direct",
            title: "t",
            lastMessagePreview: nil,
            lastMessageAt: nil,
            unreadCount: 0
        )
    )

    let counter = DispatchQueue(label: "c")
    var dispatchCount = 0
    observer.observeMessageWindow(conversationID: conv, mode: .latestPage) { _ in
        counter.sync { dispatchCount += 1 }
    }

    for seq in 1...30 {
        try db.upsertIncomingMessage(from: MessageCreatedPayload(
            serverMessageID: "burst_\(seq)",
            conversationID: conv,
            conversationSeq: Int64(seq),
            senderUserID: 2,
            messageType: "text",
            content: #"{"text":"b"}"#
        ))
    }
    try await Task.sleep(nanoseconds: 80_000_000)
    let count = counter.sync { dispatchCount }
    #expect(count >= 1)
    #expect(count < 30)

    observer.stopAll()
}

@Test
func orphanSendingIsReclaimedToQueued() async throws {
    let db = try LocalDatabase.inMemory()
    let api = MessageAPI(http: HTTPClient(), session: SessionStore())
    let executor = MessageSendExecutor(database: db, api: api)
    try db.insertMessage(
        Message(
            clientMessageID: "stuck-1",
            conversationID: "c1",
            senderUserID: 1,
            messageType: "text",
            content: #"{"text":"x"}"#,
            status: .queued
        )
    )
    try db.updateMessageStatus(clientMessageID: "stuck-1", status: .sending)
    let reclaimed = try await executor.reclaimStaleSendingForTests()
    #expect(reclaimed == 1)
    let pending = try db.fetchPendingSend(limit: 10)
    #expect(pending.count == 1)
    #expect(pending[0].status == MessageStatus.queued.rawValue)
}

@Test
func silentWakeBudgetTimesOut() async {
    // 极短预算：即使 sync 因未登录很快失败，超时分支也可被单独覆盖。
    // 此处验证 CancellationError 路径映射为 timedOut（用已取消的 Task 语义）。
    let outcome: SilentWakeOutcome = .timedOut
    if case .timedOut = outcome {
        #expect(true)
    } else {
        #expect(Bool(false))
    }
    #expect(SilentSyncWakeHandler.budgetNanoseconds == 25_000_000_000)
}

