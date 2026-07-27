import Foundation
import TGReduxKit
import ChatApplication
import ChatInfrastructure
import ChatDomain
#if canImport(UIKit)
import UIKit
#elseif canImport(AppKit)
import AppKit
#endif

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
                    // 列表投影由 ValueObservation 驱动；这里只确保远程已写入 GRDB。
                    await store.dispatch(.chat(.busy(false)))
                    await MainActor.run {
                        ensureConversationObservation(store: store, services: services, userID: session.userID)
                    }
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
                    try await openConversation(
                        store: store,
                        services: services,
                        conversationID: result.conversationID,
                        myUserID: session.userID
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
                    try await openConversation(
                        store: store,
                        services: services,
                        conversationID: id,
                        myUserID: session.userID
                    )
                } catch {
                    await store.dispatch(.chat(.failed(error.localizedDescription)))
                }
            }
        case .leaveConversation:
            services.projections.stopMessages()
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
                    // 纯文本 → content 信封 {"text":"..."}；不把草稿直接当 content 上行。
                    let payload = try TextMessageContent.encode(draft)
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
                    await store.dispatch(.chat(.busy(false)))
                    // 消息窗 / 列表由 ValueObservation 刷新，不在此手刷。
                } catch SendQueueError.full {
                    await store.dispatch(.chat(.failed("发送队列已满")))
                } catch {
                    await store.dispatch(.chat(.failed(error.localizedDescription)))
                }
            }
        case .sendImageTapped(let data, let width, let height, let mimeType, let fileName):
            let conversationID = store.state.chat.activeConversationID
            let conversationTitle = store.state.chat.conversationRows
                .first(where: { $0.conversationID == conversationID })?.title
            store.runTask(id: CancellationID("chat.sendImage")) {
                guard let conversationID else { return }
                guard let session = try? services.auth.restoredSession() else {
                    await store.dispatch(.chat(.failed("未登录")))
                    return
                }
                await store.dispatch(.chat(.busy(true)))
                do {
                    let metadata = ImageMetadata(
                        mimeType: mimeType,
                        sizeBytes: Int64(data.count),
                        fileName: fileName,
                        width: width,
                        height: height
                    )
                    let attachment = try await services.media.uploadImage(
                        data,
                        metadata: metadata,
                        conversationID: conversationID
                    )
                    let clientID = "ios-\(session.deviceID)-\(UUID().uuidString.lowercased())"
                    let content = try ImageMessageContent.encodeAttachment(attachment)
                    let message = Message(
                        clientMessageID: clientID,
                        conversationID: conversationID,
                        senderUserID: session.userID,
                        messageType: "image",
                        content: content,
                        status: .queued
                    )
                    try await services.sendExecutor.enqueueLocalThenSend(message)
                    try services.database.upsertConversationSummary(
                        ConversationSummary(
                            userID: session.userID,
                            conversationID: conversationID,
                            type: "direct",
                            title: conversationTitle,
                            lastMessagePreview: "[图片]",
                            lastMessageAt: Date(),
                            unreadCount: 0
                        )
                    )
                    await store.dispatch(.chat(.busy(false)))
                } catch SendQueueError.full {
                    await store.dispatch(.chat(.failed("发送队列已满")))
                } catch {
                    await store.dispatch(.chat(.failed(error.localizedDescription)))
                }
            }
        case .loadOlderMessagesTapped:
            let conversationID = store.state.chat.activeConversationID
            let beforeSeq = store.state.chat.oldestLoadedSeq
            store.runTask(id: CancellationID("chat.loadOlder")) {
                guard let conversationID, let beforeSeq else { return }
                await store.dispatch(.chat(.busy(true)))
                do {
                    guard let session = try services.auth.restoredSession() else { return }
                    let page = try services.database.fetchOlderMessages(
                        conversationID: conversationID,
                        beforeSeq: beforeSeq
                    )
                    guard let newOldest = page.oldestSeq else {
                        await store.dispatch(.chat(.busy(false)))
                        return
                    }
                    let window = try loadVisibleMessageWindow(
                        database: services.database,
                        conversationID: conversationID,
                        myUserID: session.userID,
                        mode: .fromSeq(newOldest)
                    )
                    await store.dispatch(.chat(.visibleMessagesUpdated(
                        window.messages,
                        oldestLoadedSeq: window.oldestLoadedSeq,
                        hasMoreOlder: window.hasMoreOlder
                    )))
                    await MainActor.run {
                        bindMessageObservation(
                            store: store,
                            services: services,
                            conversationID: conversationID,
                            myUserID: session.userID,
                            mode: .fromSeq(newOldest)
                        )
                    }
                } catch {
                    await store.dispatch(.chat(.failed(error.localizedDescription)))
                }
            }
        case .retryQueuedTapped:
            store.runTask(id: CancellationID("chat.retryQueued")) {
                await services.sendExecutor.reclaimStaleSendingAndProcess()
            }
        case .copyMessageTapped(let text):
            copyToPasteboard(text)
        case .deleteLocalMessageTapped(let clientMessageID):
            store.runTask(id: CancellationID("chat.deleteLocal.\(clientMessageID)")) {
                do {
                    try services.database.deleteLocalMessage(clientMessageID: clientMessageID)
                } catch {
                    await store.dispatch(.chat(.failed(error.localizedDescription)))
                }
            }
        case .retryMessageTapped(let clientMessageID):
            store.runTask(id: CancellationID("chat.retryOne.\(clientMessageID)")) {
                do {
                    try services.database.updateMessageStatus(
                        clientMessageID: clientMessageID,
                        status: .queued
                    )
                    await services.sendExecutor.processPending()
                } catch {
                    await store.dispatch(.chat(.failed(error.localizedDescription)))
                }
            }
        case .cancelSendTapped(let clientMessageID):
            store.runTask(id: CancellationID("chat.cancelSend.\(clientMessageID)")) {
                await services.sendExecutor.cancelSend(clientMessageID: clientMessageID)
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
                    if let activeConversationID {
                        _ = try? await services.gapBackfill.backfillIfNeeded(
                            conversationID: activeConversationID
                        )
                    }
                    // 投影由 observation 吸收 DB 写入。
                } catch {
                    await store.dispatch(.chat(.failed(error.localizedDescription)))
                }
            }
            store.dispatch(.chat(.realtimeEnsureStarted))
            services.pathResume.start()
        case .sceneBecameBackground:
            store.dispatch(.chat(.realtimeStop))
        case .realtimeEnsureStarted:
            guard store.state.isLoggedIn else { return }
            if let session = try? services.auth.restoredSession() {
                ensureConversationObservation(store: store, services: services, userID: session.userID)
            }
            if services.realtimeListenGate.beginIfNeeded() {
                // 无 CancellationID：避免同 id 取消导致 AsyncStream 丢订阅。
                // databaseChanged 不再手刷 Store——ValueObservation 是主路径。
                store.runTask {
                    for await event in await services.realtime.events {
                        switch event {
                        case .status(let status):
                            await store.dispatch(.chat(.setConnectionBanner(banner(for: status))))
                        case .databaseChanged:
                            break
                        }
                    }
                }
            }
            store.runTask(id: CancellationID("chat.realtime.start")) {
                await services.realtime.start()
            }
            services.pathResume.start()
        case .realtimeStop:
            store.runTask(id: CancellationID("chat.realtime.stop")) {
                await services.realtime.stop(reason: "background")
                await store.dispatch(.chat(.setConnectionBanner("WS 已断开（后台）")))
            }
        case .registerPushTokenTapped:
            guard store.state.isLoggedIn else { return }
            store.runTask(id: CancellationID("chat.push.register")) {
                do {
                    let deviceID = try services.auth.currentDeviceID()
                    let token = PushTokenFactory.mockToken(deviceID: deviceID)
                    try await services.pushTokenAPI.register(pushToken: token)
                    await store.dispatch(
                        .chat(.setPushTokenBanner("push_token 已注册 · \(String(token.prefix(24)))…"))
                    )
                } catch {
                    await store.dispatch(.chat(.failed(error.localizedDescription)))
                }
            }
        case .pushTokenReceived(let token):
            store.runTask(id: CancellationID("chat.push.apns")) {
                do {
                    try await services.pushTokenAPI.register(pushToken: token)
                    await store.dispatch(
                        .chat(.setPushTokenBanner("APNs token 已上报 · \(String(token.prefix(16)))…"))
                    )
                } catch {
                    await store.dispatch(.chat(.failed(error.localizedDescription)))
                }
            }
        case .silentPushWakeTapped:
            store.dispatch(.chat(.silentPushWake(reason: "ui_inject")))
        case .silentPushWake(let reason):
            // Spec 13 §8.2：唤醒只跑增量 sync，不启动 WS、不做大媒体；≤25s 预算。
            store.runTask(id: CancellationID("chat.push.silentWake")) {
                await store.dispatch(.chat(.syncStarted))
                let outcome = await services.silentWake.handleWake(reason: reason)
                switch outcome {
                case .success(let result):
                    await store.dispatch(
                        .chat(.syncFinished(applied: result.appliedCount, cursor: result.cursor))
                    )
                case .timedOut:
                    await store.dispatch(.chat(.syncFinished(applied: 0, cursor: -1)))
                    await store.dispatch(.chat(.setConnectionBanner("静默同步超时（预算）")))
                case .failure(let error):
                    await store.dispatch(.chat(.failed(error.localizedDescription)))
                }
            }
        default:
            break
        }
    }
}

