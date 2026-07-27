import Foundation
import TGReduxKit
import ChatApplication
import ChatInfrastructure
import ChatDomain

@MainActor
func makeChatMiddleware(services: AppServices) -> Middleware<AppState, AppAction> {
    { store, action, next in
        next(action)
        guard case .chat(let chatAction) = action else { return }

        switch chatAction {
        case .refreshConversationsTapped:
            store.runTask(id: CancellationID("chat.refreshConversations")) {
                await store.dispatch(.chat(.busy(true)))
                do {
                    guard let session = try services.auth.restoredSession() else {
                        await store.dispatch(.chat(.busy(false)))
                        return
                    }
                    let remote = try await services.conversations.listRemoteSummaries()
                    for summary in remote {
                        try services.database.upsertConversationSummary(summary)
                    }
                    let local = try services.database.fetchConversationSummaries(userID: session.userID)
                    let rows = local.map {
                        ChatState.ConversationRow(
                            conversationID: $0.conversationID,
                            title: $0.title ?? $0.conversationID,
                            preview: $0.lastMessagePreview ?? ""
                        )
                    }
                    await store.dispatch(.chat(.conversationsLoaded(rows)))
                } catch {
                    await store.dispatch(.chat(.failed(error.localizedDescription)))
                }
            }
        case .openDirectTapped:
            let raw = store.state.chat.peerUserIDInput.trimmingCharacters(in: .whitespacesAndNewlines)
            store.runTask(id: CancellationID("chat.openDirect")) {
                await store.dispatch(.chat(.busy(true)))
                do {
                    guard let peer = Int64(raw) else {
                        await store.dispatch(.chat(.failed("peer_user_id 必须是数字")))
                        return
                    }
                    guard let session = try services.auth.restoredSession() else {
                        await store.dispatch(.chat(.failed("未登录")))
                        return
                    }
                    let result = try await services.conversations.ensureDirect(peerUserID: peer)
                    try services.database.upsertConversationSummary(
                        ConversationSummary(
                            userID: session.userID,
                            conversationID: result.conversationID,
                            type: result.type,
                            title: "user \(result.peerUserID)",
                            lastMessagePreview: nil,
                            lastMessageAt: nil,
                            unreadCount: 0
                        )
                    )
                    let messages = try loadVisibleMessages(
                        database: services.database,
                        conversationID: result.conversationID,
                        myUserID: session.userID
                    )
                    await store.dispatch(
                        .chat(.conversationOpened(id: result.conversationID, messages: messages))
                    )
                    await store.dispatch(.chat(.refreshConversationsTapped))
                } catch {
                    await store.dispatch(.chat(.failed(error.localizedDescription)))
                }
            }
        case .selectConversation(let id):
            store.runTask(id: CancellationID("chat.selectConversation")) {
                do {
                    guard let session = try services.auth.restoredSession() else { return }
                    let messages = try loadVisibleMessages(
                        database: services.database,
                        conversationID: id,
                        myUserID: session.userID
                    )
                    await store.dispatch(
                        .chat(.conversationOpened(id: id, messages: messages))
                    )
                } catch {
                    await store.dispatch(.chat(.failed(error.localizedDescription)))
                }
            }
        case .sendTapped:
            let draft = store.state.chat.composeDraft.trimmingCharacters(in: .whitespacesAndNewlines)
            let conversationID = store.state.chat.activeConversationID
            let conversationTitle = store.state.chat.conversationRows
                .first(where: { $0.conversationID == conversationID })?.title
            store.runTask(id: CancellationID("chat.send")) {
                guard let conversationID, !draft.isEmpty else { return }
                guard let session = try? services.auth.restoredSession() else {
                    await store.dispatch(.chat(.failed("未登录")))
                    return
                }
                await store.dispatch(.chat(.busy(true)))
                do {
                    let clientID = "ios-\(session.deviceID)-\(UUID().uuidString.lowercased())"
                    let payload = #"{"text":\#(jsonString(draft))}"#
                    let message = Message(
                        clientMessageID: clientID,
                        conversationID: conversationID,
                        senderUserID: session.userID,
                        messageType: "text",
                        content: payload,
                        status: .queued
                    )
                    try await services.sendExecutor.enqueueLocalThenSend(message)
                    await store.dispatch(.chat(.updateDraft("")))
                    let messages = try loadVisibleMessages(
                        database: services.database,
                        conversationID: conversationID,
                        myUserID: session.userID
                    )
                    await store.dispatch(.chat(.visibleMessagesUpdated(messages)))
                    try services.database.upsertConversationSummary(
                        ConversationSummary(
                            userID: session.userID,
                            conversationID: conversationID,
                            type: "direct",
                            title: conversationTitle,
                            lastMessagePreview: draft,
                            lastMessageAt: Date(),
                            unreadCount: 0
                        )
                    )
                    await store.dispatch(.chat(.refreshConversationsTapped))
                } catch SendQueueError.full {
                    await store.dispatch(.chat(.failed("发送队列已满")))
                } catch {
                    await store.dispatch(.chat(.failed(error.localizedDescription)))
                }
            }
        case .retryQueuedTapped:
            let conversationID = store.state.chat.activeConversationID
            store.runTask(id: CancellationID("chat.retryQueued")) {
                await services.sendExecutor.processPending()
                guard
                    let conversationID,
                    let session = try? services.auth.restoredSession()
                else { return }
                if let messages = try? loadVisibleMessages(
                    database: services.database,
                    conversationID: conversationID,
                    myUserID: session.userID
                ) {
                    await store.dispatch(.chat(.visibleMessagesUpdated(messages)))
                }
            }
        case .syncTapped, .sceneBecameActive:
            guard store.state.isLoggedIn else { return }
            let activeConversationID = store.state.chat.activeConversationID
            store.runTask(id: CancellationID("chat.sync")) {
                await store.dispatch(.chat(.syncStarted))
                do {
                    let result = try await services.syncExecutor.syncIncremental()
                    await store.dispatch(
                        .chat(.syncFinished(applied: result.appliedCount, cursor: result.cursor))
                    )
                    await store.dispatch(.chat(.refreshConversationsTapped))
                    if let activeConversationID,
                       let session = try? services.auth.restoredSession(),
                       let messages = try? loadVisibleMessages(
                           database: services.database,
                           conversationID: activeConversationID,
                           myUserID: session.userID
                       ) {
                        await store.dispatch(.chat(.visibleMessagesUpdated(messages)))
                    }
                } catch {
                    await store.dispatch(.chat(.failed(error.localizedDescription)))
                }
            }
        default:
            break
        }
    }
}

private func loadVisibleMessages(
    database: LocalDatabase,
    conversationID: String,
    myUserID: Int64
) throws -> [ChatState.MessageRow] {
    let records = try database.fetchMessages(conversationID: conversationID, limit: 200)
    return records.map { record in
        let text = extractText(from: record.content)
        return ChatState.MessageRow(
            clientMessageID: record.clientMessageID,
            serverMessageID: record.serverMessageID,
            text: text,
            status: record.status,
            isMine: record.senderUserID == myUserID
        )
    }
}

private func extractText(from content: String?) -> String {
    guard let content, let data = content.data(using: .utf8) else {
        return content ?? ""
    }
    struct Payload: Decodable { let text: String? }
    if let payload = try? JSONDecoder().decode(Payload.self, from: data), let text = payload.text {
        return text
    }
    return content
}

private func jsonString(_ value: String) -> String {
    let data = try? JSONEncoder().encode(value)
    return data.flatMap { String(data: $0, encoding: .utf8) } ?? "\"\""
}
