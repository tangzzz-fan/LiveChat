import Testing
import Foundation
import ChatDomain
@testable import ChatInfrastructure

@Test
func incomingMessageIncrementsUnreadForPeer() throws {
    let messages = FakeMessageStore()
    let conversations = FakeConversationStore()
    let myUserID: Int64 = 1

    try IncomingMessageApplier.applyMessageCreated(
        MessageCreatedPayload(
            serverMessageID: "m1",
            conversationID: "c1",
            conversationSeq: 1,
            senderUserID: 2,
            messageType: "text",
            content: #"{"text":"hi"}"#,
            serverReceivedAtMs: 1
        ),
        myUserID: myUserID,
        messages: messages,
        conversations: conversations
    )
    try IncomingMessageApplier.applyMessageCreated(
        MessageCreatedPayload(
            serverMessageID: "m2",
            conversationID: "c1",
            conversationSeq: 2,
            senderUserID: 2,
            messageType: "text",
            content: #"{"text":"again"}"#,
            serverReceivedAtMs: 2
        ),
        myUserID: myUserID,
        messages: messages,
        conversations: conversations
    )

    let rows = try conversations.fetchConversationSummaries(userID: myUserID)
    #expect(rows.count == 1)
    #expect(rows[0].unreadCount == 2)
}

@Test
func clearUnreadAndMessageReadAdvanceOwnStatus() throws {
    let messages = FakeMessageStore()
    let conversations = FakeConversationStore()
    let myUserID: Int64 = 10

    try messages.insertMessage(
        Message(
            serverMessageID: "own-1",
            clientMessageID: "c-own-1",
            conversationID: "c-read",
            conversationSeq: 3,
            senderUserID: myUserID,
            messageType: "text",
            content: #"{"text":"me"}"#,
            status: .accepted
        )
    )
    try conversations.upsertConversationSummary(
        ConversationSummary(
            userID: myUserID,
            conversationID: "c-read",
            type: "direct",
            title: "peer",
            lastMessagePreview: "x",
            unreadCount: 4
        )
    )

    try conversations.clearUnread(userID: myUserID, conversationID: "c-read")
    #expect(try conversations.fetchConversationSummaries(userID: myUserID)[0].unreadCount == 0)

    try IncomingMessageApplier.applyMessageRead(
        conversationID: "c-read",
        lastReadSeq: 3,
        myUserID: myUserID,
        messages: messages
    )
    #expect(messages.message(clientMessageID: "c-own-1")?.status == .read)
}

@Test
func acceptedCanTransitionDirectlyToRead() {
    #expect(MessageStatus.accepted.canTransition(to: .read))
}
