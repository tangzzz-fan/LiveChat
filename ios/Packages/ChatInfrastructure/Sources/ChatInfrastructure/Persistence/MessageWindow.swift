import Foundation

/// 消息窗分页常量（高负载 #2：Store 不灌全表）。
public enum MessageWindow {
    public static let defaultPageSize = 50
}

public struct MessageWindowPage: Sendable {
    public let records: [MessageRecord]
    /// 是否还有比本页更早的、已分配 `conversation_seq` 的消息。
    public let hasMoreOlder: Bool
    /// 本页中带 seq 的最小值；仅 pending（无 seq）时为 nil。
    public let oldestSeq: Int64?

    public init(records: [MessageRecord], hasMoreOlder: Bool, oldestSeq: Int64?) {
        self.records = records
        self.hasMoreOlder = hasMoreOlder
        self.oldestSeq = oldestSeq
    }
}

public enum MessageWindowLoadMode: Sendable, Equatable {
    /// 打开会话：最新一页（按 conversation_seq 尾部）。
    case latestPage
    /// 已向上翻页后刷新：从锚点 seq 起到最新，并附带 pending。
    case fromSeq(Int64)
}
