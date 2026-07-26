import Foundation
import TGReduxKit

/// 视图真相（Spec 13 §4.3）：不含全量消息数组。
public struct AppState: Equatable, Sendable {
    public var isLoggedIn: Bool
    public var connectionBanner: String?
    /// 当前会话可见窗口投影（分页）；不是 DB 全量。
    public var visibleMessageIDs: [String]
    public var composeDraft: String

    public init(
        isLoggedIn: Bool = false,
        connectionBanner: String? = nil,
        visibleMessageIDs: [String] = [],
        composeDraft: String = ""
    ) {
        self.isLoggedIn = isLoggedIn
        self.connectionBanner = connectionBanner
        self.visibleMessageIDs = visibleMessageIDs
        self.composeDraft = composeDraft
    }
}

public enum AppAction: Sendable {
    case bootstrap
    case setLoggedIn(Bool)
    case setConnectionBanner(String?)
    case setVisibleMessageIDs([String])
    case updateDraft(String)
}

@MainActor
public enum AppStoreFactory {
    public static let reducer: Reducer<AppState, AppAction> = { state, action in
        switch action {
        case .bootstrap:
            break
        case .setLoggedIn(let value):
            state.isLoggedIn = value
        case .setConnectionBanner(let banner):
            state.connectionBanner = banner
        case .setVisibleMessageIDs(let ids):
            state.visibleMessageIDs = ids
        case .updateDraft(let text):
            state.composeDraft = text
        }
    }

    public static func make() -> Store<AppState, AppAction> {
        Store(initialState: AppState(), reducer: reducer, middlewares: [])
    }
}
