# IM 客户端：Core SDK vs UI SDK

面向学习与产品拆分决策。**§1 不绑定 LiveChat 实现**，先讲业界（环信、融云、腾讯云 IM、网易云信等）常见分层；**§2 再对照本仓现有 SPM 包**，讨论若对外交付 SDK 应如何切。

相关： [Spec 13](../specs/13-iOS客户端架构设计.md) · [ios-client-rewrite](./ios-client-rewrite.md) · [ios-client-study-guide](./ios-client-study-guide.md) · 工程问题 [19](./engineering-problems/19-domain-repository-ports-vs-concrete-executors.md)

---

## 0. 为什么供应商一定拆两层

宿主 App 有两类买家：

| 买家 | 诉求 | 典型交付 |
|------|------|----------|
| **要聊天能力、UI 自己做** | 可控品牌、自定义会话流、嵌入既有导航 | **Core SDK only** |
| **要尽快上线聊天页** | 会话列表 / 气泡 / 输入栏开箱即用 | **Core + UI SDK（UIKit）** |

拆分的工程动机：

1. **依赖重量**：Core 可不依赖 SwiftUI/UIKit 业务组件；UI 必然绑 UI 框架与资源包。
2. **版本节奏**：协议/同步修 bug 高频发 Core；换皮肤发 UI，互不拖累。
3. **许可证与体积**：部分厂商 UIKit 另计费或可选下载。
4. **可测性**：Core 用 Fake 网络/DB 跑 CI；UI 用 Snapshot / UI Test，边界清晰。

经验法则：

> **Core = 真相与动作（data + actions）**  
> **UI = 呈现与手势（views + chrome）**  
> **中间常有一层极薄的「绑定 / ViewModel / Observer」，归属争议最大——成熟方案多半放 Core 的只读投影 API，或单独 `KitBridge` 包。**

---

## 1. 业界常见边界（与具体项目无关）

### 1.1 放进 Core SDK 的能力

Core 对宿主暴露的是 **可脚本化的聊天引擎**，不渲染气泡。

| 类别 | 典型内容 | 说明 |
|------|----------|------|
| **初始化与配置** | AppKey、环境、日志级别、存储路径、推送证书配置入口 | 单例或 `Client` 工厂；可多实例（少见） |
| **身份与会话** | 登录 / 登出、Token 刷新钩子、踢下线回调 | 常要求宿主提供 Token，SDK 不持账号密码 |
| **连接** | 长连接建连、心跳、重连策略、连接状态枚举 | UI 只订阅 `ConnectionState` |
| **会话** | 会话列表 CRUD、置顶、免打扰、草稿（若进本地库） | 返回模型，不画 Cell |
| **消息** | 发文本/图/语音/自定义、重发、撤回（若协议有）、本地删除 | 本地优先队列通常在 Core |
| **状态机** | 发送态、已送达、已读；幂等键 | **绝不能只在 UI 里维护** |
| **同步** | 增量 sync、游标、缺口补拉、多端收敛 | 后台可跑，不依赖页面存活 |
| **实时** | 收消息、回执、群事件 → 统一事件总线 / Delegate / AsyncStream | UI 订阅；Core 落库 |
| **本地存储** | SQLite/GRDB/厂商自研 DB、迁移、加密库（可选） | 单一真相源在 Core |
| **媒体管线（引擎侧）** | 上传凭证、缩略图生成、下载缓存键、进度回调 | 「选图器 UI」不在 Core |
| **推送适配钩子** | 注册 device token、解析静默 payload → 触发 sync | 系统权限弹窗仍在 App |
| **群组 / 关系（若产品有）** | 建群、成员、禁言策略 API | 管理页 UI 常在 UIKit 或宿主自研 |
| **可观测** | 内部 trace、错误码表、诊断导出 | 给宿主排障 |

**Core 对外形态（概念）**：

