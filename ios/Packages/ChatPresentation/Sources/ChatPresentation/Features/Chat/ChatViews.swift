import SwiftUI
import TGReduxKit
import ChatApplication
import ChatDomain
import ImageIO
#if canImport(UIKit)
import UIKit
#endif
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
    @FocusState private var isComposerFocused: Bool
    @State private var scrollToLatestTask: Task<Void, Never>?
    /// 加载更早后，把视口钉回加载前的首条，避免跳到最新。
    @State private var restoreScrollMessageID: String?
    #if canImport(PhotosUI)
    @State private var pickerItem: PhotosPickerItem?
    #endif

    public var body: some View {
        VStack(spacing: 0) {
            if store.state.chat.hasMoreOlder {
                Button("加载更早消息") {
                    restoreScrollMessageID = store.state.chat.visibleMessages.first?.id
                    store.dispatch(.chat(.loadOlderMessagesTapped))
                }
                .font(.subheadline)
                .padding(.vertical, 8)
                .disabled(store.state.chat.isBusy)
            }

            ScrollViewReader { proxy in
                List(store.state.chat.visibleMessages) { message in
                    let selected = store.state.chat.selectedClientMessageIDs.contains(message.clientMessageID)
                    HStack(spacing: 8) {
                        if store.state.chat.isMultiSelecting {
                            Image(systemName: selected ? "checkmark.circle.fill" : "circle")
                                .foregroundStyle(selected ? Color.accentColor : Color.secondary)
                        }
                        if message.isMine { Spacer(minLength: 40) }
                        VStack(alignment: message.isMine ? .trailing : .leading, spacing: 4) {
                            if message.isImage, let objectKey = message.imageObjectKey {
                                MessageImageBubble(
                                    objectKey: objectKey,
                                    conversationID: conversationID,
                                    media: media,
                                    pixelWidth: message.imageWidth,
                                    pixelHeight: message.imageHeight
                                )
                            } else {
                                Text(message.text)
                            }
                            HStack(spacing: 4) {
                                if message.isMine {
                                    if message.status == "queued" || message.status == "sending" {
                                        ProgressView()
                                            .controlSize(.mini)
                                        Button("取消") {
                                            store.dispatch(.chat(.cancelSendTapped(message.clientMessageID)))
                                        }
                                        .font(.caption2)
                                        .buttonStyle(.borderless)
                                    }
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
                        .contextMenu {
                            if !store.state.chat.isMultiSelecting {
                                if !message.isImage {
                                    Button("复制") {
                                        store.dispatch(.chat(.copyMessageTapped(message.text)))
                                    }
                                }
                                Button("转发") {
                                    store.dispatch(.chat(.forwardMessageTapped(message.clientMessageID)))
                                }
                                Button("分享") {
                                    store.dispatch(.chat(.shareMessageTapped(message.clientMessageID)))
                                }
                                Button("选择") {
                                    store.dispatch(.chat(.enterMultiSelect(message.clientMessageID)))
                                }
                                if message.isMine,
                                   message.status == "queued" || message.status == "sending" {
                                    Button("取消发送", role: .destructive) {
                                        store.dispatch(.chat(.cancelSendTapped(message.clientMessageID)))
                                    }
                                }
                                Button("删除", role: .destructive) {
                                    store.dispatch(.chat(.deleteLocalMessageTapped(message.clientMessageID)))
                                }
                                if message.isMine, message.status == "failed" {
                                    Button("重试") {
                                        store.dispatch(.chat(.retryMessageTapped(message.clientMessageID)))
                                    }
                                }
                            }
                        }
                        if !message.isMine { Spacer(minLength: 40) }
                    }
                    .listRowSeparator(.hidden)
                    .listRowBackground(selected ? Color.accentColor.opacity(0.08) : Color.clear)
                    .id(message.id)
                    .contentShape(Rectangle())
                    .onTapGesture {
                        if store.state.chat.isMultiSelecting {
                            store.dispatch(.chat(.toggleMessageSelection(message.clientMessageID)))
                        } else {
                            dismissComposerIfNeeded()
                        }
                    }
                }
                .listStyle(.plain)
                .scrollDismissesKeyboard(.interactively)
                // iOS 17+：优先从底部锚定，减轻进会话先停在顶部的闪烁。
                .defaultScrollAnchor(.bottom)
                .simultaneousGesture(
                    TapGesture().onEnded {
                        dismissComposerIfNeeded()
                    }
                )
                .onAppear {
                    pinToLatest(proxy: proxy, animated: false)
                }
                .onDisappear {
                    scrollToLatestTask?.cancel()
                    scrollToLatestTask = nil
                }
                .onChange(of: store.state.chat.visibleMessages.last?.id) { _, newID in
                    guard newID != nil else { return }
                    // 加载更早通常不改 last.id；若正在 restore，跳过钉底。
                    if restoreScrollMessageID != nil { return }
                    pinToLatest(proxy: proxy, animated: false)
                }
                .onChange(of: store.state.chat.visibleMessages.count) { oldCount, newCount in
                    guard newCount > oldCount, let anchorID = restoreScrollMessageID else { return }
                    restoreScrollMessageID = nil
                    scrollToLatestTask?.cancel()
                    DispatchQueue.main.async {
                        proxy.scrollTo(anchorID, anchor: .top)
                    }
                }
                .onChange(of: isComposerFocused) { _, focused in
                    if focused {
                        pinToLatest(proxy: proxy, animated: true)
                    }
                }
                #if canImport(UIKit)
                .onReceive(NotificationCenter.default.publisher(for: UIResponder.keyboardDidShowNotification)) { _ in
                    pinToLatest(proxy: proxy, animated: true)
                }
                #endif
            }
        }
        .safeAreaInset(edge: .bottom, spacing: 0) {
            if store.state.chat.isMultiSelecting {
                multiSelectBar
            } else {
                composerBar
            }
        }
        .navigationTitle(threadTitle)
        #if os(iOS)
        .navigationBarTitleDisplayMode(.inline)
        #endif
        .toolbar {
            if store.state.chat.isMultiSelecting {
                ToolbarItem(placement: .cancellationAction) {
                    Button("取消") { store.dispatch(.chat(.exitMultiSelect)) }
                }
                ToolbarItem(placement: .primaryAction) {
                    Text("已选 \(store.state.chat.selectedClientMessageIDs.count)")
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                }
            } else {
                ToolbarItem(placement: .cancellationAction) {
                    Button("返回") { store.dispatch(.chat(.leaveConversation)) }
                }
            }
        }
        .sheet(
            isPresented: Binding(
                get: { store.state.chat.isForwardPickerPresented },
                set: { if !$0 { store.dispatch(.chat(.dismissForwardPicker)) } }
            )
        ) {
            ForwardConversationPickerView(
                excludeConversationID: conversationID,
                rows: store.state.chat.conversationRows
            )
        }
        #if canImport(UIKit)
        .sheet(
            item: Binding(
                get: { store.state.chat.sharePresentation },
                set: { if $0 == nil { store.dispatch(.chat(.clearSharePresentation)) } }
            )
        ) { payload in
            ActivityShareView(items: activityItems(for: payload))
        }
        #endif
    }

    #if canImport(UIKit)
    private func activityItems(for payload: ChatState.SharePresentation) -> [Any] {
        switch payload {
        case .text(let text):
            return [text]
        case .imageFilePath(let path):
            return [URL(fileURLWithPath: path)]
        }
    }
    #endif

    private var multiSelectBar: some View {
        HStack(spacing: 16) {
            Button("删除", role: .destructive) {
                store.dispatch(.chat(.batchDeleteSelectedTapped))
            }
            .disabled(store.state.chat.selectedClientMessageIDs.isEmpty)
            Button("转发") {
                store.dispatch(.chat(.forwardSelectedTapped))
            }
            .disabled(store.state.chat.selectedClientMessageIDs.isEmpty)
            Button("分享") {
                store.dispatch(.chat(.shareSelectedTapped))
            }
            .disabled(store.state.chat.selectedClientMessageIDs.isEmpty)
            Spacer()
        }
        .padding()
        .background(.bar)
    }

    private var composerBar: some View {
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
            .focused($isComposerFocused)
            #if os(iOS)
            .textInputAutocapitalization(.sentences)
            #endif
            .onSubmit {
                store.dispatch(.chat(.sendTapped))
            }
            Button("发送") { store.dispatch(.chat(.sendTapped)) }
                .disabled(store.state.chat.composeDraft.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
            Button("重试") { store.dispatch(.chat(.retryQueuedTapped)) }
        }
        .padding()
        .background(.bar)
    }

    private func dismissComposerIfNeeded() {
        guard isComposerFocused else { return }
        isComposerFocused = false
    }

    /// List 行 `.id` 往往在下一帧才挂上；用短重试钉底。
    /// 图片行高由元数据预留，不再靠 150ms 多帧硬顶（会抖）。
    private func pinToLatest(proxy: ScrollViewProxy, animated: Bool) {
        scrollToLatestTask?.cancel()
        scrollToLatestTask = Task { @MainActor in
            let delays: [UInt64] = [0, 80_000_000]
            for (index, delay) in delays.enumerated() {
                if delay > 0 {
                    try? await Task.sleep(nanoseconds: delay)
                }
                guard !Task.isCancelled else { return }
                guard store.state.chat.activeConversationID == conversationID else { return }
                guard restoreScrollMessageID == nil else { return }
                guard let lastID = store.state.chat.visibleMessages.last?.id else { return }
                if animated, index == 0 {
                    withAnimation(.easeOut(duration: 0.2)) {
                        proxy.scrollTo(lastID, anchor: .bottom)
                    }
                } else {
                    proxy.scrollTo(lastID, anchor: .bottom)
                }
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
        case "cancelled": return "×"
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

/// 缩略展示：按元数据预留固定框；缓存优先；离屏取消 Task（高负载 #9 / 0064）。
struct MessageImageBubble: View {
    let objectKey: String
    let conversationID: String
    let media: (any MediaRepository)?
    var pixelWidth: Int? = nil
    var pixelHeight: Int? = nil

    @State private var image: CGImage?
    @State private var loadTask: Task<Void, Never>?
    @State private var failed = false

    private var reservedSize: CGSize {
        imageBubbleDisplaySize(width: pixelWidth, height: pixelHeight)
    }

    var body: some View {
        ZStack {
            if let image {
                Image(decorative: image, scale: 1)
                    .resizable()
                    .scaledToFit()
            } else if failed {
                Text("图片加载失败")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            } else {
                ProgressView()
            }
        }
        .frame(width: reservedSize.width, height: reservedSize.height)
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

/// 气泡预留尺寸：按 attachment 宽高等比缩进，加载前后行高不变。
private func imageBubbleDisplaySize(width: Int?, height: Int?, maxSide: CGFloat = 220) -> CGSize {
    let rawW = CGFloat(max(width ?? 4, 1))
    let rawH = CGFloat(max(height ?? 3, 1))
    let scale = min(maxSide / rawW, maxSide / rawH, 1)
    return CGSize(width: (rawW * scale).rounded(), height: (rawH * scale).rounded())
}

// MARK: - Forward / Share (0058)

struct ForwardConversationPickerView: View {
    @Environment(Store<AppState, AppAction>.self) private var store
    let excludeConversationID: String
    let rows: [ChatState.ConversationRow]

    private var candidates: [ChatState.ConversationRow] {
        rows.filter { $0.conversationID != excludeConversationID }
    }

    var body: some View {
        NavigationStack {
            List {
                if candidates.isEmpty {
                    ContentUnavailableView(
                        "没有其他会话",
                        systemImage: "bubble.left.and.bubble.right",
                        description: Text("先打开另一会话，再回来转发")
                    )
                    .listRowBackground(Color.clear)
                } else {
                    ForEach(candidates) { row in
                        Button {
                            store.dispatch(.chat(.confirmForwardToConversation(row.conversationID)))
                        } label: {
                            VStack(alignment: .leading, spacing: 2) {
                                Text(row.title.isEmpty ? row.conversationID : row.title)
                                    .font(.body.weight(.medium))
                                    .foregroundStyle(.primary)
                                if !row.preview.isEmpty {
                                    Text(row.preview)
                                        .font(.caption)
                                        .foregroundStyle(.secondary)
                                        .lineLimit(1)
                                }
                            }
                        }
                    }
                }
            }
            .navigationTitle("转发到")
            #if os(iOS)
            .navigationBarTitleDisplayMode(.inline)
            #endif
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("取消") { store.dispatch(.chat(.dismissForwardPicker)) }
                }
            }
        }
    }
}

#if canImport(UIKit)
struct ActivityShareView: UIViewControllerRepresentable {
    let items: [Any]

    func makeUIViewController(context: Context) -> UIActivityViewController {
        UIActivityViewController(activityItems: items, applicationActivities: nil)
    }

    func updateUIViewController(_ uiViewController: UIActivityViewController, context: Context) {}
}
#endif

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
