# iOS 客户端复习导读（0035–0041）

面向「做完功能后回头复习」：把各票涉及的技术映射到已有文档，并标明**缺口**与**补齐位置**。

相关决策：[ios-client-rewrite.md](./ios-client-rewrite.md) · 联调：[ios-app-testing.md](./ios-app-testing.md) · Spec：[specs/13-iOS客户端架构设计.md](../specs/13-iOS客户端架构设计.md)

---

## 1. 结论先说

| 问题 | 结论 |
|------|------|
| 票里的技术是否都有对应文档？ | **大框架有**（架构决策、高负载、联调），但 **0040/0041 落地踩坑**（WS 生命周期、`AsyncStream` 单消费者、proto 生成、收发边界）此前**不足**。 |
| 粘包？ | **走 `URLSessionWebSocketTask` 时，应用层几乎不会撞上经典 TCP 粘包**；真正要防的是「一帧多消息 / 半帧解析 / 双通道重复」等边界。详见工程问题 16。 |
| 本轮补齐 | 本文 + 工程问题 **16 / 17 / 18**。 |

---

## 2. 票 → 文档对照

| 票 | 主题 | 已有文档 | 本轮补齐 / 仍缺口 |
|----|------|----------|------------------|
| 0035 | 父票 / 重写范围 | [ios-client-rewrite.md](./ios-client-rewrite.md)、[0035](../issues/0035-ios-client-rewrite.md) | 父票子表状态需与 INDEX 同步（0040/0041 已 complete） |
| 0036 | Spec 修订 + 决策 | Spec 13、ios-client-rewrite | — |
| 0037 | SPM 脚手架 | [ios/README.md](../ios/README.md)、gen_proto 说明（rewrite §2） | **18**：proto 生成流程与版本坑 |
| 0038 | OTP + Keychain | [09](./engineering-problems/09-two-step-auth-code-storage.md)、[08](./engineering-problems/08-session-version-device-revocation.md)、ios-app-testing | — |
| 0039 | 本地优先发送 | [06](./engineering-problems/06-message-lifecycle-stages.md)、[01](./engineering-problems/01-message-durability-outbox.md)、[02](./engineering-problems/02-message-ordering-sequence.md) | 发送队列/429 已在 testing；深度见本文 §4 |
| 0040 | 增量 sync | [05](./engineering-problems/05-offline-gap-detection.md)、ios-high-load「sync 洪流」 | cursor 单调 + 与 WS 共用落库见本文 §4 |
| 0041 | WS 实时 | [03](./engineering-problems/03-reconnection-storm.md)、ios-client-rewrite §4–5、API参考 §6 | **16** 帧边界；**17** Task/`AsyncStream` 生命周期 |
| 0042 | Push（已完成） | [11](./engineering-problems/11-push-deduplication-coalescing.md)、ios-app-testing §2.3 | mock token + 本地静默注入；真 APNs 证书不在本仓范围 |

过时注意：[`iOS多端接入评估与实现.md`](./iOS多端接入评估与实现.md) 仍写「iOS 未接 WS」，以 **0038–0041 实现 + 本文** 为准。

---

## 3. 推荐复习顺序（约 1–2 小时）

1. **边界与真相源**：ios-client-rewrite §3（GRDB vs Store）  
2. **发送语义**：工程问题 06（Accepted ≠ Delivered）→ 看 `MessageSendExecutor`  
3. **离线补齐**：工程问题 05 → `SyncExecutor`  
4. **实时通道**：工程问题 **16 → 17** → `RealtimeSession` + `URLSessionWebSocketTransport`  
5. **协议工具链**：工程问题 **18** → `ios/scripts/gen_proto.sh`  
6. **横切**：ios-high-load-client（突发投递、重连、写风暴）  
7. **动手**：ios-app-testing 双模拟器剧本  

---

## 4. 收发消息边界条件清单（覆盖度）

图例：✅ 已处理 · ⚠️ 部分/有意识但未完整 · ❌ 未做（留给后续票）

### 4.1 传输层帧边界

