import Foundation
import SwiftProtobuf

public enum WsOpcode {
    public static let handshakeReq: UInt32 = 0x0001
    public static let handshakeResp: UInt32 = 0x0002
    public static let heartbeat: UInt32 = 0x0003
    public static let heartbeatAck: UInt32 = 0x0004
    public static let ack: UInt32 = 0x0005
    public static let error: UInt32 = 0x0006
    public static let disconnect: UInt32 = 0x0007
    public static let messageDelivery: UInt32 = 0x1001
    public static let messageStatus: UInt32 = 0x1002
    public static let syncEvent: UInt32 = 0x2001
    public static let conversationUpdate: UInt32 = 0x2002
}

public enum WsProtocol {
    public static let version: UInt32 = 0x01
}

public enum WsCodec {
    public static func encodeFrame(
        opcode: UInt32,
        payload: (any SwiftProtobuf.Message)? = nil,
        seqID: UInt64 = 0
    ) throws -> Data {
        var frame = Livechat_Ws_WsFrame()
        frame.version = WsProtocol.version
        frame.opcode = opcode
        frame.seqID = seqID
        frame.timestampMs = Int64(Date().timeIntervalSince1970 * 1000)
        if let payload {
            frame.payload = try payload.serializedData()
        }
        return try frame.serializedData()
    }

    public static func decodeFrame(_ data: Data) throws -> Livechat_Ws_WsFrame {
        try Livechat_Ws_WsFrame(serializedBytes: data)
    }

    public static func decodeHandshakeResponse(_ frame: Livechat_Ws_WsFrame) throws -> Livechat_Ws_HandshakeResponse {
        try Livechat_Ws_HandshakeResponse(serializedBytes: frame.payload)
    }

    public static func decodeMessageDelivery(_ frame: Livechat_Ws_WsFrame) throws -> Livechat_Ws_WsMessageDelivery {
        try Livechat_Ws_WsMessageDelivery(serializedBytes: frame.payload)
    }

    public static func decodeError(_ frame: Livechat_Ws_WsFrame) throws -> Livechat_Ws_ErrorFrame {
        try Livechat_Ws_ErrorFrame(serializedBytes: frame.payload)
    }

    public static func decodeDisconnect(_ frame: Livechat_Ws_WsFrame) throws -> Livechat_Ws_DisconnectFrame {
        try Livechat_Ws_DisconnectFrame(serializedBytes: frame.payload)
    }

    public static func makeHandshakeRequest(
        accessToken: String,
        deviceID: String,
        lastEventSeq: UInt64,
        platform: String = "ios",
        appVersion: String = "0.4.0"
    ) -> Livechat_Ws_HandshakeRequest {
        var req = Livechat_Ws_HandshakeRequest()
        req.accessToken = accessToken
        req.deviceID = deviceID
        req.platform = platform
        req.appVersion = appVersion
        req.protocolVer = WsProtocol.version
        req.lastEventSeq = lastEventSeq
        return req
    }
}
