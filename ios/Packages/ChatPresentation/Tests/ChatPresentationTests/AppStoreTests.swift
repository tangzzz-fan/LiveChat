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
