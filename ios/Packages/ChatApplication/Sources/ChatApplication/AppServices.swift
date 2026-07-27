import Foundation
import ChatDomain
import ChatInfrastructure

public struct AppServices: Sendable {
    public let database: LocalDatabase
    public let transport: any WebSocketTransport
    public let auth: AuthRepositoryLive
    public let http: HTTPClient
    public let session: SessionStore
    public let conversations: ConversationAPI
    public let messages: MessageAPI
    public let sendExecutor: MessageSendExecutor
    public let syncExecutor: SyncExecutor

    public init(
        database: LocalDatabase,
        transport: any WebSocketTransport,
        auth: AuthRepositoryLive,
        http: HTTPClient,
        session: SessionStore,
        conversations: ConversationAPI,
        messages: MessageAPI,
        sendExecutor: MessageSendExecutor,
        syncExecutor: SyncExecutor
    ) {
        self.database = database
        self.transport = transport
        self.auth = auth
        self.http = http
        self.session = session
        self.conversations = conversations
        self.messages = messages
        self.sendExecutor = sendExecutor
        self.syncExecutor = syncExecutor
    }

    public static func make(
        apiBaseURL: URL = URL(string: "http://127.0.0.1:8080")!,
        gatewayWSURL: URL = URL(string: "ws://127.0.0.1:8081/ws")!
    ) throws -> AppServices {
        let http = HTTPClient(config: APIConfig(baseURL: apiBaseURL))
        let session = SessionStore()
        let auth = AuthRepositoryLive(http: http, session: session)
        let db = try LocalDatabase.applicationDefault()
        let transport = URLSessionWebSocketTransport(url: gatewayWSURL)
        let conversations = ConversationAPI(http: http, session: session)
        let messages = MessageAPI(http: http, session: session)
        let sendExecutor = MessageSendExecutor(database: db, api: messages)
        let syncAPI = SyncAPI(http: http, session: session)
        let syncExecutor = SyncExecutor(database: db, api: syncAPI, session: session)
        return AppServices(
            database: db,
            transport: transport,
            auth: auth,
            http: http,
            session: session,
            conversations: conversations,
            messages: messages,
            sendExecutor: sendExecutor,
            syncExecutor: syncExecutor
        )
    }
}
