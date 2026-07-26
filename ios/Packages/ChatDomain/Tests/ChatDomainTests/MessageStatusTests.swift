import Testing
@testable import ChatDomain

@Test
func sendingCanBecomeAcceptedOrFailed() {
    #expect(MessageStatus.sending.canTransition(to: .accepted))
    #expect(MessageStatus.sending.canTransition(to: .failed))
    #expect(!MessageStatus.sending.canTransition(to: .queued))
}
