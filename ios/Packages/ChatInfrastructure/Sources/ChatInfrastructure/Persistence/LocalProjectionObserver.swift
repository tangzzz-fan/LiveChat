import Foundation
import GRDB

/// GRDB `ValueObservation` + 去抖：WS/sync 只写库，UI 经此投影进 Store（高负载 #1）。
public final class LocalProjectionObserver: @unchecked Sendable {
    /// 默认约一帧（16ms）；可配置到 33ms。
    public static let defaultDebounceNanoseconds: UInt64 = 16_000_000

    private let database: LocalDatabase
    private let debounceNanoseconds: UInt64
    private let syncQueue = DispatchQueue(label: "livechat.projection.observer")
    private var conversationCancellable: AnyDatabaseCancellable?
    private var messageCancellable: AnyDatabaseCancellable?
    private var conversationDebounceTask: Task<Void, Never>?
    private var messageDebounceTask: Task<Void, Never>?

    public init(
        database: LocalDatabase,
        debounceNanoseconds: UInt64 = LocalProjectionObserver.defaultDebounceNanoseconds
    ) {
        self.database = database
        self.debounceNanoseconds = debounceNanoseconds
    }

    public func stopAll() {
        syncQueue.sync {
            conversationCancellable?.cancel()
            conversationCancellable = nil
            messageCancellable?.cancel()
            messageCancellable = nil
            conversationDebounceTask?.cancel()
            conversationDebounceTask = nil
            messageDebounceTask?.cancel()
            messageDebounceTask = nil
        }
    }

    public func stopMessages() {
        syncQueue.sync {
            messageCancellable?.cancel()
            messageCancellable = nil
            messageDebounceTask?.cancel()
            messageDebounceTask = nil
        }
    }

    public func observeConversations(
        userID: Int64,
        onChange: @escaping @Sendable ([ConversationSummaryRecord]) -> Void
    ) {
        let observation = ValueObservation.tracking { db in
            try LocalDatabase.conversationSummaries(db: db, userID: userID)
        }
        syncQueue.sync {
            conversationCancellable?.cancel()
            conversationCancellable = observation.start(
                in: database.dbQueue,
                scheduling: .async(onQueue: DispatchQueue.global(qos: .userInitiated))
            ) { _ in
            } onChange: { [weak self] rows in
                self?.scheduleConversationDebounce { onChange(rows) }
            }
        }
    }

    public func observeMessageWindow(
        conversationID: String,
        mode: MessageWindowLoadMode,
        pageSize: Int = MessageWindow.defaultPageSize,
        onChange: @escaping @Sendable (MessageWindowPage) -> Void
    ) {
        let observation = ValueObservation.tracking { db in
            try LocalDatabase.messageWindow(
                db: db,
                conversationID: conversationID,
                mode: mode,
                pageSize: pageSize
            )
        }
        syncQueue.sync {
            messageCancellable?.cancel()
            messageCancellable = observation.start(
                in: database.dbQueue,
                scheduling: .async(onQueue: DispatchQueue.global(qos: .userInitiated))
            ) { _ in
            } onChange: { [weak self] page in
                self?.scheduleMessageDebounce { onChange(page) }
            }
        }
    }

    private func scheduleConversationDebounce(_ body: @escaping @Sendable () -> Void) {
        syncQueue.async { [weak self] in
            guard let self else { return }
            self.conversationDebounceTask?.cancel()
            let ns = self.debounceNanoseconds
            self.conversationDebounceTask = Task {
                try? await Task.sleep(nanoseconds: ns)
                guard !Task.isCancelled else { return }
                body()
            }
        }
    }

    private func scheduleMessageDebounce(_ body: @escaping @Sendable () -> Void) {
        syncQueue.async { [weak self] in
            guard let self else { return }
            self.messageDebounceTask?.cancel()
            let ns = self.debounceNanoseconds
            self.messageDebounceTask = Task {
                try? await Task.sleep(nanoseconds: ns)
                guard !Task.isCancelled else { return }
                body()
            }
        }
    }
}
