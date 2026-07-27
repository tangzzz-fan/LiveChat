import Foundation

/// URLSessionWebSocketTask 实现。RealtimeSession 每次重连应新建实例。
public final class URLSessionWebSocketTransport: WebSocketTransport, @unchecked Sendable {
    private let url: URL
    private let continuation: AsyncStream<TransportEvent>.Continuation
    public let events: AsyncStream<TransportEvent>

    private let stateQueue = DispatchQueue(label: "livechat.ws.transport")
    private var task: URLSessionWebSocketTask?
    private let session: URLSession
    private var finished = false

    public init(url: URL, session: URLSession = .shared) {
        self.url = url
        self.session = session
        let pair = AsyncStream.makeStream(of: TransportEvent.self)
        self.events = pair.stream
        self.continuation = pair.continuation
    }

    public func connect() async throws {
        let alreadyFinished: Bool = stateQueue.sync { finished }
        if alreadyFinished { throw TransportError.closed }

        let ws = session.webSocketTask(with: url)
        stateQueue.sync {
            task?.cancel(with: .goingAway, reason: nil)
            task = ws
        }
        ws.resume()
        continuation.yield(.connected)
        Task { [weak self] in
            await self?.receiveLoop(ws)
        }
    }

    public func send(_ frame: Data) async throws {
        let ws: URLSessionWebSocketTask? = stateQueue.sync { task }
        guard let ws else { throw TransportError.notConnected }
        try await ws.send(.data(frame))
    }

    public func close() {
        let ws: URLSessionWebSocketTask? = stateQueue.sync {
            let current = task
            task = nil
            let shouldFinish = !finished
            finished = true
            return shouldFinish ? current : nil
        }
        // If already finished, ws is nil and we skip yield.
        guard let ws else {
            // still cancel if task was cleared on a previous finish path
            return
        }
        ws.cancel(with: .goingAway, reason: nil)
        continuation.yield(.closed(code: nil, reason: "client_close"))
        continuation.finish()
    }

    private func receiveLoop(_ ws: URLSessionWebSocketTask) async {
        while true {
            let current: URLSessionWebSocketTask? = stateQueue.sync { task }
            guard current === ws else { return }
            do {
                let message = try await ws.receive()
                switch message {
                case .data(let data):
                    continuation.yield(.frame(data))
                case .string(let text):
                    continuation.yield(.frame(Data(text.utf8)))
                @unknown default:
                    break
                }
            } catch {
                let shouldFinish: Bool = stateQueue.sync {
                    guard !finished else { return false }
                    finished = true
                    task = nil
                    return true
                }
                if shouldFinish {
                    continuation.yield(.closed(code: nil, reason: error.localizedDescription))
                    continuation.finish()
                }
                return
            }
        }
    }
}
