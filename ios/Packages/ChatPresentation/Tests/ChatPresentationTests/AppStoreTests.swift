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

@Test
@MainActor
func selectConversationClearsMessageWindow() {
    let store = AppStoreFactory.make()
    store.dispatch(
        .chat(.conversationOpened(
            id: "conv_old",
            messages: [
                ChatState.MessageRow(
                    clientMessageID: "m1",
                    serverMessageID: "s1",
                    text: "hi",
                    status: "accepted",
                    isMine: true
                )
            ],
            oldestLoadedSeq: 1,
            hasMoreOlder: true
        ))
    )
    #expect(store.state.chat.visibleMessages.count == 1)
    #expect(store.state.chat.hasMoreOlder)

    store.dispatch(.chat(.selectConversation("conv_new")))
    #expect(store.state.chat.activeConversationID == "conv_new")
    #expect(store.state.chat.visibleMessages.isEmpty)
    #expect(store.state.chat.oldestLoadedSeq == nil)
    #expect(!store.state.chat.hasMoreOlder)
}

@Test
@MainActor
func multiSelectEnterToggleAndExit() {
    let store = AppStoreFactory.make()
    store.dispatch(.chat(.enterMultiSelect("m1")))
    #expect(store.state.chat.isMultiSelecting)
    #expect(store.state.chat.selectedClientMessageIDs == ["m1"])
    store.dispatch(.chat(.toggleMessageSelection("m2")))
    #expect(store.state.chat.selectedClientMessageIDs == ["m1", "m2"])
    store.dispatch(.chat(.toggleMessageSelection("m1")))
    #expect(store.state.chat.selectedClientMessageIDs == ["m2"])
    store.dispatch(.chat(.exitMultiSelect))
    #expect(!store.state.chat.isMultiSelecting)
    #expect(store.state.chat.selectedClientMessageIDs.isEmpty)
}
