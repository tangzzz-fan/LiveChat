import Foundation

public enum SilentWakeOutcome: Sendable {
    case success(SyncRunResult)
    /// 预算内未跑完：已取消未完成分页；cursor 可能已部分推进。
    case timedOut
    case failure(Error)
}

/// 后台/静默唤醒只跑增量 sync（Spec 13 §8.2）：不重连 WS、不做重 UI/大媒体；≤25s 预算取消。
public actor SilentSyncWakeHandler {
    public static let budgetNanoseconds: UInt64 = 25_000_000_000

    private let syncExecutor: SyncExecutor
    private let budgetNanoseconds: UInt64

    public init(
        syncExecutor: SyncExecutor,
        budgetNanoseconds: UInt64 = SilentSyncWakeHandler.budgetNanoseconds
    ) {
        self.syncExecutor = syncExecutor
        self.budgetNanoseconds = budgetNanoseconds
    }

    public func handleWake(reason: String) async -> SilentWakeOutcome {
        _ = reason
        let syncTask = Task {
            try await syncExecutor.syncIncremental()
        }
        let budgetTask = Task {
            try await Task.sleep(nanoseconds: budgetNanoseconds)
            syncTask.cancel()
        }
        defer { budgetTask.cancel() }

        do {
            let result = try await syncTask.value
            return .success(result)
        } catch is CancellationError {
            return .timedOut
        } catch {
            // Task 取消时常被包装；若已 cancel 视为超时。
            if syncTask.isCancelled {
                return .timedOut
            }
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
