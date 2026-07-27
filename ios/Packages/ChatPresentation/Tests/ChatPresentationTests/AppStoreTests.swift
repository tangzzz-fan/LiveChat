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

@Test
@MainActor
func syncFinishedUpdatesBanner() {
    let store = AppStoreFactory.make()
    store.dispatch(.chat(.syncStarted))
    #expect(store.state.chat.isSyncing)
    store.dispatch(.chat(.syncFinished(applied: 2, cursor: 7)))
    #expect(!store.state.chat.isSyncing)
    #expect(store.state.chat.syncBanner?.contains("2") == true)
    #expect(store.state.chat.syncBanner?.contains("7") == true)
}

@Test
@MainActor
func pushTokenBannerUpdates() {
    let store = AppStoreFactory.make()
    store.dispatch(.chat(.setPushTokenBanner("push_token 已注册")))
    #expect(store.state.chat.pushTokenBanner?.contains("push_token") == true)
}
