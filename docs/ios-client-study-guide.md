# iOS 客户端复习导读（0035–0048）

面向「做完功能后回头复习」：把各票涉及的技术映射到已有文档，并标明**缺口**与**补齐位置**。

相关决策：[ios-client-rewrite.md](./ios-client-rewrite.md) · 高负载：[ios-high-load-client.md](./ios-high-load-client.md) · 联调：[ios-app-testing.md](./ios-app-testing.md) · Spec：[specs/13-iOS客户端架构设计.md](../specs/13-iOS客户端架构设计.md)

---

## 1. 结论先说

| 问题 | 结论 |
|------|------|
| 票里的技术是否都有对应文档？ | **大框架有**；0040/0041 踩坑见工程问题 16–18；**0043–0048** 见本文 §2 / §5–§7 + 高负载对照表 + 工程问题 **19**。 |
| 粘包？ | **走 `URLSessionWebSocketTask` 时，应用层几乎不会撞上经典 TCP 粘包**；见工程问题 16。 |
| Domain 仓储协议都实现了吗？ | **细粒度 Store/Remote 已落地（0052）**；`AuthRepository` + `WebSocketTransport` 保留；粗 `*Repository` 已删。见 §7 与工程问题 **19**。 |
| 高负载 10 项还剩什么？ | #9 图片（0049）、#横切 Instruments（0050）；其余主链路项已勾选。 |

---

## 2. 票 → 文档对照

| 票 | 主题 | 已有文档 | 本轮补齐 / 仍缺口 |
|----|------|----------|------------------|
| 0035 | 父票 / 重写范围 | [ios-client-rewrite.md](./ios-client-rewrite.md)、[0035](../issues/0035-ios-client-rewrite.md) | — |
| 0036 | Spec 修订 + 决策 | Spec 13、ios-client-rewrite | — |
| 0037 | SPM 脚手架 | [ios/README.md](../ios/README.md)、gen_proto 说明（rewrite §2） | **18**：proto 生成流程与版本坑 |
| 0038 | OTP + Keychain | [09](./engineering-problems/09-two-step-auth-code-storage.md)、[08](./engineering-problems/08-session-version-device-revocation.md)、ios-app-testing | — |
| 0039 | 本地优先发送 | [06](./engineering-problems/06-message-lifecycle-stages.md)、[01](./engineering-problems/01-message-durability-outbox.md)、[02](./engineering-problems/02-message-ordering-sequence.md) | 发送队列/429；深度见本文 §4 |
| 0040 | 增量 sync | [05](./engineering-problems/05-offline-gap-detection.md)、ios-high-load「sync 洪流」 | cursor 单调 + 与 WS 共用落库见本文 §4 |
| 0041 | WS 实时 | [03](./engineering-problems/03-reconnection-storm.md)、ios-client-rewrite §4–5 | **16** 帧边界；**17** Task/`AsyncStream` |
| 0042 | Push | [11](./engineering-problems/11-push-deduplication-coalescing.md)、ios-app-testing §2.3 | mock token + 静默注入 |
| 0043 | 高负载剩余父票 | [ios-high-load-client.md](./ios-high-load-client.md)、[0043](../issues/0043-ios-high-load-leftover.md) | 子票依赖与实现序见父票 |
| 0044 | 消息窗按 `conversation_seq` + 分页 | Spec 06 §5.3、高负载 #2/#10 | 本文 §5；`MessageWindow` / `fetchLatestMessageWindow` |
| 0045 | ValueObservation 去抖 | 高负载 #1、ios-client-rewrite 桥接段 | 本文 §5；`LocalProjectionObserver` |
| 0046 | 弱网 path 续跑 + sending 超时 | 高负载 #5、Spec 13 §8.3 | 本文 §5；`SendPathResumeMonitor` |
| 0047 | 会话 seq 缺口补拉 | [05](./engineering-problems/05-offline-gap-detection.md)、Spec 06 §4.4 | 本文 §5；`ConversationGapBackfill`；服务端 `latest_seq` |
| 0048 | 静默唤醒预算 | Spec 13 §8.2、高负载 #7 | 本文 §5；`SilentWakeOutcome` |
| 0049 | 图片（开放） | 高负载 #9、工程问题 13 | 未做 |
| 0050 | 横切验收（开放） | 高负载「建议的横切验收项」 | 未做 |

过时注意：[`iOS多端接入评估与实现.md`](./iOS多端接入评估与实现.md) 仍写「iOS 未接 WS」，以 **0038–0048 实现 + 本文** 为准。

