import SwiftUI
import TGReduxKit

public struct HomeView: View {
    @Environment(Store<AppState, AppAction>.self) private var store

    public init() {}

    public var body: some View {
        NavigationStack {
            if let conversationID = store.state.chat.activeConversationID {
                ChatThreadView(conversationID: conversationID)
            } else {
                ConversationListView()
            }
        }
    }
}

public struct ConversationListView: View {
    @Environment(Store<AppState, AppAction>.self) private var store

    public var body: some View {
        List {
            Section("账号") {
                LabeledContent("user_id", value: store.state.auth.userID.map(String.init) ?? "-")
                LabeledContent("device_id", value: store.state.auth.deviceID ?? "-")
            }

            Section("打开 1:1") {
                TextField(
                    "对方 user_id",
                    text: store.binding(
                        get: { $0.chat.peerUserIDInput },
                        send: { .chat(.setPeerUserIDInput($0)) }
                    )
                )
                #if os(iOS)
                .keyboardType(.numberPad)
                #endif
                Button("创建/打开会话") {
                    store.dispatch(.chat(.openDirectTapped))
                }
                .disabled(store.state.chat.isBusy || store.state.chat.peerUserIDInput.isEmpty)
            }

            Section("会话") {
                if store.state.chat.conversationRows.isEmpty {
                    Text("暂无会话，先打开 1:1")
                        .foregroundStyle(.secondary)
                } else {
                    ForEach(store.state.chat.conversationRows) { row in
                        Button {
                            store.dispatch(.chat(.selectConversation(row.conversationID)))
                        } label: {
                            VStack(alignment: .leading, spacing: 4) {
                                Text(row.title).font(.headline)
                                Text(row.preview.isEmpty ? row.conversationID : row.preview)
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                            }
                        }
                    }
                }
            }

            Section("同步") {
                if let banner = store.state.chat.connectionBanner {
                    Text(banner).font(.caption.monospaced())
                }
                if let banner = store.state.chat.syncBanner {
                    Text(banner).font(.caption.monospaced())
                }
                Button(store.state.chat.isSyncing ? "同步中…" : "手动同步") {
                    store.dispatch(.chat(.syncTapped))
                }
                .disabled(store.state.chat.isSyncing)
            }

            Section("推送 / 静默唤醒") {
                if let banner = store.state.chat.pushTokenBanner {
                    Text(banner).font(.caption.monospaced())
                }
                Text("后台不硬撑 WS；静默唤醒只跑增量 sync（Spec 13 §8.2）")
                    .font(.caption2)
                    .foregroundStyle(.secondary)
                Button("注册 Push Token（mock）") {
                    store.dispatch(.chat(.registerPushTokenTapped))
                }
                Button("模拟静默唤醒 → sync") {
                    store.dispatch(.chat(.silentPushWakeTapped))
                }
                .disabled(store.state.chat.isSyncing)
            }

            Section("设备") {
                ForEach(store.state.auth.deviceSummaries, id: \.self) { line in
                    Text(line).font(.caption.monospaced())
                }
                Button("刷新设备") { store.dispatch(.auth(.refreshDevicesTapped)) }
                Button("刷新会话") { store.dispatch(.chat(.refreshConversationsTapped)) }
            }

            if let error = store.state.chat.errorMessage {
                Section {
                    Text(error).foregroundStyle(.red).font(.footnote)
                }
            }
        }
        .navigationTitle("LiveChat")
        .toolbar {
            ToolbarItem(placement: .primaryAction) {
                Button("退出") { store.dispatch(.auth(.logoutTapped)) }
            }
        }
    }
}

public struct ChatThreadView: View {
    @Environment(Store<AppState, AppAction>.self) private var store
    let conversationID: String

    public var body: some View {
        VStack(spacing: 0) {
            List(store.state.chat.visibleMessages) { message in
                HStack {
                    if message.isMine { Spacer(minLength: 40) }
                    VStack(alignment: message.isMine ? .trailing : .leading, spacing: 4) {
                        Text(message.text)
                        Text(message.serverMessageID ?? message.status)
                            .font(.caption2.monospaced())
                            .foregroundStyle(.secondary)
                    }
                    .padding(8)
                    .background(message.isMine ? Color.accentColor.opacity(0.15) : Color.secondary.opacity(0.12))
                    .clipShape(RoundedRectangle(cornerRadius: 10))
                    if !message.isMine { Spacer(minLength: 40) }
                }
                .listRowSeparator(.hidden)
            }
            .listStyle(.plain)

            HStack {
                TextField(
                    "输入消息",
                    text: store.binding(
                        get: { $0.chat.composeDraft },
                        send: { .chat(.updateDraft($0)) }
                    )
                )
                Button("发送") { store.dispatch(.chat(.sendTapped)) }
                    .disabled(store.state.chat.composeDraft.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
                Button("重试队列") { store.dispatch(.chat(.retryQueuedTapped)) }
            }
            .padding()
        }
        .navigationTitle(conversationID)
        #if os(iOS)
        .navigationBarTitleDisplayMode(.inline)
        #endif
        .toolbar {
            ToolbarItem(placement: .cancellationAction) {
                Button("返回") { store.dispatch(.chat(.leaveConversation)) }
            }
        }
    }
}
