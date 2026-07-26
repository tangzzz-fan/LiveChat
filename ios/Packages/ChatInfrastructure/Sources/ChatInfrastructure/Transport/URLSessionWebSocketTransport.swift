import Foundation

/// 脚手架空壳：证明类型可编译；真实握手/读写循环在后续功能票实现。
public final class URLSessionWebSocketTransport: WebSocketTransport, @unchecked Sendable {
    private let url: URL
    private let continuation: AsyncStream<TransportEvent>.Continuation
    public let events: AsyncStream<TransportEvent>

    private var task: URLSessionWebSocketTask?
    private let session: URLSession

    public init(url: URL, session: URLSession = .shared) {
        self.url = url
        self.session = session
        let pair = AsyncStream.makeStream(of: TransportEvent.self)
        self.events = pair.stream
        self.continuation = pair.continuation
    }

    public func connect() async throws {
        let ws = session.webSocketTask(with: url)
        task = ws
        ws.resume()
        continuation.yield(.connected)
        // TODO(0037+): arm receive loop + app-level heartbeat
    }

    public func send(_ frame: Data) async throws {
        guard let task else {
            throw TransportError.notConnected
        }
        try await task.send(.data(frame))
    }

    public func close() {
        task?.cancel(with: .goingAway, reason: nil)
        task = nil
        continuation.yield(.closed(code: nil, reason: "client_close"))
        continuation.finish()
    }
}

public enum TransportError: Error, Sendable {
    case notConnected
}