---

## 3. 推荐复习顺序

### 主链路（约 1–2 小时）

1. **边界与真相源**：ios-client-rewrite §3（GRDB vs Store）  
2. **发送语义**：工程问题 06 → `MessageSendExecutor`  
3. **离线补齐**：工程问题 05 → `SyncExecutor`  
4. **实时通道**：工程问题 **16 → 17** → `RealtimeSession`  
5. **协议工具链**：工程问题 **18** → `ios/scripts/gen_proto.sh`  
6. **横切**：ios-high-load-client  
7. **动手**：ios-app-testing 双模拟器剧本  

### 高负载剩余（0044–0048，约 45 分钟）

1. **排序与窗口**：Spec 06 §5.3 → `MessageWindow` / `LocalDatabase.messageWindow` → ChatThread「加载更早」  
2. **投影去抖**：高负载 #1 → `LocalProjectionObserver` → ChatMiddleware 不再靠 `databaseChanged` 手刷  
3. **两层补拉**：工程问题 05 → 全局 `SyncExecutor` vs 会话 `ConversationGapBackfill`（不推进 sync cursor）  
4. **弱网发送**：`SendPathResumeMonitor` + sending→queued 超时策略  
5. **后台预算**：`SilentSyncWakeHandler` ≤25s + AppDelegate `completionHandler`  
6. **端口幻觉**：工程问题 **19** → 打开 `RepositoryProtocols.swift` 对照 `AppServices`  

---

## 4. 收发消息边界条件清单（覆盖度）

图例：✅ 已处理 · ⚠️ 部分/有意识但未完整 · ❌ 未做（留给后续票）

### 4.1 传输层帧边界

| 边界 | 状态 | 说明 |
|------|------|------|
| TCP 粘包 / 半包 | ✅（下沉到 WS 栈） | `receive()` 交付**完整** WebSocket message |
| 一发一收语义 | ✅ | 1 个 WS binary message = 1 个 `WsFrame` protobuf |
| 一帧塞多条业务消息 | ✅ 约定禁止 | — |
| WS 分片帧 | ✅（系统重组） | — |
| `maximumMessageSize` | ⚠️ | 未单独配置 |
| 网关 `ReadFrame` 的 4 字节长度前缀 | ✅ 不混用 | iOS/压测 WS **不要**加长度前缀 |

### 4.2 发送路径

| 边界 | 状态 | 说明 |
|------|------|------|
| 弱网先落本地 | ✅ | `queued` → `sending` → `accepted` / `failed` |
| `client_message_id` 幂等 | ✅ | — |
| 有界发送队列 | ✅ | 满则 `SendQueueError.full` |
| HTTP `429` + Retry-After | ✅ | 退避抖动，保持 sending |
| path 恢复续跑 | ✅（0046） | `SendPathResumeMonitor` → `reclaimStaleSendingAndProcess` |
| sending 超时/孤儿 | ✅（0046） | 超时或无内存计时 → **queued**（允许 `sending→queued`） |
| 自己发的消息不靠 sync 回环 | ✅ | — |
| 展示排序 | ✅（0044） | 按 `conversation_seq`；无 seq 的 pending 置底 |

### 4.3 接收路径（WS + Sync）

| 边界 | 状态 | 说明 |
|------|------|------|
| WS / Sync 双通道重复 | ✅ | `IncomingMessageApplier` / `server_message_id` 幂等 |
| 突发投递打爆 UI | ✅（0045） | 批量落库 + ValueObservation ~16ms 去抖 |
| sync cursor 只前进 | ✅ | 单事件 apply 成功后才推进 |
| 会话 seq 缺口检测 | ✅（0047） | `ConversationGapBackfill` + `latest_seq`；**不**推进 sync cursor |
| 投递 ACK（客户端 → 网关） | ❌ | opcode `0x0005` 未发 |
| 已读回执 UI | ❌ | — |
| `MESSAGE_STATUS` / `CONV_UPDATE` | ❌ | handler 忽略 |
| Token 过期中途断连 | ⚠️ | 握手失败不重连；无静默 refresh |

### 4.4 连接生命周期

| 边界 | 状态 | 说明 |
|------|------|------|
| 应用层心跳 | ✅ | — |
| 断线检测弱 | ⚠️ | 心跳 + `NWPathMonitor` |
| 重连退避 + jitter | ✅ | — |
| 建连 single-flight | ✅ | — |
| 后台断开 / 前台重连+sync | ✅ | — |
| Transport / AsyncStream 只消费一次 | ✅ | 工程问题 **17** |
| 静默唤醒预算 | ✅（0048） | ≤25s 取消；`completionHandler` 必调 |