```text
Host App
   │  init / login / send / observe
   ▼
┌──────────────────────────────────────┐
│              Core SDK                │
│  Client · Connection · Conversation  │
│  Message · Sync · MediaEngine        │
│  LocalDB · EventBus                  │
└──────────────────────────────────────┘
```

宿主应能在无任何聊天 UI 的情况下：登录 → 发消息 → 收消息 → 读本地会话列表（打印或写进自己的首页）。

### 1.2 放进 UI SDK（UIKit）的能力

UI SDK **只依赖 Core**（或依赖 Core 公开的只读模型 + 命令接口），负责「看起来像聊天产品」。

| 类别 | 典型内容 | 说明 |
|------|----------|------|
| **会话列表页** | 头像、标题、预览、时间、未读角标、滑动操作 | 数据来自 Core 投影 |
| **会话页（Thread）** | 气泡、时间分隔、已读符、下拉历史、滚底 | 分页窗口由 Core 提供 |
| **输入区** | 文本框、表情、+ 号面板、语音按住 | 最终调用 Core `send` |
| **媒体交互** | 相册/相机、大图浏览、进度条 UI | 上传仍走 Core |
| **消息手势** | 长按菜单、多选、转发页、分享 sheet | 动作落到 Core；菜单是 UI |
| **连接横幅 / 弱网提示** | 「连接中…」「发送失败点重试」 | 状态来自 Core |
| **主题与皮肤** | 色板、气泡资源、字体、暗色模式 | 常可配置；默认皮肤在 UI 包 |
| **导航胶水** | 推会话、present 选人、路由协议**实现** | 路由**协议**可放 Core 或 Host |
| **本地化文案** | 「昨天」「重试」「已读」等 UI 字符串 | 错误码文案也可部分在 Core |

**UI 对外形态（概念）**：

```text
Host App
   │  push ConversationListController / ChatView
   ▼
┌──────────────────────────────────────┐
│              UI SDK                  │
│  ListVC · ChatVC · InputBar · Theme  │
└──────────────────┬───────────────────┘
                   │ subscribe / command
                   ▼
              Core SDK
```

### 1.3 灰色地带（最容易拆错）

| 能力 | 更合理归属 | 原因 |
|------|------------|------|
| **消息 Cell 布局计算**（高度缓存） | UI，可抽 `UILayout` 子模块 | 依赖字体/气泡图；Core 不应知 UIKit |
| **「未读数」业务规则** | Core | 与 sync/已读 ACK 绑定；UI 只展示 |
| **草稿** | Core（持久）+ UI（编辑器） | 换皮肤草稿不能丢 |
| **@ 提及解析** | Core 出模型；UI 高亮 | 发送 payload 在 Core |
| **翻译 / 审核结果展示** | 结果进 Core 扩展字段；渲染在 UI | |
| **通话信令** | 常独立 Call SDK；UI 另包 | 别塞进消息 Core |
| **Push 通知内容拼装** | App / NSE；Core 可提供摘要 API | 系统 Notification 不在 SDK UI |
| **登录页 / OTP** | **多数厂商不放进 IM UIKit** | 账号体系属宿主；IM 只收 Token |

### 1.4 集成模式对照

| 模式 | 宿主做什么 | 适用 |
|------|------------|------|
| **A. Core only** | 自研全部聊天 UI，调 Client API | 强品牌、复杂业务嵌入 |
| **B. Core + 官方 UI** | 换肤 / 少量自定义 Cell | 快速上线 |
| **C. Core + 官方 UI + 自定义页面** | 列表用官方，会话页自研（或反之） | 常见折中 |
| **D. 源码级 UI** | 厂商开 UI 源码，Core 闭源 | 深度定制但仍要升级 Core |

评判拆分是否健康的三条测试：

1. **无 UI 可测**：关掉 UI 包，Core 单测仍能走完收发/同步。
2. **换皮不改协议**：只改 UI 资源与组件，消息字节流不变。
3. **宿主可替换列表**：用自己的 `UITableView` 绑 Core 的 conversation observer，不 fork Core。

