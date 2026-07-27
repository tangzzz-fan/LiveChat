import SwiftUI
import TGReduxKit
import ChatApplication
import ChatDomain
import ImageIO
#if canImport(PhotosUI)
import PhotosUI
#endif

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

/// WhatsApp 风格会话列表：标题 / 预览 / 时间 / 未读；工具入口收进菜单。
public struct ConversationListView: View {
    @Environment(Store<AppState, AppAction>.self) private var store
    @State private var showNewChat = false
    @State private var showTools = false

    public var body: some View {
        List {
            if let banner = store.state.chat.connectionBanner {
                Section {
                    Text(banner)
                        .font(.caption.monospaced())
                        .foregroundStyle(.secondary)
                }
            }

            Section {
                if store.state.chat.conversationRows.isEmpty {
                    ContentUnavailableView(
                        "暂无会话",
                        systemImage: "bubble.left.and.bubble.right",
                        description: Text("点右上角「新建」用对方 user_id 打开 1:1")
                    )
                    .listRowBackground(Color.clear)
                } else {
                    ForEach(store.state.chat.conversationRows) { row in
                        Button {
                            store.dispatch(.chat(.selectConversation(row.conversationID)))
                        } label: {
                            ConversationRowView(row: row)
                        }
                        .buttonStyle(.plain)
                    }
                }
            }

            if let error = store.state.chat.errorMessage {
                Section {
                    Text(error).foregroundStyle(.red).font(.footnote)
                }
            }
        }
        .listStyle(.plain)
        .navigationTitle("聊天")
        .toolbar {
            ToolbarItem(placement: .cancellationAction) {
                Button("工具") { showTools = true }
            }
            ToolbarItem(placement: .primaryAction) {
                Button {
                    showNewChat = true
                } label: {
                    Image(systemName: "square.and.pencil")
                }
                .accessibilityLabel("新建会话")
            }
            ToolbarItem(placement: .secondaryAction) {
                Button("退出") { store.dispatch(.auth(.logoutTapped)) }
            }
        }
        .sheet(isPresented: $showNewChat) {
            NavigationStack {
                Form {
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
                            showNewChat = false
                        }
                        .disabled(store.state.chat.isBusy || store.state.chat.peerUserIDInput.isEmpty)
                    }
                }
                .navigationTitle("新建聊天")
                .toolbar {
                    ToolbarItem(placement: .cancellationAction) {
                        Button("关闭") { showNewChat = false }
                    }
                }
            }
            .presentationDetents([.medium])
        }
        .sheet(isPresented: $showTools) {
            NavigationStack {
                Form {
                    Section("账号") {
                        LabeledContent("user_id", value: store.state.auth.userID.map(String.init) ?? "-")
                        LabeledContent("device_id", value: store.state.auth.deviceID ?? "-")
                    }
                    Section("同步") {
                        if let banner = store.state.chat.syncBanner {
                            Text(banner).font(.caption.monospaced())
                        }
                        Button(store.state.chat.isSyncing ? "同步中…" : "手动同步") {
                            store.dispatch(.chat(.syncTapped))
                        }
                        .disabled(store.state.chat.isSyncing)
                        Button("刷新会话列表") {
                            store.dispatch(.chat(.refreshConversationsTapped))
                        }
                    }
                    Section("推送 / 静默唤醒") {
                        if let banner = store.state.chat.pushTokenBanner {
                            Text(banner).font(.caption.monospaced())
                        }
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
                    }
                }
                .navigationTitle("工具")
                .toolbar {
                    ToolbarItem(placement: .cancellationAction) {
                        Button("完成") { showTools = false }
                    }
                }
            }
        }
        .onAppear {
            store.dispatch(.chat(.refreshConversationsTapped))
        }
    }
}

private struct ConversationRowView: View {
    let row: ChatState.ConversationRow

    var body: some View {
        HStack(spacing: 12) {
            ZStack {
                Circle()
                    .fill(Color.accentColor.opacity(0.18))
                    .frame(width: 48, height: 48)
                Text(String(row.title.prefix(1)).uppercased())
                    .font(.headline)
                    .foregroundStyle(Color.accentColor)
            }

            VStack(alignment: .leading, spacing: 4) {
                HStack {
                    Text(row.title)
                        .font(.headline)
                        .lineLimit(1)
                    Spacer(minLength: 8)
                    if let date = row.lastMessageAt {
                        Text(Self.timeString(date))
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                }
                HStack(alignment: .top) {
                    Text(row.preview.isEmpty ? "暂无消息" : row.preview)
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                        .lineLimit(2)
                    Spacer(minLength: 8)
                    if row.unreadCount > 0 {
                        Text(row.unreadCount > 99 ? "99+" : "\(row.unreadCount)")
                            .font(.caption2.weight(.semibold))
                            .foregroundStyle(.white)
                            .padding(.horizontal, 6)
                            .padding(.vertical, 2)
                            .background(Capsule().fill(Color.accentColor))
                    }
                }
            }
        }
        .padding(.vertical, 4)
        .contentShape(Rectangle())
    }

    private static func timeString(_ date: Date) -> String {
        let calendar = Calendar.current
        if calendar.isDateInToday(date) {
            return date.formatted(date: .omitted, time: .shortened)
        }
        if calendar.isDateInYesterday(date) {
            return "昨天"
        }
        return date.formatted(date: .abbreviated, time: .omitted)
    }
}

public struct ChatThreadView: View {
    @Environment(Store<AppState, AppAction>.self) private var store
    @Environment(\.mediaRepository) private var media
    let conversationID: String
    #if canImport(PhotosUI)
    @State private var pickerItem: PhotosPickerItem?
    #endif

