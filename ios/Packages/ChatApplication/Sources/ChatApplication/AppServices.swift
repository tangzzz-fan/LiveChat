import Foundation
import ChatDomain
import ChatInfrastructure

public struct AppServices: Sendable {
    public let database: LocalDatabase
    public let auth: AuthRepositoryLive
    public let http: HTTPClient
    public let session: SessionStore
    public let conversations: ConversationAPI
    public let messages: MessageAPI
    public let sendExecutor: MessageSendExecutor
    public let syncExecutor: SyncExecutor
    public let realtime: RealtimeSession
    public let gatewayWSURL: URL
    public let realtimeListenGate: RealtimeListenGate
    public let pushTokenAPI: PushTokenAPI
    public let silentWake: SilentSyncWakeHandler

    public init(
        database: LocalDatabase,
        auth: AuthRepositoryLive,
        http: HTTPClient,
        session: SessionStore,
        conversations: ConversationAPI,
        messages: MessageAPI,
        sendExecutor: MessageSendExecutor,
        syncExecutor: SyncExecutor,
        realtime: RealtimeSession,
        gatewayWSURL: URL,
        realtimeListenGate: RealtimeListenGate = RealtimeListenGate(),
        pushTokenAPI: PushTokenAPI,
        silentWake: SilentSyncWakeHandler
    ) {
        self.database = database
        self.auth = auth
        self.http = http
        self.session = session
        self.conversations = conversations
        self.messages = messages
        self.sendExecutor = sendExecutor
        self.syncExecutor = syncExecutor
        self.realtime = realtime
        self.gatewayWSURL = gatewayWSURL
        self.realtimeListenGate = realtimeListenGate
        self.pushTokenAPI = pushTokenAPI
        self.silentWake = silentWake
    }

    public static func make(
        apiBaseURL: URL = URL(string: "http://127.0.0.1:8080")!,
        gatewayWSURL: URL = URL(string: "ws://127.0.0.1:8081/ws")!
    ) throws -> AppServices {
        let http = HTTPClient(config: APIConfig(baseURL: apiBaseURL))
        let session = SessionStore()
        let auth = AuthRepositoryLive(http: http, session: session)
        let db = try LocalDatabase.applicationDefault()
        let conversations = ConversationAPI(http: http, session: session)
        let messages = MessageAPI(http: http, session: session)
        let sendExecutor = MessageSendExecutor(database: db, api: messages)
        let syncAPI = SyncAPI(http: http, session: session)
        let syncExecutor = SyncExecutor(database: db, api: syncAPI, session: session)
        let realtime = RealtimeSession(gatewayURL: gatewayWSURL, database: db, session: session)
        let pushTokenAPI = PushTokenAPI(http: http, session: session)
        let silentWake = SilentSyncWakeHandler(syncExecutor: syncExecutor)
        return AppServices(
            database: db,
            auth: auth,
            http: http,
            session: session,
            conversations: conversations,
            messages: messages,
            sendExecutor: sendExecutor,
            syncExecutor: syncExecutor,
            realtime: realtime,
            gatewayWSURL: gatewayWSURL,
            pushTokenAPI: pushTokenAPI,
            silentWake: silentWake
        )
    }
}

public final class RealtimeListenGate: @unchecked Sendable {
    private let lock = NSLock()
    private var started = false

    public init() {}

    public func beginIfNeeded() -> Bool {
        lock.lock()
        defer { lock.unlock() }
        if started { return false }
        started = true
        return true
    }
}
