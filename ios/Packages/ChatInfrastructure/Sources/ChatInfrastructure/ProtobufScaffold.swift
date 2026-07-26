import Foundation
import SwiftProtobuf

/// 脚手架：证明 SwiftProtobuf 链路可编译。
/// 正式类型由 `ios/scripts/gen_proto.sh` 从 `livechat-server/proto/ws_frame.proto` 生成后入库。
public enum ProtobufScaffold {
    /// 占位：后续替换为生成的 `WsFrame` 序列化。
    public static func emptyMessageData() -> Data {
        // SwiftProtobuf 链接冒烟；真实帧编解码在功能票接入 Generated/*.pb.swift
        Data()
    }

    public static var libraryLinked: Bool {
        // Touch a SwiftProtobuf symbol so the import is not dead-stripped in checks.
        _ = Google_Protobuf_Empty()
        return true
    }
}