### 1.5 反模式（供应商文档里偶发，应避免）

- UI 包内直接 HTTP/WebSocket，绕过 Core（双通道、双状态机）。
- Core 依赖具体 `ChatViewController` 类型（反向依赖）。
- 「发送中」只存在于 Cell 的 `@State`，杀进程后丢失。
- UIKit 强制单例 `shared`，无法多账号 / 预览注入 Fake。

---

## 2. 若 LiveChat 拆成 SDK：怎么切

本节对照**当前仓库**，讨论可落地的产品形态。当前工程是 **学习用 App + 内部 SPM 包**，尚未对外发版；以下是「若要 SDK 化」的目标架构，不是要求立刻搬家。

### 2.1 现状与供应商分层的映射

当前包（Spec 13）：

```text
ChatPresentation  →  偏 UI SDK（SwiftUI + TGReduxKit）
ChatApplication   →  组合根 / 用例边界（偏 Core 的「门面」）
ChatDomain        →  Core 的领域内核
ChatInfrastructure→  Core 的适配器（DB/HTTP/WS/Media/Push）
薄 App target     →  宿主
```

| 现有模块 | 更像 | SDK 化建议 |
|----------|------|------------|
| `ChatDomain` | Core 内核 | **必须进 Core**；零 UI 依赖保持 |
| `ChatInfrastructure` | Core 实现 | **进 Core**（或 `LiveChatCore` 内 target） |
| `ChatApplication` / `AppServices` | Core 门面 | **进 Core 公开 API**（`LiveChatClient`） |
| `ChatPresentation` | UI SDK | **`LiveChatUI`**（可选依赖） |
| App（登录壳、权限、组装） | Host | **留在 App**；OTP/Keychain 策略可由 Core 提供存储接口，登录 UI 默认不进 UI SDK |

**关键结论**：本仓已经按「Presentation 不碰 Infrastructure」切开，**距离厂商 Core/UI 拆分只差「对外稳定 API + 可选 UI 包」**，而不是从零分层。

### 2.2 推荐产品包形态

```text
┌─────────────────────────────────────────────────────────┐
│  Host App（LiveChat iOS App）                           │
│  权限、Push 注册、深链、品牌壳、可选自研页面               │
└─────────────┬───────────────────────────┬───────────────┘
              │                           │
              ▼                           ▼
┌─────────────────────────┐   ┌───────────────────────────┐
│  LiveChatUI（可选）      │   │  宿主自研 SwiftUI/UIKit   │
│  ConversationListView   │   │  只依赖 LiveChatCore      │
│  ChatThreadView         │   └─────────────┬─────────────┘
│  Composer · Theme       │                 │
└─────────────┬───────────┘                 │
              └──────────────┬──────────────┘
                             ▼
              ┌──────────────────────────────┐
              │  LiveChatCore                │
              │  ┌────────────────────────┐  │
              │  │ Public: LiveChatClient │  │
              │  │ Events / Snapshots     │  │
              │  └──────────┬─────────────┘  │
              │  Domain + Infrastructure     │
              │  (GRDB, WS, HTTP, Executors) │
              └──────────────────────────────┘
```

SPM 示意（目标态，非现状文件名强制）：

| Package / Product | 类型 | 依赖 |
|-------------------|------|------|
| `LiveChatCore` | library | Domain+Infra 合并或内部多 target；**不依赖** SwiftUI |
| `LiveChatUI` | library | `LiveChatCore` + SwiftUI（+ 可选 TGReduxKit） |
| App | executable | `LiveChatUI` 或仅 `LiveChatCore` |

### 2.3 Core 对外 API 草案（稳定面）

宿主与 UI SDK **只应依赖这一层**，不 import GRDB / protobuf 生成细节（生成物可 `internal`）。

