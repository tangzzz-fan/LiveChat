import Foundation

/// 注册/更新当前 JWT 设备的 APNs push token（服务端从 token 取 device_id）。
public final class PushTokenAPI: Sendable {
    private let http: HTTPClient
    private let session: SessionStore

    public init(http: HTTPClient, session: SessionStore) {
        self.http = http
        self.session = session
    }

    public func register(pushToken: String, platform: String = "ios") async throws {
        guard let creds = try session.load() else {
            throw HTTPClientError.status(code: 401, body: "not logged in")
        }
        struct Body: Encodable {
            let push_token: String
            let platform: String
        }
        struct Resp: Decodable {
            let updated: Bool?
        }
        let _: Resp = try await http.postJSON(
            path: "/v1/devices/push-token",
            body: Body(push_token: pushToken, platform: platform),
            bearerToken: creds.accessToken
        )
    }
}