---

## 5. 0044–0048 代码锚点

| 票 | 主题 | 路径 |
|----|------|------|
| 0044 | 消息窗 / 分页 | `Persistence/MessageWindow.swift`、`LocalDatabase+Messaging.swift`（`messageWindow` / `fetchOlderMessages`） |
| 0044 | UI 窗口状态 | `ChatFeature`（`oldestLoadedSeq` / `hasMoreOlder`）、`ChatViews`「加载更早」 |
| 0045 | 去抖投影 | `Persistence/LocalProjectionObserver.swift`；`ChatMiddleware` 绑 observation |
| 0046 | path 续跑 | `Messaging/SendPathResumeMonitor.swift`、`MessageSendExecutor.reclaimStaleSendingAndProcess` |
| 0047 | 缺口补拉 | `Sync/ConversationGapBackfill.swift`；服务端 `GET .../messages` → `latest_seq` |
| 0048 | 静默预算 | `Push/SilentSyncWakeHandler.swift`、`AppDelegate` fetchCompletionHandler |
| — | 组合根 | `ChatApplication/AppServices.swift` |

**排序规则（0044）**：有 `conversation_seq` 升序；`queued`/`sending` 且无 seq → 列表底部（`created_at`）。  
**两层 sync（0047）**：全局 `event_seq` cursor ≠ 会话 `conversation_seq`；缺口只打消息补拉 API。

---

## 6. 主链路代码锚点（0038–0042）

| 主题 | 路径 |
|------|------|
| WS 传输 | `.../URLSessionWebSocketTransport.swift` |
| 会话 / 重连 | `.../Transport/RealtimeSession.swift` |
| 编解码 | `.../Transport/WsCodec.swift` |
| 统一落库 | `.../Messaging/IncomingMessageApplier.swift` |
| Sync | `.../Sync/SyncExecutor.swift` |
| 发送队列 | `.../Messaging/MessageSendExecutor.swift` |
| UI 订阅 | `ChatPresentation/.../ChatMiddleware.swift` |
| 生成脚本 | `ios/scripts/gen_proto.sh` |

---

## 7. Domain 仓储协议落地现状

详见工程问题 **[19](./engineering-problems/19-domain-repository-ports-vs-concrete-executors.md)**（0051 文档降级 → **0052 细粒度 Port 已完成** → 0053 AppServices Fake）。

| Domain 协议 | 角色 | 实际类型 |
|-------------|------|----------|
| `AuthRepository` | ✅ 正式保留 | `AuthRepositoryLive` |
| `WebSocketTransport` | ✅ 正式保留 | `URLSessionWebSocketTransport`；编排 `RealtimeSession` |
| `MessageStore` / `MessageRemote` | ✅ | `LocalDatabase` / `MessageAPI` → `MessageSendExecutor` |
| `SyncCursorStore` / `SyncRemote` | ✅ | `LocalDatabase` / `SyncAPI` → `SyncExecutor` |
| `ConversationStore` / `ConversationRemote` | ✅ | `LocalDatabase` / `ConversationAPI` |
| `MediaRepository` | ❌ 未做 | 留给 [0049](../issues/0049-ios-image-message.md) |

**禁止**空壳 `MessageRepositoryLive`。粗 `MessageRepository` / `SyncRepository` / `ConversationRepository` / `WebSocketRepository` / `PushRepository` **已删除**，不再是待实现清单。

DTO（`SendMessageRequest` / `SyncEvent` / `AuthTokens` 等）**正在使用**。Fake 测见 `PortFakesTests`；`AppServices` 组合根 Port 化留给 0053。

---

## 8. 工程问题交叉引用

| # | 文档 |
|---|------|
| 16 | [WebSocket 帧边界](./engineering-problems/16-websocket-framing-vs-tcp-sticky-packets.md) |
| 17 | [WebSocketTask / AsyncStream 生命周期](./engineering-problems/17-urlsession-websocket-lifecycle-asyncstream.md) |
| 18 | [Swift Protobuf 生成入库](./engineering-problems/18-swift-protobuf-codegen-workflow.md) |
| 19 | [Domain 仓储协议与落地实现漂移](./engineering-problems/19-domain-repository-ports-vs-concrete-executors.md) |
