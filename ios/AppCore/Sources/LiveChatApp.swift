import SwiftUI
import ChatDomain

/// @main App 入口（Spec 13 §8.1）
@main
struct LiveChatApp: App {
    init() {
        AppContainer.shared.initialize()
    }

    var body: some Scene {
        WindowGroup {
            Text("LiveChat iOS")
                .font(.largeTitle)
                .onAppear {
                    print("[LiveChatApp] didFinishLaunching — modules loaded")
                }
        }
    }
}
