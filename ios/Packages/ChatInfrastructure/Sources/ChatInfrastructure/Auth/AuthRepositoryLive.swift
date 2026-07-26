import Foundation
import ChatDomain

public struct DeviceInfoDTO: Decodable, Sendable, Equatable {
    public let deviceID: String
    public let platform: String
    public let sessionVersion: Int
    public let isCurrent: Bool?

    enum CodingKeys: String, CodingKey {
        case deviceID = "device_id"
        case platform
        case sessionVersion = "session_version"
        case isCurrent = "is_current"
    }
}

public final class AuthRepositoryLive: AuthRepository, @unchecked Sendable {
    private let http: HTTPClient
    private let session: SessionStore

    public init(http: HTTPClient, session: SessionStore = SessionStore()) {
        self.http = http
        self.session = session
    }

    public func requestCode(phone: String) async throws -> CodeRequestResponse {
        struct Body: Encodable { let phone_e164: String }
        struct Resp: Decodable {
            let retry_after_sec: Int
            let expires_in_sec: Int
        }
        let resp: Resp = try await http.postJSON(
            path: "/v1/auth/request_code",
            body: Body(phone_e164: phone)
        )
        return CodeRequestResponse(
            retryAfterSec: resp.retry_after_sec,
            expiresInSec: resp.expires_in_sec
        )
    }

    public func verifyCode(
        phone: String,
        code: String,
        deviceID: String,
        platform: String
    ) async throws -> AuthTokens {
        struct Body: Encodable {
            let phone_e164: String
            let verification_code: String
            let device_id: String
            let platform: String
        }
        struct Resp: Decodable {
            let access_token: String
            let refresh_token: String
            let expires_in: Int64
            let user_id: Int64
        }
        let resp: Resp = try await http.postJSON(
            path: "/v1/auth/verify_code",
            body: Body(
                phone_e164: phone,
                verification_code: code,
                device_id: deviceID,
                platform: platform
            )
        )
        let tokens = AuthTokens(
            accessToken: resp.access_token,
            refreshToken: resp.refresh_token,
            userID: resp.user_id,
            expiresIn: resp.expires_in
        )
        try session.save(
            SessionCredentials(
                accessToken: tokens.accessToken,
                refreshToken: tokens.refreshToken,
                userID: tokens.userID,
                deviceID: deviceID
            )
        )
        return tokens
    }

    public func refreshToken(_ token: String) async throws -> AuthTokens {
        struct Body: Encodable { let refresh_token: String }
        struct Resp: Decodable {
            let access_token: String
            let refresh_token: String
            let expires_in: Int64?
            let user_id: Int64?
        }
        let resp: Resp = try await http.postJSON(
            path: "/v1/auth/refresh",
            body: Body(refresh_token: token)
        )
        let existing = try session.load()
        let userID = resp.user_id ?? existing?.userID ?? 0
        let deviceID = try session.deviceID()
        let tokens = AuthTokens(
            accessToken: resp.access_token,
            refreshToken: resp.refresh_token,
            userID: userID,
            expiresIn: resp.expires_in ?? 3600
        )
        try session.save(
            SessionCredentials(
                accessToken: tokens.accessToken,
                refreshToken: tokens.refreshToken,
                userID: tokens.userID,
                deviceID: deviceID
            )
        )
        return tokens
    }

    public func logout() async throws {
        try session.clearTokens()
    }

    public func currentDeviceID() throws -> String {
        try session.deviceID()
    }

    public func restoredSession() throws -> SessionCredentials? {
        try session.load()
    }

    public func listDevices(accessToken: String) async throws -> [DeviceInfoDTO] {
        struct Resp: Decodable {
            let devices: [DeviceInfoDTO]
        }
        let wrapped: Resp = try await http.getJSON(
            path: "/v1/devices",
            bearerToken: accessToken
        )
        return wrapped.devices
    }
}
