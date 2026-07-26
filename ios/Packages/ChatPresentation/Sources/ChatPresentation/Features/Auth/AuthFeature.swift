import Foundation
import TGReduxKit

public struct AuthState: Equatable, Sendable {
    public var isLoggedIn: Bool
    public var userID: Int64?
    public var deviceID: String?
    public var phoneInput: String
    public var codeInput: String
    public var phase: Phase
    public var errorMessage: String?
    public var isBusy: Bool
    public var deviceSummaries: [String]
    public var deviceBanner: String?

    public enum Phase: String, Equatable, Sendable {
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
        phase: Phase = .phone,
        errorMessage: String? = nil,
        isBusy: Bool = false,
        deviceSummaries: [String] = [],
        deviceBanner: String? = nil
    ) {
        self.isLoggedIn = isLoggedIn
        self.userID = userID
        self.deviceID = deviceID
        self.phoneInput = phoneInput
        self.codeInput = codeInput
        self.phase = phase
        self.errorMessage = errorMessage
        self.isBusy = isBusy
        self.deviceSummaries = deviceSummaries
        self.deviceBanner = deviceBanner
    }
}

public enum AuthAction: Sendable {
    case bootstrap
    case setPhoneInput(String)
    case setCodeInput(String)
    case requestCodeTapped
    case verifyCodeTapped
    case logoutTapped
    case refreshDevicesTapped
    case resetToPhone

    case busy(Bool)
    case failed(String)
    case codeRequested
    case sessionRestored(userID: Int64, deviceID: String)
    case loginSucceeded(userID: Int64, deviceID: String)
    case loggedOut
    case devicesLoaded([String])
    case setDeviceBanner(String?)
}

@MainActor
public enum AuthFeature {
    public static let reducer: Reducer<AuthState, AuthAction> = { state, action in
        switch action {
        case .bootstrap, .requestCodeTapped, .verifyCodeTapped, .logoutTapped, .refreshDevicesTapped:
            break
        case .resetToPhone:
            state.phase = .phone
            state.codeInput = ""
            state.errorMessage = nil
            state.isBusy = false
        case .setPhoneInput(let phone):
            state.phoneInput = phone
            state.errorMessage = nil
        case .setCodeInput(let code):
            state.codeInput = code
            state.errorMessage = nil
        case .busy(let busy):
            state.isBusy = busy
        case .failed(let message):
            state.errorMessage = message
            state.isBusy = false
        case .codeRequested:
            state.phase = .code
            state.isBusy = false
            state.errorMessage = nil
        case .sessionRestored(let userID, let deviceID), .loginSucceeded(let userID, let deviceID):
            state.isLoggedIn = true
            state.userID = userID
            state.deviceID = deviceID
            state.phase = .loggedIn
            state.isBusy = false
            state.errorMessage = nil
            state.codeInput = ""
        case .loggedOut:
            state.isLoggedIn = false
            state.userID = nil
            state.phase = .phone
            state.codeInput = ""
            state.deviceSummaries = []
            state.isBusy = false
        case .devicesLoaded(let lines):
            state.deviceSummaries = lines
        case .setDeviceBanner(let banner):
            state.deviceBanner = banner
        }
    }
}
