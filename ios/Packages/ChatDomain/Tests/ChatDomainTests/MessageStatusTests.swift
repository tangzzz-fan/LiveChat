import Testing
@testable import ChatDomain

@Test
func sendingCanBecomeAcceptedFailedOrQueued() {
    #expect(MessageStatus.sending.canTransition(to: .accepted))
    #expect(MessageStatus.sending.canTransition(to: .failed))
    #expect(MessageStatus.sending.canTransition(to: .queued))
}
