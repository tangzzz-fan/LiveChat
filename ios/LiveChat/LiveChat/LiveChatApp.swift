import SwiftUI
import Foundation
import TGReduxKit
import ChatPresentation
import ChatApplication
import ChatDomain

typealias StoreOfApp = Store<AppState, AppAction>

@main
struct LiveChatApp: App {
    @UIApplicationDelegateAdaptor(AppDelegate.self) private var appDelegate
    @State private var store: StoreOfApp
    @Environment(\.scenePhase) private var scenePhase
    private let media: any MediaRepository

    init() {
        let services = (try? AppServices.make()) ?? {
            preconditionFailure("Failed to assemble AppServices")
        }()
        let store = AppStoreFactory.make(services: services)
        _store = State(initialValue: store)
        media = services.media
        // AppDelegate 在 init 之后由系统注入；在 body.task 里再绑 store/services。
        AppBootstrap.shared.services = services
        AppBootstrap.shared.store = store
    }

    var body: some Scene {
        WindowGroup {
            RootView()
                .provideStore(store)
                .environment(\.mediaRepository, media)
                .task {
                    appDelegate.store = AppBootstrap.shared.store
                    appDelegate.services = AppBootstrap.shared.services
                    appDelegate.requestNotificationAuthorizationAndRegister()
                    store.dispatch(.auth(.bootstrap))
                }
                .onChange(of: scenePhase) { _, phase in
                    switch phase {
                    case .active:
                        store.dispatch(.chat(.sceneBecameActive))
                    case .background:
                        store.dispatch(.chat(.sceneBecameBackground))
                    case .inactive:
                        break
                    @unknown default:
                        break
                    }
                }
        }
    }
}

/// 让 AppDelegate 与 SwiftUI App init 共享同一次组装的 services/store。
enum AppBootstrap {
    final class Box: @unchecked Sendable {
        var services: AppServices?
        var store: StoreOfApp?
    }
    static let shared = Box()
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
