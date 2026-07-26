import Testing
@testable import ChatPresentation

@Test
@MainActor
func rootStoreUpdatesDraft() {
    let store = AppStoreFactory.make()
    store.dispatch(.updateDraft("hello"))
    #expect(store.state.composeDraft == "hello")
    #expect(store.state.visibleMessageIDs.isEmpty)
}

@Test
@MainActor
func authPhaseMovesToCode() {
    let store = AppStoreFactory.make()
    store.dispatch(.setPhoneInput("+8613800000001"))
    store.dispatch(.codeRequested)
    #expect(store.state.authPhase == .code)
}
