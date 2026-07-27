import Foundation
import ChatDomain
import ChatInfrastructure

/// 应用组合根：Live 装配点；Executor 依赖以 `any Port` 注入。
public struct AppServices: Sendable {
    /// 投影 / 分页等尚未收进 Port 的表面仍走 Live DB。
    public let database: LocalDatabase
    public let messageStore: any MessageStore
    public let conversationStore: any ConversationStore
    public let syncCursorStore: any SyncCursorStore
    public let messageRemote: any MessageRemote
    public let conversationRemote: any ConversationRemote
    public let syncRemote: any SyncRemote
    public let auth: AuthRepositoryLive
    public let http: HTTPClient
    public let session: SessionStore
    public let sendExecutor: MessageSendExecutor
    public let syncExecutor: SyncExecutor
    public let realtime: RealtimeSession
    public let gatewayWSURL: URL
    public let realtimeListenGate: RealtimeListenGate
    public let pushTokenAPI: PushTokenAPI
    public let silentWake: SilentSyncWakeHandler
    public let projections: LocalProjectionObserver
    public let pathResume: SendPathResumeMonitor
    public let gapBackfill: ConversationGapBackfill
    public let media: any MediaRepository

    /// 兼容旧调用方命名（类型已是 Port）。
    public var conversations: any ConversationRemote { conversationRemote }
    public var messages: any MessageRemote { messageRemote }

    public init(
        database: LocalDatabase,
        messageStore: any MessageStore,
        conversationStore: any ConversationStore,
        syncCursorStore: any SyncCursorStore,
        messageRemote: any MessageRemote,
        conversationRemote: any ConversationRemote,
        syncRemote: any SyncRemote,
        auth: AuthRepositoryLive,
        http: HTTPClient,
        session: SessionStore,
        sendExecutor: MessageSendExecutor,
        syncExecutor: SyncExecutor,
        realtime: RealtimeSession,
        gatewayWSURL: URL,
        realtimeListenGate: RealtimeListenGate = RealtimeListenGate(),
        pushTokenAPI: PushTokenAPI,
        silentWake: SilentSyncWakeHandler,
        projections: LocalProjectionObserver,
        pathResume: SendPathResumeMonitor,
        gapBackfill: ConversationGapBackfill,
        media: any MediaRepository
    ) {
        self.database = database
        self.messageStore = messageStore
        self.conversationStore = conversationStore
        self.syncCursorStore = syncCursorStore
        self.messageRemote = messageRemote
        self.conversationRemote = conversationRemote
        self.syncRemote = syncRemote
        self.auth = auth
        self.http = http
        self.session = session
        self.sendExecutor = sendExecutor
        self.syncExecutor = syncExecutor
        self.realtime = realtime
        self.gatewayWSURL = gatewayWSURL
        self.realtimeListenGate = realtimeListenGate
        self.pushTokenAPI = pushTokenAPI
        self.silentWake = silentWake
        self.projections = projections
        self.pathResume = pathResume
        self.gapBackfill = gapBackfill
        self.media = media
    }

    // MARK: - Port assembly（唯一 Live / Fake 装配缝）

    public static func assembleSendExecutor(
        store: any MessageStore,
        remote: any MessageRemote,
        sendingTimeoutNanoseconds: UInt64 = MessageSendExecutor.sendingTimeoutNanoseconds
    ) -> MessageSendExecutor {
        MessageSendExecutor(
            store: store,
            remote: remote,
            sendingTimeoutNanoseconds: sendingTimeoutNanoseconds
        )
    }

    public static func assembleSyncExecutor(
        cursorStore: any SyncCursorStore,
        remote: any SyncRemote,
        messageStore: any MessageStore,
        conversationStore: any ConversationStore,
        session: SessionStore
    ) -> SyncExecutor {
        SyncExecutor(
            cursorStore: cursorStore,
            remote: remote,
            messageStore: messageStore,
            conversationStore: conversationStore,
            session: session
        )
    }

    /// 生产组合根：Live Port → Executor。
    public static func make(
        apiBaseURL: URL = URL(string: "http://127.0.0.1:8080")!,
        gatewayWSURL: URL = URL(string: "ws://127.0.0.1:8081/ws")!
    ) throws -> AppServices {
        let http = HTTPClient(config: APIConfig(baseURL: apiBaseURL))
        let session = SessionStore()
        let auth = AuthRepositoryLive(http: http, session: session)
        let db = try LocalDatabase.applicationDefault()
        let conversationRemote: any ConversationRemote = ConversationAPI(http: http, session: session)
        let messageRemote: any MessageRemote = MessageAPI(http: http, session: session)
        let syncRemote: any SyncRemote = SyncAPI(http: http, session: session)
        let sendExecutor = assembleSendExecutor(store: db, remote: messageRemote)
        let syncExecutor = assembleSyncExecutor(
            cursorStore: db,
            remote: syncRemote,
            messageStore: db,
            conversationStore: db,
            session: session
        )
        let realtime = RealtimeSession(gatewayURL: gatewayWSURL, database: db, session: session)
        let pushTokenAPI = PushTokenAPI(http: http, session: session)
        let silentWake = SilentSyncWakeHandler(syncExecutor: syncExecutor)
        let projections = LocalProjectionObserver(database: db)
        let pathResume = SendPathResumeMonitor(sendExecutor: sendExecutor)
        let conversationMessages = ConversationMessagesAPI(http: http, session: session)
        let gapBackfill = ConversationGapBackfill(
            database: db,
            api: conversationMessages,
            session: session
        )
        let media: any MediaRepository = MediaAPI(http: http, session: session)
        return AppServices(
            database: db,
            messageStore: db,
            conversationStore: db,
            syncCursorStore: db,
            messageRemote: messageRemote,
            conversationRemote: conversationRemote,
            syncRemote: syncRemote,
            auth: auth,
            http: http,
            session: session,
            sendExecutor: sendExecutor,
            syncExecutor: syncExecutor,
            realtime: realtime,
            gatewayWSURL: gatewayWSURL,
            pushTokenAPI: pushTokenAPI,
            silentWake: silentWake,
            projections: projections,
            pathResume: pathResume,
            gapBackfill: gapBackfill,
            media: media
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
