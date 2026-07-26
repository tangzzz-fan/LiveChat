import Testing
@testable import ChatApplication
import ChatInfrastructure
import Foundation

@Test
func scaffoldServicesConstruct() throws {
    let services = try AppServices.makeScaffold(
        gatewayWSURL: URL(string: "ws://127.0.0.1:8081/ws")!
    )
    #expect(services.transport is URLSessionWebSocketTransport)
}
