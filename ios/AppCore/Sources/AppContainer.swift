import Foundation
import ChatDomain
import ChatInfrastructure

/// 基础 DI Container — 简单的服务注册与解析
public final class AppContainer {
    public static let shared = AppContainer()

    private var services: [String: Any] = [:]

    public func register<T>(_ type: T.Type, factory: @escaping () -> T) {
        let key = String(describing: type)
        services[key] = factory
    }

    public func resolve<T>(_ type: T.Type) -> T? {
        let key = String(describing: type)
        if let factory = services[key] as? () -> T {
            return factory()
        }
        return nil
    }

    public func initialize() {
        // Register stub repositories
        register(MessageRepository.self) { StubMessageRepository() }
        register(ConversationRepository.self) { StubConversationRepository() }
        register(SyncRepository.self) { StubSyncRepository() }
        register(PushRepository.self) { StubPushRepository() }
        register(WebSocketRepository.self) { StubWebSocketRepository() }
        register(AuthRepository.self) { StubAuthRepository() }
        register(MediaRepository.self) { StubMediaRepository() }

        print("[AppContainer] initialized — \(services.count) services registered")
    }
}
