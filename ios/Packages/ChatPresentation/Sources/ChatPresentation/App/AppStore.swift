import Foundation
import TGReduxKit
import ChatApplication

public struct AppState: Equatable, Sendable {
    public var auth: AuthState
    public var chat: ChatState

    public init(auth: AuthState = AuthState(), chat: ChatState = ChatState()) {
        self.auth = auth
        self.chat = chat
    }

    public var isLoggedIn: Bool { auth.isLoggedIn }
}

public enum AppAction: Sendable {
    case auth(AuthAction)
    case chat(ChatAction)
}

@MainActor
public enum AppStoreFactory {
    public static let reducer: Reducer<AppState, AppAction> = combineReducers(
        pullback(
            AuthFeature.reducer,
            state: \.auth,
            extract: { if case .auth(let a) = $0 { a } else { nil } }
        ),
        pullback(
            ChatFeature.reducer,
            state: \.chat,
            extract: { if case .chat(let a) = $0 { a } else { nil } }
        ),
        { state, action in
            if case .auth(.loggedOut) = action {
                state.chat = ChatState()
            }
        }
    )

    public static func make(services: AppServices) -> Store<AppState, AppAction> {
        Store(
            initialState: AppState(),
            reducer: reducer,
            middlewares: [
                makeAuthMiddleware(services: services),
                makeChatMiddleware(services: services),
            ]
        )
    }

    public static func make() -> Store<AppState, AppAction> {
        Store(initialState: AppState(), reducer: reducer, middlewares: [])
    }
}
