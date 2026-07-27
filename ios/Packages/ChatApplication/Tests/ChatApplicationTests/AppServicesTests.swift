import Testing
@testable import ChatApplication
import ChatDomain
import ChatInfrastructure
import Foundation

@Test
func scaffoldServicesConstruct() throws {
    let services = try AppServices.make(
        gatewayWSURL: URL(string: "ws://127.0.0.1:8081/ws")!
    )
    #expect(services.gatewayWSURL.host == "127.0.0.1")
    #expect(services.realtimeListenGate.beginIfNeeded())
    #expect(!services.realtimeListenGate.beginIfNeeded())
    // 组合根以 Port 暴露（Live 实例）。
    #expect(services.messageRemote is MessageAPI)
    #expect(services.conversationRemote is ConversationAPI)
}

@Test
func fakeAssembledSendPathSucceedsToAccepted() async throws {
    let store = FakeMessageStore()
    let remote = FakeMessageRemote(
        behavior: .succeed(
            SendMessageResponse(
                serverMessageID: "srv-ok-1",
                conversationSeq: 7,
                isDuplicate: false,
                serverReceivedAtMs: 1_700_000_000_000
            )
        )
    )
    let executor = AppServices.assembleSendExecutor(store: store, remote: remote)

    try await executor.enqueueLocalThenSend(
        Message(
            clientMessageID: "c-ok-1",
            conversationID: "conv-ok",
            senderUserID: 1,
            messageType: "text",
            content: #"{"text":"ok"}"#,
            status: .queued
        )
    )

    #expect(remote.sendCount == 1)
    let message = store.message(clientMessageID: "c-ok-1")
    #expect(message?.status == .accepted)
    #expect(message?.serverMessageID == "srv-ok-1")
    #expect(message?.conversationSeq == 7)
}

@Test
func fakeAssembledSendPathFailsToFailed() async throws {
    let store = FakeMessageStore()
    let remote = FakeMessageRemote(
        behavior: .fail(HTTPClientError.status(code: 500, body: "boom"))
    )
    let executor = AppServices.assembleSendExecutor(store: store, remote: remote)

    try await executor.enqueueLocalThenSend(
        Message(
            clientMessageID: "c-fail-1",
            conversationID: "conv-fail",
            senderUserID: 1,
            messageType: "text",
            content: #"{"text":"fail"}"#,
            status: .queued
        )
    )

    #expect(remote.sendCount == 1)
    #expect(store.message(clientMessageID: "c-fail-1")?.status == .failed)
}