```swift
/// 概念草案 — 说明边界，非现成代码
public final class LiveChatClient: @unchecked Sendable {
    public init(config: LiveChatConfig) async throws

    // Identity
    public func login(accessToken: String, deviceID: String) async throws
    public func logout() async throws

    // Connection
    public var connectionStates: AsyncStream<ConnectionState>

    // Conversations
    public func conversations() -> AsyncStream<[ConversationSummary]>
    public func openConversation(_ id: String) async throws -> ConversationHandle

    // Messaging (local-first)
    public func sendText(conversationID: String, text: String) async throws -> ClientMessageID
    public func sendImage(conversationID: String, jpegData: Data) async throws -> ClientMessageID
    public func cancelSend(clientMessageID: ClientMessageID) async throws
    public func retry(clientMessageID: ClientMessageID) async throws
    public func deleteLocally(clientMessageID: ClientMessageID) async throws

    // Thread window
    public func messages(conversationID: String, window: MessageWindowQuery)
        -> AsyncStream<[MessageView]>

    // Read / sync hooks
    public func markRead(conversationID: String) async throws
    public func syncNow() async throws

    // Push hooks (App 调)
    public func registerPushToken(_ token: Data) async throws
    public func handleSilentNotification(_ userInfo: [AnyHashable: Any]) async -> SilentWakeOutcome
}
```

与现状锚点：

| 草案 API | 今日大致对应 |
|----------|----------------|
| `LiveChatClient` | `AppServices` + 生命周期编排 |
| `send*` / 队列 | `MessageSendExecutor` |
| `messages` 流 | `LocalProjectionObserver` + `MessageWindow` |
| `connectionStates` | `RealtimeSession` / path monitor 投影 |
| `syncNow` | `SyncExecutor` |
| `markRead` | `RealtimeSession.markConversationRead` + 清未读 |
| Ports / Fakes | `PortFakes` → Core 测试与宿主 Demo |

**刻意不进公开 Core API 的东西**：`DatabaseQueue`、原始 `WsFrame`、具体 HTTP path、Redux `Action` 枚举。

### 2.4 UI SDK 边界（对照 0055–0058）

| UI 模块 | 依赖 Core | 不该做的事 |
|---------|-----------|------------|
| 会话列表 | `conversations()` | 自己算未读真相 |
| 会话页 | `messages` + `markRead` | 直连 `MessageAPI` |
| Composer | `sendText` / `sendImage` | 本地再维护一套发送状态机 |
| 发送中 / 取消（0055） | `cancelSend` + 状态字段 | 只 cancel UI Task |
| 长按菜单（0056） | `deleteLocally` / `retry` | 菜单里写 SQL |
| 多选 / 转发（0057–0058） | Core 批量命令 + 选会话 API | UI 包内复制消息表 |

主题、气泡样式、菜单文案 → **仅 LiveChatUI**。  
`MessageStatus` 枚举与转移规则 → **仅 LiveChatCore**（Domain）。

### 2.5 Auth / Push / 媒体：SDK 与 Host 再切一刀

| 能力 | LiveChat 建议 | 理由 |
|------|---------------|------|
| OTP 登录页 | **Host**（可提供 Sample） | 账号体系属产品；Core 收 Token |
| Token / Keychain | Core 提供 `TokenStore` 协议 + 默认 Keychain 实现 | 与 Session Version 绑定 |
| APNs 权限弹窗 | Host | 系统 API |
| Token 上报 / 静默 sync | Core | 已有 `PushTokenAPI` / `SilentSyncWakeHandler` |
| PhotosPicker | UI SDK | 选完 Data 交 Core |
| 上传与签名 URL | Core | 工程问题 13 |

### 2.6 推荐落地阶段（深入执行序）

不必一次改名搬家；按可验证切片推进：

