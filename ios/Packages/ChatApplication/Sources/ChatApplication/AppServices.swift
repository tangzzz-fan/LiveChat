import Foundation
import ChatInfrastructure

/// 应用层组装入口（脚手架）。真实 UseCase 在功能票落地。
public struct AppServices: Sendable {
    public let database: LocalDatabase
    public let transport: any WebSocketTransport

    public init(database: LocalDatabase, transport: any WebSocketTransport) {
        self.database = database
        self.transport = transport
    }

    public static func makeScaffold(gatewayWSURL: URL) throws -> AppServices {
        let db = try LocalDatabase.inMemory()
        let transport = URLSessionWebSocketTransport(url: gatewayWSURL)
        return AppServices(database: db, transport: transport)
    }
}
