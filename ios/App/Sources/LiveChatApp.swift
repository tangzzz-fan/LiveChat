import SwiftUI
import Foundation
import TGReduxKit
import ChatPresentation
import ChatApplication

typealias StoreOfApp = Store<AppState, AppAction>

@main
struct LiveChatApp: App {
    @State private var store: StoreOfApp

    init() {
        let services = (try? AppServices.make()) ?? {
            preconditionFailure("Failed to assemble AppServices")
        }()
        _store = State(initialValue: AppStoreFactory.make(services: services))
    }

    var body: some Scene {
        WindowGroup {
            RootView()
                .provideStore(store)
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