| 阶段 | 做什么 | 验收 |
|------|--------|------|
| **S0 文档与边界** | 本文 + Spec 13 增「SDK 产品面」小节（可选） | 团队认同 Core/UI 表 |
| **S1 收拢门面** | 把 `AppServices` 常用能力收成 `LiveChatClient`（仍在仓内），Presentation **只**经 Client | App 行为不变；UI 不再碰 Executor 类型 |
| **S2 隐藏基础设施** | Infrastructure 改为 `internal` / 不对外 product | 宿主 Demo 无法 `import` GRDB 细节 |
| **S3 可选 UI 包** | `ChatPresentation` 改名为 `LiveChatUI` product；App 可选依赖 | 另做一个 **Core-only Demo**（命令行或最小 SwiftUI 自绘列表） |
| **S4 二进制 / 版本** | 语义化版本、Core/UI 独立 tag、changelog | UI x.y 可依赖 Core ≥a.b |
| **S5 多端** | Android 对称 Core（Kotlin）+ UI；协议与状态机对齐 Spec | 非本仓短期目标 |

**不要做的过早优化**：一上来拆十几个 micro-SDK（Call、Beauty、Search…）；先 Core/UI 两刀。

### 2.7 与「Ports 演进」（0051–0053）的关系

细粒度 `MessageStore` / `MessageRemote` 等，对 SDK 化是加分项：

- Core **内部**继续用 Port 拼 Executor；
- **对外**只暴露 Client 用例 API；
- `PortFakes` 成为 **Core 单测 + UI Previews** 的标准注入方式。

避免把 Port 协议全部 `public` 给宿主——那会变成「宿主自己实现半个 IM」。除非要做 **Provider 插件**（自建存储），否则 Port 保持 `public` 给 UI 同 module 或 `package` 可见即可。

### 2.8 Core-only Demo 应覆盖的剧本

用于证明拆分有效（可对照 `docs/ios-app-testing.md`）：

1. Token 登录 → 连接态变 Ready  
2. 建/开 1:1 → `sendText` → 本地 `queued→accepted`  
3. 第二设备 / 第二 Client → sync 或 WS 收到消息  
4. 杀进程重启 → 会话列表与消息仍在  
5. 断网发送 → 恢复后续跑（0046）  
6. 静默通知注入 → `syncNow` 被触发  

**全程不 import LiveChatUI。**

### 2.9 风险与取舍

| 风险 | 缓解 |
|------|------|
| Client 门面变成上帝类 | 按域拆 `client.messages` / `client.conversations` 命名空间，仍一个入口类型 |
| UI 为图省事再次直连 Infra | CI 依赖规则：UI target 不得 link 非 Core 的 Infra |
| Redux Action 泄漏成「公开协议」 | Action 留在 UI 包；Core 用领域事件 / AsyncStream |
| 学习仓过早产品化 | 停在 S1–S3 即可；S4 按需 |

---

## 3. 一页对照表（决策用）

| 问题 | Core | UI |
|------|------|----|
| 消息会不会丢？ | ✓ | |
| 气泡长什么样？ | | ✓ |
| `conversation_seq` 排序？ | ✓ | |
| 长按复制？ | 提供 delete/retry API | ✓ 菜单 |
| 取消发送？ | ✓ 状态机 + 取消 Task | ✓ 按钮 |
| 主题色？ | | ✓ |
| OTP 验证码页？ | Token 存储钩子 | 默认 Host |
| GRDB schema？ | ✓ | |
| TGReduxKit Store？ | | ✓（或 Host 自研状态） |

---

## 4. 小结

1. **业界**：Core 管连接、存储、收发、同步、状态机与事件；UI 管列表/会话/输入/手势/主题；登录页与系统权限多半留宿主。  
2. **本仓**：`Domain + Infrastructure + AppServices` ≈ Core；`ChatPresentation` ≈ UI；薄 App ≈ Host。  
3. **落地**：先收 `LiveChatClient` 门面与依赖方向，再可选拆 `LiveChatUI`，用 Core-only Demo 验收；不必立刻商业发版。

后续若要动代码，建议单独开票（例如「S1：AppServices → LiveChatClient 门面」），避免与 0055–0058 功能票缠在一起。