private func banner(for status: RealtimeStatus) -> String {
    switch status {
    case .idle:
        return "WS idle"
    case .connecting:
        return "WS 连接中…"
    case .connected(let sessionID):
        return "WS 已连接 · \(sessionID)"
    case .reconnecting(let attempt):
        return "WS 重连中 #\(attempt)"
    case .disconnected(let reason):
        return "WS 断开 · \(reason)"
    }
}

@MainActor
private func openConversation(
    store: Store<AppState, AppAction>,
    services: AppServices,
    conversationID: String,
    myUserID: Int64
) async throws {
    let window = try loadVisibleMessageWindow(
        database: services.database,
        conversationID: conversationID,
        myUserID: myUserID,
        mode: .latestPage
    )
    await store.dispatch(
        .chat(.conversationOpened(
            id: conversationID,
            messages: window.messages,
            oldestLoadedSeq: window.oldestLoadedSeq,
            hasMoreOlder: window.hasMoreOlder
        ))
    )
    bindMessageObservation(
        store: store,
        services: services,
        conversationID: conversationID,
        myUserID: myUserID,
        mode: .latestPage
    )
    // 0054：进会话清未读 + WS ACK(read)
    await services.realtime.markConversationRead(conversationID: conversationID, userID: myUserID)
    _ = try? await services.gapBackfill.backfillIfNeeded(conversationID: conversationID)
}

