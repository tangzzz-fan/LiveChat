import Testing
@testable import ChatDomain

@Test
func sendingCanBecomeAcceptedFailedOrQueued() {
    #expect(MessageStatus.sending.canTransition(to: .accepted))
    #expect(MessageStatus.sending.canTransition(to: .failed))
    #expect(MessageStatus.sending.canTransition(to: .queued))
}

@Test
func userCanCancelQueuedOrSendingButNotAccepted() {
    #expect(MessageStatus.queued.canTransition(to: .cancelled))
    #expect(MessageStatus.sending.canTransition(to: .cancelled))
    #expect(!MessageStatus.accepted.canTransition(to: .cancelled))
    #expect(!MessageStatus.failed.canTransition(to: .cancelled))
    #expect(MessageStatus.cancelled.allowedTransitions.isEmpty)
    #expect(MessageStatus.queued.canCancelSend)
    #expect(MessageStatus.sending.canCancelSend)
    #expect(!MessageStatus.accepted.canCancelSend)
    #expect(MessageStatus.sending.showsSendingIndicator)
}
