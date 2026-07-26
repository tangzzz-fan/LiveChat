import Testing
@testable import ChatPresentation

@Test
@MainActor
func chatDraftUpdatesViaFeatureAction() {
    let store = AppStoreFactory.make()
    store.dispatch(.chat(.updateDraft("hello")))
    #expect(store.state.chat.composeDraft == "hello")
}

@Test
@MainActor
func authPhaseMovesToCode() {
    let store = AppStoreFactory.make()
    store.dispatch(.auth(.setPhoneInput("+8613800000001")))
    store.dispatch(.auth(.codeRequested))
    #expect(store.state.auth.phase == .code)
}

@Test
@MainActor
func logoutClearsChatState() {
    let store = AppStoreFactory.make()
    store.dispatch(.chat(.updateDraft("x")))
    store.dispatch(.auth(.loggedOut))
    #expect(store.state.chat.composeDraft.isEmpty)
}
