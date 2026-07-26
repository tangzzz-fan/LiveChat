import Foundation
import ChatDomain
import ChatInfrastructure

/// 应用层组装入口。
public struct AppServices: Sendable {
    public let database: LocalDatabase
    public let transport: any WebSocketTransport
    public let auth: AuthRepositoryLive
    public let http: HTTPClient
    public let session: SessionStore

    public init(
        database: LocalDatabase,
        transport: any WebSocketTransport,
        auth: AuthRepositoryLive,
        http: HTTPClient,
        session: SessionStore
    ) {
        self.database = database
        self.transport = transport
        self.auth = auth
        self.http = http
        self.session = session
    }

    public static func make(
        apiBaseURL: URL = URL(string: "http://127.0.0.1:8080")!,
        gatewayWSURL: URL = URL(string: "ws://127.0.0.1:8081/ws")!
    ) throws -> AppServices {
        let http = HTTPClient(config: APIConfig(baseURL: apiBaseURL))
        let session = SessionStore()
        let auth = AuthRepositoryLive(http: http, session: session)
        let db = try LocalDatabase.inMemory()
        let transport = URLSessionWebSocketTransport(url: gatewayWSURL)
        return AppServices(
            database: db,
            transport: transport,
            auth: auth,
            http: http,
            session: session
        )
    }

    @available(*, deprecated, renamed: "make(apiBaseURL:gatewayWSURL:)")
    public static func makeScaffold(gatewayWSURL: URL) throws -> AppServices {
        try make(gatewayWSURL: gatewayWSURL)
    }
}
