import SwiftUI
import Foundation
import TGReduxKit
import ChatPresentation
import ChatApplication
import ChatInfrastructure

typealias StoreOfApp = Store<AppState, AppAction>

/// 薄 App 入口样板：创建 Xcode 工程后，把本文件加入 App target（替换默认 `*App.swift`）。
@main
struct LiveChatApp: App {
    @State private var store = AppStoreFactory.make()

    var body: some Scene {
        WindowGroup {
            RootView()
                .provideStore(store)
                .task {
                    store.dispatch(.bootstrap)
                    _ = try? AppServices.makeScaffold(
                        gatewayWSURL: URL(string: "ws://127.0.0.1:8081/ws")!
                    )
                    _ = ProtobufScaffold.libraryLinked
                }
        }
    }
}

struct RootView: View {
    @Environment(StoreOfApp.self) private var store

    var body: some View {
        VStack(spacing: 12) {
            Text("LiveChat")
                .font(.largeTitle.weight(.semibold))
            Text("SPM scaffold ready")
                .foregroundStyle(.secondary)
            if let banner = store.state.connectionBanner {
                Text(banner)
                    .font(.footnote)
            }
        }
        .padding()
    }
}
