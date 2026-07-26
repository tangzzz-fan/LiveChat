import Foundation
import TGReduxKit
import ChatApplication
import ChatInfrastructure

/// 视图真相（Spec 13 §4.3）：不含全量消息数组。
public struct AppState: Equatable, Sendable {
    public var isLoggedIn: Bool
    public var userID: Int64?
    public var deviceID: String?
    public var phoneInput: String
    public var codeInput: String
    public var authPhase: AuthPhase
    public var authError: String?
    public var isAuthBusy: Bool
    public var deviceSummaries: [String]
    public var connectionBanner: String?
    public var visibleMessageIDs: [String]
    public var composeDraft: String

    public enum AuthPhase: String, Equatable, Sendable {
        case phone
        case code
        case loggedIn
    }

    public init(
        isLoggedIn: Bool = false,
        userID: Int64? = nil,
        deviceID: String? = nil,
        phoneInput: String = "",
        codeInput: String = "",
        authPhase: AuthPhase = .phone,
        authError: String? = nil,
        isAuthBusy: Bool = false,
        deviceSummaries: [String] = [],
        connectionBanner: String? = nil,
        visibleMessageIDs: [String] = [],
        composeDraft: String = ""
    ) {
        self.isLoggedIn = isLoggedIn
        self.userID = userID
        self.deviceID = deviceID
        self.phoneInput = phoneInput
        self.codeInput = codeInput
        self.authPhase = authPhase
        self.authError = authError
        self.isAuthBusy = isAuthBusy
        self.deviceSummaries = deviceSummaries
        self.connectionBanner = connectionBanner
        self.visibleMessageIDs = visibleMessageIDs
        self.composeDraft = composeDraft
    }
}

public enum AppAction: Sendable {
    case bootstrap
    case setPhoneInput(String)
    case setCodeInput(String)
    case requestCodeTapped
    case verifyCodeTapped
    case logoutTapped
    case refreshDevicesTapped
    case resetAuthToPhone

    case authBusy(Bool)
    case authFailed(String)
    case codeRequested
    case sessionRestored(userID: Int64, deviceID: String)
    case loginSucceeded(userID: Int64, deviceID: String)
    case loggedOut
    case devicesLoaded([String])

    case setConnectionBanner(String?)
    case setVisibleMessageIDs([String])
    case updateDraft(String)
}

@MainActor
public enum AppStoreFactory {
    public static let reducer: Reducer<AppState, AppAction> = { state, action in
        switch action {
        case .bootstrap, .requestCodeTapped, .verifyCodeTapped, .logoutTapped, .refreshDevicesTapped:
            break
        case .resetAuthToPhone:
            state.authPhase = .phone
            state.codeInput = ""
            state.authError = nil
            state.isAuthBusy = false
        case .setPhoneInput(let phone):
            state.phoneInput = phone
            state.authError = nil
        case .setCodeInput(let code):
            state.codeInput = code
            state.authError = nil
        case .authBusy(let busy):
            state.isAuthBusy = busy
        case .authFailed(let message):
            state.authError = message
            state.isAuthBusy = false
        case .codeRequested:
            state.authPhase = .code
            state.isAuthBusy = false
            state.authError = nil
        case .sessionRestored(let userID, let deviceID), .loginSucceeded(let userID, let deviceID):
            state.isLoggedIn = true
            state.userID = userID
            state.deviceID = deviceID
            state.authPhase = .loggedIn
            state.isAuthBusy = false
            state.authError = nil
            state.codeInput = ""
        case .loggedOut:
            state.isLoggedIn = false
            state.userID = nil
            state.authPhase = .phone
            state.codeInput = ""
            state.deviceSummaries = []
            state.isAuthBusy = false
        case .devicesLoaded(let lines):
            state.deviceSummaries = lines
        case .setConnectionBanner(let banner):
            state.connectionBanner = banner
        case .setVisibleMessageIDs(let ids):
            state.visibleMessageIDs = ids
        case .updateDraft(let text):
            state.composeDraft = text
        }
    }

    public static func make(services: AppServices) -> Store<AppState, AppAction> {
        Store(
            initialState: AppState(),
            reducer: reducer,
            middlewares: [makeAuthMiddleware(services: services)]
        )
    }

    /// 无依赖装配（仅单测 / 脚手架）。
    public static func make() -> Store<AppState, AppAction> {
        Store(initialState: AppState(), reducer: reducer, middlewares: [])
    }

    public static func makeAuthMiddleware(services: AppServices) -> Middleware<AppState, AppAction> {
        { store, action, next in
            next(action)
            switch action {
            case .bootstrap:
                store.runTask(id: CancellationID("auth.bootstrap")) {
                    do {
                        let deviceID = try services.auth.currentDeviceID()
                        if let session = try services.auth.restoredSession() {
                            await store.dispatch(
                                .sessionRestored(userID: session.userID, deviceID: deviceID)
                            )
                            await loadDevices(store: store, services: services, token: session.accessToken)
                        } else {
                            await store.dispatch(.setConnectionBanner("device: \(deviceID)"))
                        }
                    } catch {
                        await store.dispatch(.authFailed(error.localizedDescription))
                    }
                }
            case .requestCodeTapped:
                let phone = store.state.phoneInput.trimmingCharacters(in: .whitespacesAndNewlines)
                store.runTask(id: CancellationID("auth.requestCode")) {
                    await store.dispatch(.authBusy(true))
                    do {
                        _ = try await services.auth.requestCode(phone: phone)
                        await store.dispatch(.codeRequested)
                    } catch {
                        await store.dispatch(.authFailed(error.localizedDescription))
                    }
                }
            case .verifyCodeTapped:
                let phone = store.state.phoneInput.trimmingCharacters(in: .whitespacesAndNewlines)
                let code = store.state.codeInput.trimmingCharacters(in: .whitespacesAndNewlines)
                store.runTask(id: CancellationID("auth.verifyCode")) {
                    await store.dispatch(.authBusy(true))
                    do {
                        let deviceID = try services.auth.currentDeviceID()
                        let tokens = try await services.auth.verifyCode(
                            phone: phone,
                            code: code,
                            deviceID: deviceID,
                            platform: "ios"
                        )
                        await store.dispatch(
                            .loginSucceeded(userID: tokens.userID, deviceID: deviceID)
                        )
                        await loadDevices(store: store, services: services, token: tokens.accessToken)
                    } catch {
                        await store.dispatch(.authFailed(error.localizedDescription))
                    }
                }
            case .logoutTapped:
                store.runTask(id: CancellationID("auth.logout")) {
                    try? await services.auth.logout()
                    await store.dispatch(.loggedOut)
                }
            case .refreshDevicesTapped:
                store.runTask(id: CancellationID("auth.devices")) {
                    guard let session = try? services.auth.restoredSession() else { return }
                    await loadDevices(store: store, services: services, token: session.accessToken)
                }
            default:
                break
            }
        }
    }

    private static func loadDevices(
        store: Store<AppState, AppAction>,
        services: AppServices,
        token: String
    ) async {
        do {
            let devices = try await services.auth.listDevices(accessToken: token)
            let lines = devices.map { d in
                let mark = (d.isCurrent == true) ? " *" : ""
                return "\(d.deviceID) (\(d.platform)) sv=\(d.sessionVersion)\(mark)"
            }
            await store.dispatch(.devicesLoaded(lines))
        } catch {
            await store.dispatch(.authFailed(error.localizedDescription))
        }
    }
}
