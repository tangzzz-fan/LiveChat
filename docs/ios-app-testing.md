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

## 7. 横切验收（0050）

结论与步骤见 [ios-high-load-client.md](./ios-high-load-client.md)「建议的横切验收项」。

---

## 8. 已读回执联调（0054）

1. A 发消息给 B（B 不在该会话）→ B 列表未读应累加  
2. B 点进会话 → 未读角标清零；若 WS 已连接会发 `ACK(read)`  
3. A 侧手动同步或等 sync → 己方气泡应变为已读标记（✓✓ 高亮）  
4. 服务端需 gateway + outbox-consumer 消费 `read_receipt`

---

## 9. 消息长按菜单（0056）

1. 打开任意会话，**长按**一条文本气泡 → 弹出菜单：复制 / 删除（失败消息另有「重试」）  
2. **复制**：粘贴到备忘录等，内容应为气泡纯文本（非 `{"text":...}`）  
3. **删除**：仅本机消失；对端仍可见（非服务端撤回）  
4. 将本机消息置为失败后长按 → **重试** 应重新入队发送  

图片气泡：P1 不提供复制（菜单仍有删除）。

---

## 10. 会话列表排序（0061）

1. 与两个 peer 各有会话；向 A 发一条更新消息  
2. 回到列表：含最新消息的会话应在最上  
3. 再打开较旧会话（清未读）→ 返回列表后，**仍以最新消息时间排序**，不因刚打开而置顶  

---

## 11. 聊天键盘 UX（0062）

1. 打开会话，点输入框唤起键盘 → 最新气泡应仍可见（列表上滑）  
2. 点消息列表 / 气泡 → 键盘收起、输入框失焦  
3. 上滑列表可交互收起键盘（`scrollDismissesKeyboard`）

---

## 11b. 进会话钉底（0063）

1. 选一个消息较多的会话，反复：进会话 → 返回列表 → 再进入  
2. 每次进入应稳定停在**最新消息**（不应停在顶部或中部再「掉下去」）

---

## 11c. 图片行抖动与加载更早（0064）

1. 打开含图片的会话：进页应无明显上下抖（气泡按 width/height 预留固定框）  
2. 顶部点「加载更早消息」：从**本地 GRDB** 再取一页更早 `conversation_seq`；视口应留在原阅读位置附近，不跳到最新  
3. 若本地没有更早消息，`hasMoreOlder` 变 false，按钮消失（远端历史靠 sync/gapBackfill 先写入本地，不是这个按钮直拉）

---

## 12. 文本输入内容处理（速查）

| 阶段 | 存什么 | 在哪 |
|------|--------|------|
| 键入 | 纯字符串 | `ChatState.composeDraft`（Redux，不落库） |
| 发送瞬间 | `{"text":"..."}` | `Message.content` → GRDB + HTTP |
| 气泡 / 列表预览 | 解析后的纯文本 | `TextMessageContent.parseText` / summary.preview |

代码锚点：`ChatViews` TextField → `ChatMiddleware.sendTapped` → `TextMessageContent` → `MessageSendExecutor`。
