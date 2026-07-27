import Foundation
import Network

/// 路径恢复后续跑发送队列（高负载 #5 / Spec 13 §8.3）。
/// 与 RealtimeSession 的 path monitor 独立：即使 WS 已停（后台）仍可续跑 HTTP 发送。
public final class SendPathResumeMonitor: @unchecked Sendable {
    private let sendExecutor: MessageSendExecutor
    private let queue = DispatchQueue(label: "livechat.send.path")
    private let lock = NSLock()
    private var monitor: NWPathMonitor?
    private var lastSatisfied = true

    public init(sendExecutor: MessageSendExecutor) {
        self.sendExecutor = sendExecutor
    }

    public func start() {
        lock.lock()
        defer { lock.unlock() }
        guard monitor == nil else { return }
        let monitor = NWPathMonitor()
        self.monitor = monitor
        monitor.pathUpdateHandler = { [weak self] path in
            guard let self else { return }
            let satisfied = path.status == .satisfied
            self.lock.lock()
            let wasSatisfied = self.lastSatisfied
            self.lastSatisfied = satisfied
            self.lock.unlock()
            if satisfied, !wasSatisfied {
                Task {
                    await self.sendExecutor.reclaimStaleSendingAndProcess()
                }
            }
        }
        monitor.start(queue: queue)
    }

    public func stop() {
        lock.lock()
        defer { lock.unlock() }
        monitor?.cancel()
        monitor = nil
        lastSatisfied = true
    }
}
