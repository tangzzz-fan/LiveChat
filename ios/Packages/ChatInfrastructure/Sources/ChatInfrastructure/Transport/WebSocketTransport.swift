import Foundation

/// 传输层事件（Spec 13 `WebSocketTransport`）
public enum TransportEvent: Sendable {
    case connected
    case frame(Data)
    case closed(code: Int?, reason: String?)
}

/// 长连接传输抽象：默认 URLSession；可替换，上层不改。
public protocol WebSocketTransport: Sendable {
    var events: AsyncStream<TransportEvent> { get }
    func connect() async throws
    func send(_ frame: Data) async throws
    func close()
}

public enum TransportError: Error, Sendable {
    case notConnected
    case closed
}
