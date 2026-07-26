import Foundation

public enum SessionKeys {
    public static let deviceID = "device_id"
    public static let accessToken = "access_token"
    public static let refreshToken = "refresh_token"
    public static let userID = "user_id"
}

public struct SessionCredentials: Equatable, Sendable {
    public let accessToken: String
    public let refreshToken: String
    public let userID: Int64
    public let deviceID: String

    public init(accessToken: String, refreshToken: String, userID: Int64, deviceID: String) {
        self.accessToken = accessToken
        self.refreshToken = refreshToken
        self.userID = userID
        self.deviceID = deviceID
    }
}

public final class SessionStore: Sendable {
    private let keychain: KeychainStore

    public init(keychain: KeychainStore = KeychainStore()) {
        self.keychain = keychain
    }

    public func deviceID() throws -> String {
        if let existing = try keychain.string(forKey: SessionKeys.deviceID), !existing.isEmpty {
            return existing
        }
        let id = "ios-" + UUID().uuidString.lowercased()
        try keychain.set(id, forKey: SessionKeys.deviceID)
        return id
    }

    public func load() throws -> SessionCredentials? {
        guard
            let access = try keychain.string(forKey: SessionKeys.accessToken),
            let refresh = try keychain.string(forKey: SessionKeys.refreshToken),
            let userRaw = try keychain.string(forKey: SessionKeys.userID),
            let userID = Int64(userRaw),
            let deviceID = try keychain.string(forKey: SessionKeys.deviceID)
        else {
            return nil
        }
        return SessionCredentials(
            accessToken: access,
            refreshToken: refresh,
            userID: userID,
            deviceID: deviceID
        )
    }

    public func save(_ credentials: SessionCredentials) throws {
        try keychain.set(credentials.accessToken, forKey: SessionKeys.accessToken)
        try keychain.set(credentials.refreshToken, forKey: SessionKeys.refreshToken)
        try keychain.set(String(credentials.userID), forKey: SessionKeys.userID)
        try keychain.set(credentials.deviceID, forKey: SessionKeys.deviceID)
    }

    public func clearTokens() throws {
        try keychain.delete(SessionKeys.accessToken)
        try keychain.delete(SessionKeys.refreshToken)
        try keychain.delete(SessionKeys.userID)
        // Keep device_id across logout so re-login reuses stable device.
    }
}
