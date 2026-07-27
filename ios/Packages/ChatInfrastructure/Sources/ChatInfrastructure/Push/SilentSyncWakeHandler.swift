import Foundation

/// 后台/静默唤醒只跑增量 sync（Spec 13 §8.2）：不重连 WS、不做重 UI/大媒体。
public actor SilentSyncWakeHandler {
    public static let budgetNanoseconds: UInt64 = 25_000_000_000 // ~25s 预算上限意识

    private let syncExecutor: SyncExecutor

    public init(syncExecutor: SyncExecutor) {
        self.syncExecutor = syncExecutor
    }

    public func handleWake(reason: String) async -> Result<SyncRunResult, Error> {
        let started = ContinuousClock.now
        do {
            let result = try await syncExecutor.syncIncremental()
            let elapsed = ContinuousClock.now - started
            _ = elapsed
            _ = reason
            return .success(result)
        } catch {
            return .failure(error)
        }
    }
}

/// 模拟器无真实 APNs 时使用的稳定 mock token（仍写入服务端 devices.push_token）。
public enum PushTokenFactory {
    public static func mockToken(deviceID: String) -> String {
        "sim-mock-\(deviceID)"
    }

    public static func hexToken(from data: Data) -> String {
        data.map { String(format: "%02x", $0) }.joined()
    }
}
