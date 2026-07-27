# iOS App 测试方法（截至 0042）

面向本机联调：OTP（0038）、发送（0039）、sync（0040）、WS 实时（0041）、**Push Token + 静默唤醒 sync（0042）**。

相关：
- 联调操作：本文  
- **0041 测试如何设计**：[ios-0041-realtime-test-design.md](./ios-0041-realtime-test-design.md)  
- 复习 / 边界：[ios-client-study-guide.md](./ios-client-study-guide.md)  
- API：[API参考.md](./API参考.md)

---

## 1. 前置条件

| 项 | 要求 |
|----|------|
| PostgreSQL / Redis | 本机已起，已 `make migrate-up` |
| message-service | `:8080` |
| outbox-consumer | 必开 |
| gateway | 0041/前台实时必开；**纯静默 sync 演示可不启** |
| 模拟器 | iPhone 17 Pro + Pro Max |

```bash
cd livechat-server
make migrate-up
make run-message-service
make run-outbox-consumer
make run-gateway   # 前台实时需要
```

---

## 2. 推荐联调剧本

### 2.1 登录 + Push Token（0042）

双机登录后，首页「推送 / 静默唤醒」应出现 `push_token 已注册 · sim-mock-…`（登录自动注册 mock）。  
也可再点「注册 Push Token（mock）」。

服务端设备行应有 `push_token`（可用 DB 或后续扩展设备列表展示）。

### 2.2 实时（0041）— 详见设计文档

主信号：B **不点同步** 也能看到 A 的消息。步骤与反例树见 [ios-0041-realtime-test-design.md](./ios-0041-realtime-test-design.md)。

### 2.3 静默唤醒 → sync（0042）

模拟器无真实 APNs，用 **本地注入** 演示同一路径：

1. B 进后台（WS 断开）  
2. A 发送一条新消息（确保 outbox-consumer 跑完）  
3. B 回前台**之前**，或保持后台逻辑上：在 B 点 **「模拟静默唤醒 → sync」**  
4. 期望：`已同步 +N · cursor …`，会话/气泡出现 `server_message_id`  
5. **不应**依赖此时 WS 已连接（横幅可为断开）

真机可选：系统 Silent Push → `AppDelegate.didReceiveRemoteNotification` → 同一 `SilentSyncWakeHandler`。

---

## 3. 包级测试

```bash
cd ios/Packages/ChatDomain && swift test
cd ../ChatInfrastructure && swift test
cd ../ChatApplication && swift test
cd ../ChatPresentation && swift test
```

---

## 4. 常见失败速查

| 现象 | 可能原因 |
|------|----------|
| 无 push_token 横幅 | 未登录 / POST push-token 失败 |
| 模拟静默 +0 | outbox 未消费；或消息已 sync 过 |
| 期望后台靠 WS 收消息 | 与 Spec 13 不符；应靠 sync/Push |

---

## 5. 能力边界

| 已有 | 未有 |
|------|------|
| 0038–0042 主链路 | 生产 APNs 证书 / 真推送联调 |
| 会话列表 UI（标题/预览/时间/未读） | 生产 APNs 证书 / 真推送联调 |
| 图片消息（0049：选图→上传→气泡缩略） | 视频 / 原图浏览 |
| 已读回执（0054：进会话清未读 + ACK(read) + ✓✓） | delivered 独立 ACK（服务端暂未接） |
| mock push token + 本地静默注入 | — |
| 前台 WS + 后台 sync + 缺口补拉 | — |

---

## 6. 图片消息联调（0049）

1. 双端登录，打开同一 1:1  
2. A 在会话点相册图标选图 → 发送  
3. A 气泡应出现缩略图（本地缓存，不卡列表）  
4. B 经 WS 或 sync 后可见 `[图片]` 气泡并加载缩略（download/auth；缩略解码）  
5. 快速滚动离屏：加载 Task 应取消（Instruments 可选）

服务端需 message-service（含媒体本地 store + thumbnail worker）。

---

## 9. 文本输入内容处理（速查）

| 阶段 | 存什么 | 在哪 |
|------|--------|------|
| 键入 | 纯字符串 | `ChatState.composeDraft`（Redux，不落库） |
| 发送瞬间 | `{"text":"..."}` | `Message.content` → GRDB + HTTP |
| 气泡 / 列表预览 | 解析后的纯文本 | `TextMessageContent.parseText` / summary.preview |

代码锚点：`ChatViews` TextField → `ChatMiddleware.sendTapped` → `TextMessageContent` → `MessageSendExecutor`。
