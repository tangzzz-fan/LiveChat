import Foundation
import SwiftProtobuf

/// 冒烟：确认 Generated/ws_frame.pb.swift 与 SwiftProtobuf 可链接。
public enum ProtobufScaffold {
    public static func emptyMessageData() -> Data {
        (try? Livechat_Ws_Heartbeat().serializedData()) ?? Data()
    }

    public static var libraryLinked: Bool {
        _ = Google_Protobuf_Empty()
        _ = Livechat_Ws_WsFrame()
        return true
    }
}
