import Foundation
import Network

public enum RealtimeStatus: Equatable, Sendable {
    case idle
    case connecting
    case connected(sessionID: String)
    case reconnecting(attempt: Int)
    case disconnected(reason: String)
}

public enum RealtimeEvent: Sendable {
    case status(RealtimeStatus)
    /// 批量落库后的会话 id；UI 据此刷新投影，勿在每帧 dispatch。
    case databaseChanged(conversationIDs: [String])
}

/// 前台长连接：握手、心跳、MESSAGE_DELIVERY 批量落库、指数退避重连（single-flight）。
public actor RealtimeSession {
    public static let deliveryFlushIntervalMs: UInt64 = 24

    private let gatewayURL: URL
    private let database: LocalDatabase
    private let session: SessionStore
    private let makeTransport: @Sendable (URL) -> any WebSocketTransport

    private let eventContinuation: AsyncStream<RealtimeEvent>.Continuation
    public let events: AsyncStream<RealtimeEvent>

    private var desiredRunning = false
    private var connectTask: Task<Void, Never>?
    private var transport: (any WebSocketTransport)?
    private var heartbeatTask: Task<Void, Never>?
    private var readerTask: Task<Void, Never>?
    private var flushTask: Task<Void, Never>?
    private var pathMonitor: NWPathMonitor?

    private var pendingDeliveries: [Livechat_Ws_WsMessageDelivery] = []
    private var reconnectAttempt = 0
    private var seq: UInt64 = 0
    private var myUserID: Int64 = 0
    private var heartbeatInterval: TimeInterval = 30
    private var shouldReconnectOnClose = true
    private var handshakeWaiter: CheckedContinuation<Livechat_Ws_HandshakeResponse, Error>?
    /// 已成功发出的 read ACK 水位，避免 observation 抖动重复上报。
    private var lastReadAckByConversation: [String: Int64] = [:]

    public init(
        gatewayURL: URL,
        database: LocalDatabase,
        session: SessionStore,
        makeTransport: @escaping @Sendable (URL) -> any WebSocketTransport = {
            URLSessionWebSocketTransport(url: $0)
        }
    ) {
        self.gatewayURL = gatewayURL
        self.database = database
        self.session = session
        self.makeTransport = makeTransport
        let pair = AsyncStream.makeStream(of: RealtimeEvent.self)
        self.events = pair.stream
        self.eventContinuation = pair.continuation
    }

    public func start() {
        desiredRunning = true
        shouldReconnectOnClose = true
        startPathMonitorIfNeeded()
        scheduleConnect(immediate: true)
    }

    public func stop(reason: String = "stopped") {
        desiredRunning = false
        shouldReconnectOnClose = false
        connectTask?.cancel()
        connectTask = nil
        failHandshakeWaiter(RealtimeError.closed(reason: reason))
        tearDownConnection()
        pathMonitor?.cancel()
        pathMonitor = nil
        eventContinuation.yield(.status(.disconnected(reason: reason)))
    }

    /// 打开会话 / 可见窗刷新时：发 ACK(read)。未连接或 seq=0 / 水位未变时静默跳过。
    public func sendReadAck(conversationID: String, lastReadSeq: Int64) async {
        guard lastReadSeq > 0, let transport else { return }
        if lastReadAckByConversation[conversationID] == lastReadSeq { return }
        do {
            seq += 1
            var ack = Livechat_Ws_MessageAck()
            ack.ackType = "read"
            ack.conversationID = conversationID
            ack.lastReadSeq = UInt64(lastReadSeq)
            ack.eventSeq = UInt64(lastReadSeq)
            ack.ackedAtMs = UInt64(Date().timeIntervalSince1970 * 1000)
            try await transport.send(
                try WsCodec.encodeFrame(
                    opcode: WsOpcode.ack,
                    payload: ack,
                    seqID: seq
                )
            )
            lastReadAckByConversation[conversationID] = lastReadSeq
        } catch {
            // 已读可依赖下次打开 / sync 再发；勿打断读循环
        }
    }

    /// 本地清未读（仅当未读>0）+（有 seq 时）发 read ACK。
    public func markConversationRead(conversationID: String, userID: Int64) async {
        let lastSeq = (try? database.maxConversationSeq(conversationID: conversationID)) ?? 0
        let unread = (try? database.fetchConversationSummaryRecords(userID: userID)
            .first(where: { $0.conversationID == conversationID })?
            .unreadCount) ?? 0
        if unread > 0 {
            try? database.clearUnread(userID: userID, conversationID: conversationID)
        }
        await sendReadAck(conversationID: conversationID, lastReadSeq: lastSeq)
    }

    private func scheduleConnect(immediate: Bool) {
        guard desiredRunning else { return }
        if connectTask != nil { return }
        connectTask = Task { [weak self] in
            guard let self else { return }
            await self.runConnectLoop(immediate: immediate)
            await self.clearConnectTask()
        }
    }

    private func clearConnectTask() {
        connectTask = nil
    }

    private func runConnectLoop(immediate: Bool) async {
        var attempt = reconnectAttempt
        var skipDelay = immediate
        while desiredRunning && !Task.isCancelled {
            if !skipDelay {
                eventContinuation.yield(.status(.reconnecting(attempt: attempt)))
                let delay = Self.reconnectDelay(attempt: attempt)
                try? await Task.sleep(nanoseconds: UInt64(delay * 1_000_000_000))
                guard desiredRunning && !Task.isCancelled else { return }
            }
            skipDelay = false

            do {
                try await connectOnce()
                reconnectAttempt = 0
                // Stay until reader exits (disconnect).
                await readerTask?.value
                if desiredRunning && shouldReconnectOnClose {
                    attempt = max(1, reconnectAttempt + 1)
                    reconnectAttempt = attempt
                    continue
                }
                return
            } catch {
                attempt += 1
                reconnectAttempt = attempt
                eventContinuation.yield(.status(.disconnected(reason: error.localizedDescription)))
                if !shouldReconnectOnClose {
                    return
                }
            }
        }
    }

    private func connectOnce() async throws {
        guard let creds = try session.load() else {
            throw HTTPClientError.status(code: 401, body: "not logged in")
        }
        myUserID = creds.userID
        eventContinuation.yield(.status(.connecting))

        tearDownConnection()
        let transport = makeTransport(gatewayURL)
        self.transport = transport

        // 先挂上唯一读循环，再 connect，避免握手响应丢帧。
        readerTask = Task { [weak self] in
            guard let self else { return }
            for await event in transport.events {
                await self.handleTransportEvent(event)
            }
        }

        try await transport.connect()

        let cursor = try database.getSyncCursor(userID: creds.userID, deviceID: creds.deviceID)
        let handshake = WsCodec.makeHandshakeRequest(
            accessToken: creds.accessToken,
            deviceID: creds.deviceID,
            lastEventSeq: UInt64(max(0, cursor))
        )
        seq += 1
        try await transport.send(
            try WsCodec.encodeFrame(
                opcode: WsOpcode.handshakeReq,
                payload: handshake,
                seqID: seq
            )
        )

        let resp: Livechat_Ws_HandshakeResponse = try await withThrowingTaskGroup(
            of: Livechat_Ws_HandshakeResponse.self
        ) { group in
            group.addTask {
                try await withCheckedThrowingContinuation { (cont: CheckedContinuation<Livechat_Ws_HandshakeResponse, Error>) in
                    Task { await self.setHandshakeWaiter(cont) }
                }
            }
            group.addTask {
                try await Task.sleep(nanoseconds: 10_000_000_000)
                throw RealtimeError.closed(reason: "handshake timeout")
            }
            let value = try await group.next()!
            group.cancelAll()
            return value
        }

        guard resp.success else {
            shouldReconnectOnClose = false
            throw RealtimeError.handshakeRejected(code: 0, message: resp.errorMessage)
        }
        heartbeatInterval = TimeInterval(max(5, resp.heartbeatIntervalS))
        eventContinuation.yield(.status(.connected(sessionID: resp.sessionID)))
        startHeartbeat(on: transport)
    }

    private func setHandshakeWaiter(_ cont: CheckedContinuation<Livechat_Ws_HandshakeResponse, Error>) {
        if let old = handshakeWaiter {
            handshakeWaiter = cont
            old.resume(throwing: RealtimeError.closed(reason: "superseded"))
        } else {
            handshakeWaiter = cont
        }
    }

    private func failHandshakeWaiter(_ error: Error) {
        if let waiter = handshakeWaiter {
            handshakeWaiter = nil
            waiter.resume(throwing: error)
        }
    }

    private func handleTransportEvent(_ event: TransportEvent) {
        switch event {
        case .connected:
            break
        case .frame(let data):
            do {
                try handleFrame(data)
            } catch {
                failHandshakeWaiter(error)
                tearDownConnection()
                eventContinuation.yield(.status(.disconnected(reason: error.localizedDescription)))
            }
        case .closed(_, let reason):
            failHandshakeWaiter(RealtimeError.closed(reason: reason ?? "closed"))
            tearDownConnection()
            eventContinuation.yield(.status(.disconnected(reason: reason ?? "closed")))
        }
    }

    private func handleFrame(_ data: Data) throws {
        let frame = try WsCodec.decodeFrame(data)
        switch frame.opcode {
        case WsOpcode.handshakeResp:
            let resp = try WsCodec.decodeHandshakeResponse(frame)
            if let waiter = handshakeWaiter {
                handshakeWaiter = nil
                waiter.resume(returning: resp)
            }
        case WsOpcode.heartbeatAck:
            break
        case WsOpcode.messageDelivery:
            let delivery = try WsCodec.decodeMessageDelivery(frame)
            pendingDeliveries.append(delivery)
            scheduleFlush()
        case WsOpcode.error:
            let err = try WsCodec.decodeError(frame)
            shouldReconnectOnClose = err.shouldReconnect
            if let waiter = handshakeWaiter {
                handshakeWaiter = nil
                waiter.resume(
                    throwing: RealtimeError.handshakeRejected(
                        code: Int(err.errorCode),
                        message: err.message
                    )
                )
            } else {
                throw RealtimeError.serverError(code: Int(err.errorCode), message: err.message)
            }
        case WsOpcode.disconnect:
            let disc = try WsCodec.decodeDisconnect(frame)
            shouldReconnectOnClose = disc.shouldReconnect
            throw RealtimeError.closed(reason: disc.reason)
        default:
            break
        }
    }

    private func scheduleFlush() {
        guard flushTask == nil else { return }
        flushTask = Task { [weak self] in
            try? await Task.sleep(nanoseconds: Self.deliveryFlushIntervalMs * 1_000_000)
            await self?.flushPendingDeliveries()
        }
    }

    private func flushPendingDeliveries() {
        flushTask = nil
        guard !pendingDeliveries.isEmpty else { return }
        let batch = pendingDeliveries
        pendingDeliveries.removeAll(keepingCapacity: true)
        do {
            let touched = try database.applyIncomingDeliveries(batch, myUserID: myUserID)
            if !touched.isEmpty {
                eventContinuation.yield(.databaseChanged(conversationIDs: touched))
            }
        } catch {
            pendingDeliveries.insert(contentsOf: batch, at: 0)
            eventContinuation.yield(.status(.disconnected(reason: error.localizedDescription)))
        }
    }

    private func startHeartbeat(on transport: any WebSocketTransport) {
        heartbeatTask?.cancel()
        let interval = heartbeatInterval
        heartbeatTask = Task { [weak self] in
            while !Task.isCancelled {
                try? await Task.sleep(nanoseconds: UInt64(interval * 1_000_000_000))
                guard !Task.isCancelled else { return }
                await self?.sendHeartbeat(on: transport)
            }
        }
    }

    private func sendHeartbeat(on transport: any WebSocketTransport) async {
        do {
            seq += 1
            try await transport.send(
                try WsCodec.encodeFrame(
                    opcode: WsOpcode.heartbeat,
                    payload: Livechat_Ws_Heartbeat(),
                    seqID: seq
                )
            )
        } catch {
            // reader / closed 路径负责重连
        }
    }

    private func tearDownConnection() {
        heartbeatTask?.cancel()
        heartbeatTask = nil
        flushTask?.cancel()
        flushTask = nil
        if !pendingDeliveries.isEmpty {
            flushPendingDeliveries()
        }
        let reader = readerTask
        readerTask = nil
        transport?.close()
        transport = nil
        // close() finishes the events stream → reader exits
        reader?.cancel()
    }

    private func startPathMonitorIfNeeded() {
        guard pathMonitor == nil else { return }
        let monitor = NWPathMonitor()
        pathMonitor = monitor
        monitor.pathUpdateHandler = { [weak self] path in
            guard let self else { return }
            Task { await self.handlePathUpdate(path.status == .satisfied) }
        }
        monitor.start(queue: DispatchQueue(label: "livechat.realtime.path"))
    }

    private func handlePathUpdate(_ satisfied: Bool) {
        guard desiredRunning else { return }
        if satisfied {
            scheduleConnect(immediate: reconnectAttempt == 0)
        } else {
            tearDownConnection()
            eventContinuation.yield(.status(.disconnected(reason: "path_unsatisfied")))
        }
    }

    /// Spec 05 / gateway reconnect.go：窗口内 jitter。
    public static func reconnectDelay(attempt: Int) -> TimeInterval {
        var minDelay: TimeInterval = 0.5
        var maxDelay: TimeInterval = 1.0
        let n = max(0, attempt)
        for _ in 0..<n {
            minDelay = min(minDelay * 2, 16)
            maxDelay = min(maxDelay * 2, 30)
        }
        if maxDelay <= minDelay { return minDelay }
        return minDelay + Double.random(in: 0...(maxDelay - minDelay))
    }
}

public enum RealtimeError: Error, Sendable, LocalizedError {
    case handshakeRejected(code: Int, message: String)
    case serverError(code: Int, message: String)
    case closed(reason: String)

    public var errorDescription: String? {
        switch self {
        case .handshakeRejected(let code, let message):
            return "handshake rejected (\(code)): \(message)"
        case .serverError(let code, let message):
            return "ws error (\(code)): \(message)"
        case .closed(let reason):
            return "ws closed: \(reason)"
        }
    }
}
