import SwiftUI
import Foundation
import TGReduxKit
import ChatPresentation
import ChatApplication
import ChatDomain

typealias StoreOfApp = Store<AppState, AppAction>

@main
struct LiveChatApp: App {
    @State private var store: StoreOfApp
    private let media: any MediaRepository

    init() {
        let services = (try? AppServices.make()) ?? {
            preconditionFailure("Failed to assemble AppServices")
        }()
        _store = State(initialValue: AppStoreFactory.make(services: services))
        media = services.media
    }

    var body: some Scene {
        WindowGroup {
            RootView()
                .provideStore(store)
                .environment(\.mediaRepository, media)
                .task { store.dispatch(.auth(.bootstrap)) }
        }
    }
}

struct RootView: View {
    @Environment(StoreOfApp.self) private var store

    var body: some View {
        Group {
            if store.state.isLoggedIn {
                HomeView()
            } else {
                LoginView()
            }
        }
    }
}