| 边界 | 状态 | 说明 |
|------|------|------|
| TCP 粘包 / 半包 | ✅（下沉到 WS 栈） | `receive()` 交付**完整** WebSocket message，不是裸 TCP 字节流 |
| 一发一收语义 | ✅ | 协议约定：**1 个 WS binary message = 1 个 `WsFrame` protobuf**（无应用层长度前缀） |
| 一帧塞多条业务消息 | ✅ 约定禁止 | 若违规，`WsCodec.decodeFrame` 只会解析出一个 envelope；不要在 payload 里再拼粘 |
| WS 分片帧 | ✅（系统重组） | Apple API：读到的是 reassembled message |
| `maximumMessageSize` | ⚠️ | 超大帧会 receive 失败；当前文本消息远低于默认上限，未单独配置 |
| 网关 `ReadFrame` 的 4 字节长度前缀 | ✅ 不混用 | 仅非 WS 的 `io.Reader` 路径；iOS/压测 WS **不要**加长度前缀（见 load_test `ws_protocol.py` 注释） |

### 4.2 发送路径

| 边界 | 状态 | 说明 |
|------|------|------|
| 弱网先落本地 | ✅ | `queued` → `sending` → `accepted` / `failed` |
| `client_message_id` 幂等 | ✅ | 服务端冲突返回 duplicate；本地不重复插 |
| 有界发送队列 | ✅ | 满则 `SendQueueError.full` |
| HTTP `429` + Retry-After | ✅ | 退避抖动，保持 sending |
| 自己发的消息不靠 sync 回环 | ✅ | fanout 排除 sender；本端靠本地行 + HTTP 回写 server id |
| 乱序到达服务端 | ⚠️ | 服务端 `conversation_seq` 单写点保证；客户端展示按本地 `created_at`，未做严格 seq 排序 UI |

### 4.3 接收路径（WS + Sync）

| 边界 | 状态 | 说明 |
|------|------|------|
| WS / Sync 双通道重复 | ✅ | 统一 `IncomingMessageApplier` / `server_message_id` 幂等 |
| 突发投递打爆 UI | ✅ | ~24ms 批量落库 + UI 侧 ~16ms 再合并 |
| sync cursor 只前进 | ✅ | 单事件 apply 成功后才推进 |
| 会话 seq 缺口检测 | ⚠️/❌ | 服务端能力在 Spec 06；**iOS 尚未做缺口探测 + 按会话补拉** |
| 投递 ACK（客户端 → 网关） | ❌ | opcode `0x0005` 未发 |
| 已读回执 UI | ❌ | 服务端有；客户端未接 |
| `MESSAGE_STATUS` / `CONV_UPDATE` | ❌ | 帧类型已生成，handler 忽略 |
| Token 过期中途断连 | ⚠️ | 握手失败 `should_reconnect=false`；需重新登录，无静默 refresh |

### 4.4 连接生命周期

| 边界 | 状态 | 说明 |
|------|------|------|
| 应用层心跳 | ✅ | 按握手协商间隔 |
| 断线检测弱 | ⚠️ | 心跳 + `NWPathMonitor` 兜底；无独立「超时未收到 ACK 则踢」硬逻辑 |
| 重连退避 + jitter | ✅ | 对齐 gateway `reconnect.go` |
| 建连 single-flight | ✅ | `RealtimeSession.connectTask` |
| 后台断开 / 前台重连+sync | ✅ | `scenePhase` |
| **Transport / AsyncStream 只消费一次** | ✅（踩过坑） | 见工程问题 **17**：每次重连 **new** transport；events 订阅只绑一次 |

---

## 5. 代码锚点（复习时打开）

| 主题 | 路径 |
|------|------|
| WS 传输 | `ios/Packages/ChatInfrastructure/.../URLSessionWebSocketTransport.swift` |
| 会话 / 重连 | `.../Transport/RealtimeSession.swift` |
| 编解码 | `.../Transport/WsCodec.swift` |
| 生成物 | `.../Generated/ws_frame.pb.swift` |
| 统一落库 | `.../Messaging/IncomingMessageApplier.swift` |
| Sync | `.../Sync/SyncExecutor.swift` |
| 发送队列 | `.../Messaging/MessageSendExecutor.swift` |
| UI 订阅 | `ChatPresentation/.../ChatMiddleware.swift` |
| 生成脚本 | `ios/scripts/gen_proto.sh` |

---

## 6. 工程问题交叉引用（本轮新增）

| # | 文档 |
|---|------|
| 16 | [WebSocket 帧边界：为什么不是 TCP 粘包，却仍会踩边界](./engineering-problems/16-websocket-framing-vs-tcp-sticky-packets.md) |
| 17 | [原生 WebSocketTask 生命周期与 AsyncStream 单消费者陷阱](./engineering-problems/17-urlsession-websocket-lifecycle-asyncstream.md) |
| 18 | [Swift Protobuf 生成入库与版本对齐](./engineering-problems/18-swift-protobuf-codegen-workflow.md) |
