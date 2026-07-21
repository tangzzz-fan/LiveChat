import Foundation
import ChatDomain

/// Stub repository implementations — compile-time validation of protocol signatures.
/// Real implementations will replace these in subsequent phases.

public final class StubMessageRepository: MessageRepository {
    public init() {}

    public func getMessages(in conversationID: String, limit: Int) async throws -> [Message] {
        fatalError("Not implemented — pending Phase 4")
    }

    public func sendMessage(_ request: SendMessageRequest) async throws -> SendMessageResponse {
        fatalError("Not implemented — pending Phase 4")
    }

    public func insertMessage(_ message: Message) async throws {
        fatalError("Not implemented — pending Phase 4")
    }

    public func updateMessageStatus(clientMessageID: String, status: MessageStatus) async throws {
        fatalError("Not implemented — pending Phase 4")
    }
}

public final class StubConversationRepository: ConversationRepository {
    public init() {}

    public func getConversations() async throws -> [ConversationSummary] {
        fatalError("Not implemented — pending Phase 4")
    }

    public func getConversation(id: String) async throws -> ConversationSummary? {
        fatalError("Not implemented — pending Phase 4")
    }

    public func upsertConversation(_ conversation: ConversationSummary) async throws {
        fatalError("Not implemented — pending Phase 4")
    }
}

public final class StubSyncRepository: SyncRepository {
    public init() {}

    public func getSyncCursor() async throws -> Int64 {
        fatalError("Not implemented — pending Phase 4")
    }

    public func updateSyncCursor(_ seq: Int64) async throws {
        fatalError("Not implemented — pending Phase 4")
    }

    public func fetchEvents(from cursor: Int64) async throws -> SyncResponse {
        fatalError("Not implemented — pending Phase 4")
    }
}

public final class StubPushRepository: PushRepository {
    public init() {}

    public func registerPushToken(_ token: Data) async throws {
        fatalError("Not implemented — pending Phase 4")
    }

    public func handleRemoteNotification(_ userInfo: [AnyHashable: Any]) async {
        fatalError("Not implemented — pending Phase 4")
    }
}

public final class StubWebSocketRepository: WebSocketRepository {
    public init() {}

    public func connect() async throws {
        fatalError("Not implemented — pending Phase 4")
    }

    public func disconnect() async {
        fatalError("Not implemented — pending Phase 4")
    }

    public func sendFrame(_ frame: WebSocketFrame) async throws {
        fatalError("Not implemented — pending Phase 4")
    }

    public var messageStream: AsyncStream<WebSocketFrame> {
        fatalError("Not implemented — pending Phase 4")
    }
}

public final class StubAuthRepository: AuthRepository {
    public init() {}

    public func requestCode(phone: String) async throws -> CodeRequestResponse {
        fatalError("Not implemented — pending Phase 4")
    }

    public func verifyCode(phone: String, code: String, deviceID: String, platform: String) async throws -> AuthTokens {
        fatalError("Not implemented — pending Phase 4")
    }

    public func refreshToken(_ token: String) async throws -> AuthTokens {
        fatalError("Not implemented — pending Phase 4")
    }

    public func logout() async throws {
        fatalError("Not implemented — pending Phase 4")
    }
}

public final class StubMediaRepository: MediaRepository {
    public init() {}

    public func uploadImage(_ data: Data, metadata: ImageMetadata) async throws -> Attachment {
        fatalError("Not implemented — pending Phase 4")
    }

    public func downloadImage(objectKey: String) async throws -> Data {
        fatalError("Not implemented — pending Phase 4")
    }
}