@MainActor
private func ensureConversationObservation(
    store: Store<AppState, AppAction>,
    services: AppServices,
    userID: Int64
) {
    services.projections.observeConversations(userID: userID) { records in
        let rows = records.map {
            ChatState.ConversationRow(
                conversationID: $0.conversationID,
                title: $0.title ?? $0.conversationID,
                preview: $0.lastMessagePreview ?? "",
                unreadCount: $0.unreadCount,
                lastMessageAt: $0.lastMessageAt.map {
                    Date(timeIntervalSince1970: Double($0) / 1000)
                }
            )
        }
        Task { @MainActor in
            store.dispatch(.chat(.conversationsLoaded(rows)))
        }
    }
}

@MainActor
private func bindMessageObservation(
    store: Store<AppState, AppAction>,
    services: AppServices,
    conversationID: String,
    myUserID: Int64,
    mode: MessageWindowLoadMode
) {
    services.projections.observeMessageWindow(
        conversationID: conversationID,
        mode: mode
    ) { page in
        let rows = mapMessageRows(records: page.records, myUserID: myUserID)
        let oldest = page.oldestSeq
        let hasMore = page.hasMoreOlder
        Task { @MainActor in
            guard store.state.chat.activeConversationID == conversationID else { return }
            store.dispatch(.chat(.visibleMessagesUpdated(
                rows,
                oldestLoadedSeq: oldest,
                hasMoreOlder: hasMore
            )))
            // 正在看这个会话时，持续清未读并推进已读水位（对方新消息到达）。
            await services.realtime.markConversationRead(
                conversationID: conversationID,
                userID: myUserID
            )
        }
    }
}

private struct LoadedMessageWindow: Sendable {
    let messages: [ChatState.MessageRow]
    let oldestLoadedSeq: Int64?
    let hasMoreOlder: Bool
}

private func loadVisibleMessageWindow(
    database: LocalDatabase,
    conversationID: String,
    myUserID: Int64,
    mode: MessageWindowLoadMode
) throws -> LoadedMessageWindow {
    let page = try database.fetchMessageWindow(conversationID: conversationID, mode: mode)
    return LoadedMessageWindow(
        messages: mapMessageRows(records: page.records, myUserID: myUserID),
        oldestLoadedSeq: page.oldestSeq,
        hasMoreOlder: page.hasMoreOlder
    )
}

private func mapMessageRows(
    records: [MessageRecord],
    myUserID: Int64
) -> [ChatState.MessageRow] {
    records.map { record in
        let isImage = record.messageType == "image"
        let attachment = isImage ? ImageMessageContent.parseAttachment(from: record.content) : nil
        let text = isImage ? "[图片]" : TextMessageContent.parseText(from: record.content)
        return ChatState.MessageRow(
            clientMessageID: record.clientMessageID,
            serverMessageID: record.serverMessageID,
            text: text,
            status: record.status,
            isMine: record.senderUserID == myUserID,
            messageType: record.messageType,
            imageObjectKey: attachment?.objectKey,
            imageWidth: attachment?.width,
            imageHeight: attachment?.height
        )
    }
}

private func copyToPasteboard(_ text: String) {
#if canImport(UIKit)
    UIPasteboard.general.string = text
#elseif canImport(AppKit)
    let pasteboard = NSPasteboard.general
    pasteboard.clearContents()
    pasteboard.setString(text, forType: .string)
#endif
}
