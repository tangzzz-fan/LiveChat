import Testing
@testable import ChatApplication
import ChatInfrastructure
import Foundation

@Test
func scaffoldServicesConstruct() throws {
    let services = try AppServices.make(
        gatewayWSURL: URL(string: "ws://127.0.0.1:8081/ws")!
    )
    #expect(services.gatewayWSURL.host == "127.0.0.1")
    #expect(services.realtimeListenGate.beginIfNeeded())
    #expect(!services.realtimeListenGate.beginIfNeeded())
}