    public var body: some View {
        VStack(spacing: 0) {
            if store.state.chat.hasMoreOlder {
                Button("加载更早消息") {
                    store.dispatch(.chat(.loadOlderMessagesTapped))
                }
                .font(.subheadline)
                .padding(.vertical, 8)
                .disabled(store.state.chat.isBusy)
            }

            ScrollViewReader { proxy in
                List(store.state.chat.visibleMessages) { message in
                    HStack {
                        if message.isMine { Spacer(minLength: 40) }
                        VStack(alignment: message.isMine ? .trailing : .leading, spacing: 4) {
                            if message.isImage, let objectKey = message.imageObjectKey {
                                MessageImageBubble(
                                    objectKey: objectKey,
                                    conversationID: conversationID,
                                    media: media
                                )
                            } else {
                                Text(message.text)
                            }
                            HStack(spacing: 4) {
                                if message.isMine {
                                    Text(statusGlyph(for: message.status))
                                        .font(.caption2)
                                        .foregroundStyle(message.status == "read" ? Color.accentColor : Color.secondary)
                                }
                                Text(message.serverMessageID ?? message.status)
                                    .font(.caption2.monospaced())
                                    .foregroundStyle(.secondary)
                                    .lineLimit(1)
                            }
                        }
                        .padding(8)
                        .background(message.isMine ? Color.accentColor.opacity(0.15) : Color.secondary.opacity(0.12))
                        .clipShape(RoundedRectangle(cornerRadius: 10))
                        if !message.isMine { Spacer(minLength: 40) }
                    }
                    .listRowSeparator(.hidden)
                    .id(message.id)
                }
                .listStyle(.plain)
                .onAppear {
                    scrollToLatest(proxy: proxy, animated: false)
                }
                // 仅当「列表尾部消息 id」变化时滚动（发送/接收）；加载更早不会改 last.id。
                .onChange(of: store.state.chat.visibleMessages.last?.id) { _, _ in
                    scrollToLatest(proxy: proxy, animated: true)
                }
            }

            HStack(spacing: 10) {
                #if canImport(PhotosUI)
                PhotosPicker(selection: $pickerItem, matching: .images) {
                    Image(systemName: "photo")
                }
                .disabled(store.state.chat.isBusy)
                .onChange(of: pickerItem) { _, item in
                    guard let item else { return }
                    Task {
                        await sendPickedImage(item)
                        pickerItem = nil
                    }
                }
                #endif
                TextField(
                    "输入消息",
                    text: store.binding(
                        get: { $0.chat.composeDraft },
                        send: { .chat(.updateDraft($0)) }
                    )
                )
                Button("发送") { store.dispatch(.chat(.sendTapped)) }
                    .disabled(store.state.chat.composeDraft.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
                Button("重试") { store.dispatch(.chat(.retryQueuedTapped)) }
            }
            .padding()
        }
        .navigationTitle(threadTitle)
        #if os(iOS)
        .navigationBarTitleDisplayMode(.inline)
        #endif
        .toolbar {
            ToolbarItem(placement: .cancellationAction) {
                Button("返回") { store.dispatch(.chat(.leaveConversation)) }
            }
        }
    }

    private func scrollToLatest(proxy: ScrollViewProxy, animated: Bool) {
        guard let lastID = store.state.chat.visibleMessages.last?.id else { return }
        // 等 List 完成布局后再滚，否则偶发停在旧位置。
        DispatchQueue.main.async {
            if animated {
                withAnimation(.easeOut(duration: 0.2)) {
                    proxy.scrollTo(lastID, anchor: .bottom)
                }
            } else {
                proxy.scrollTo(lastID, anchor: .bottom)
            }
        }
    }

    private var threadTitle: String {
        store.state.chat.conversationRows
            .first(where: { $0.conversationID == conversationID })?
            .title ?? conversationID
    }

    private func statusGlyph(for status: String) -> String {
        switch status {
        case "queued", "sending": return "○"
        case "accepted": return "✓"
        case "delivered": return "✓✓"
        case "read": return "✓✓"
        case "failed": return "!"
        default: return status
        }
    }

    #if canImport(PhotosUI)
    @MainActor
    private func sendPickedImage(_ item: PhotosPickerItem) async {
        do {
            guard let raw = try await item.loadTransferable(type: Data.self),
                  let jpeg = reencodeJPEG(raw)
            else {
                store.dispatch(.chat(.failed("无法读取图片")))
                return
            }
            let (width, height) = imagePixelSize(jpeg) ?? (800, 600)
            store.dispatch(
                .chat(.sendImageTapped(
                    jpeg,
                    width: width,
                    height: height,
                    mimeType: "image/jpeg",
                    fileName: "photo.jpg"
                ))
            )
        } catch {
            store.dispatch(.chat(.failed(error.localizedDescription)))
        }
    }
    #endif
}

/// 缩略展示：缓存优先；离屏 onDisappear 取消 Task（高负载 #9）。
struct MessageImageBubble: View {
    let objectKey: String
    let conversationID: String
    let media: (any MediaRepository)?

