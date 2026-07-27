import Testing
import Foundation
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