    @State private var image: CGImage?
    @State private var loadTask: Task<Void, Never>?
    @State private var failed = false

    var body: some View {
        Group {
            if let image {
                Image(decorative: image, scale: 1)
                    .resizable()
                    .scaledToFit()
                    .frame(maxWidth: 220, maxHeight: 220)
            } else if failed {
                Text("图片加载失败")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            } else {
                ProgressView()
                    .frame(width: 120, height: 80)
            }
        }
        .onAppear { startLoad() }
        .onDisappear {
            loadTask?.cancel()
            loadTask = nil
        }
    }

    private func startLoad() {
        guard image == nil, loadTask == nil, let media else { return }
        loadTask = Task {
            do {
                let data = try await media.downloadImage(
                    objectKey: objectKey,
                    conversationID: conversationID
                )
                try Task.checkCancellation()
                let downsampled = downsampleImage(data, maxPixel: 640)
                await MainActor.run {
                    self.image = downsampled
                }
            } catch is CancellationError {
                // 离屏取消
            } catch {
                await MainActor.run { failed = true }
            }
        }
    }
}

// MARK: - Environment

private struct MediaRepositoryEnvironmentKey: EnvironmentKey {
    static let defaultValue: (any MediaRepository)? = nil
}

public extension EnvironmentValues {
    var mediaRepository: (any MediaRepository)? {
        get { self[MediaRepositoryEnvironmentKey.self] }
        set { self[MediaRepositoryEnvironmentKey.self] = newValue }
    }
}

// MARK: - Image helpers

private func imagePixelSize(_ data: Data) -> (Int, Int)? {
    guard let source = CGImageSourceCreateWithData(data as CFData, nil),
          let props = CGImageSourceCopyPropertiesAtIndex(source, 0, nil) as? [CFString: Any],
          let w = props[kCGImagePropertyPixelWidth] as? Int,
          let h = props[kCGImagePropertyPixelHeight] as? Int
    else { return nil }
    return (w, h)
}

private func downsampleImage(_ data: Data, maxPixel: CGFloat) -> CGImage? {
    let options: [CFString: Any] = [kCGImageSourceShouldCache: false]
    guard let source = CGImageSourceCreateWithData(data as CFData, options as CFDictionary) else {
        return nil
    }
    let downsample: [CFString: Any] = [
        kCGImageSourceCreateThumbnailFromImageAlways: true,
        kCGImageSourceThumbnailMaxPixelSize: maxPixel,
        kCGImageSourceCreateThumbnailWithTransform: true,
        kCGImageSourceShouldCacheImmediately: true,
    ]
    return CGImageSourceCreateThumbnailAtIndex(source, 0, downsample as CFDictionary)
}

private func reencodeJPEG(_ data: Data, quality: CGFloat = 0.85) -> Data? {
    let options: [CFString: Any] = [kCGImageSourceShouldCache: false]
    guard let source = CGImageSourceCreateWithData(data as CFData, options as CFDictionary),
          let cgImage = CGImageSourceCreateImageAtIndex(source, 0, nil)
    else { return nil }
    let mutable = NSMutableData()
    guard let destination = CGImageDestinationCreateWithData(
        mutable, "public.jpeg" as CFString, 1, nil
    ) else { return nil }
    let props: [CFString: Any] = [kCGImageDestinationLossyCompressionQuality: quality]
    CGImageDestinationAddImage(destination, cgImage, props as CFDictionary)
    guard CGImageDestinationFinalize(destination) else { return nil }
    return mutable as Data
}
