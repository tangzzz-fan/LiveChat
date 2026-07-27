# iOS App 测试方法（截至 0040）

面向本机联调：已完成 OTP 登录（0038）、本地优先发文本 / 1:1（0039）、增量 sync（0040）。  
WebSocket 实时（0041）尚未落地；对端可 **手动同步 / 回前台 / 冷启动** 拉到消息，但不会实时推送。

相关：[`API参考.md`](./API参考.md) · [`ios/README.md`](../ios/README.md) · [`ios-client-rewrite.md`](./ios-client-rewrite.md)

---

## 1. 前置条件

| 项 | 要求 |
|----|------|
| PostgreSQL | 本机 `localhost:5432`，已 `make migrate-up` |
| Redis | 本机 `localhost:6379` |
| message-service | 默认 `http://127.0.0.1:8080`（App `APIConfig` 同此） |
| **outbox-consumer** | **0040 必开**：否则 `message_created` 不会写入对端 `sync_events` |
| gateway | 0040 可不启；WS 留给 0041 |
| 模拟器 | 推荐 **iPhone 17 Pro** + **iPhone 17 Pro Max**（iOS 26.5） |
| Bundle ID | `com.tango.LiveChat` |

服务端启动：

```bash
cd livechat-server
make migrate-up
make run-message-service    # :8080
make run-outbox-consumer    # fanout → sync_events（0040 关键）
# 可选：make run-gateway
```

确认健康：

```bash
curl -s -X POST http://127.0.0.1:8080/v1/auth/request_code \
  -H 'Content-Type: application/json' \
  -d '{"phone_e164":"+8613800009999"}'
# 期望 200：{"retry_after_sec":...,"expires_in_sec":300}
```

ATS：App 已开 `NSAllowsLocalNetworking`，模拟器访问本机 HTTP 无需额外配置。

---

## 2. 手机号与验证码

### 2.1 格式（E.164）

服务端正则：`^\+[1-9]\d{6,14}$`

| 规则 | 说明 |
|------|------|
| 必须以 `+` 开头 | `13800138000` ❌ |
| `+` 后第一位不能是 `0` | `+086...` ❌ |
| 数字总长（不含 `+`） | 7–15 位 |

**推荐测试号（中国示例）**：

| 用途 | 手机号 |
|------|--------|
| 模拟器 A | `+8613800000001` |
| 模拟器 B | `+8613800000002` |
| 临时加号 | `+86138` + 自选 8 位，保证全局唯一更稳 |

### 2.2 验证码（本地 mock）

- Redis 侧 mock 码通常为 **`123456`**
- UI 提示文案：`本地 mock 验证码通常为 123456`

### 2.3 频控（踩坑）

| 维度 | 限制 | 表现 |
|------|------|------|
| 同手机号 | ≤ 约 3 次/小时 request_code | `429` |
| 同 IP | ≤ 约 20 次/小时 request_code | `429` |

---

## 3. 设备与登录态

| 项 | 行为 |
|----|------|
| `device_id` | 首次启动写入 Keychain，形如 `ios-<uuid>`；**退出登录不清除** |
| 多模拟器 | 每台模拟器各自 Keychain → 天然不同 `device_id` |
| 冷启动 | 有 token 则自动登录，并触发一次 sync |
| 回前台 | `scenePhase == .active` → sync |
| 退出 | 清 token / user_id，保留 device_id |

---

## 4. 推荐联调剧本（双模拟器）

### 4.1 编译与安装

```bash
xcodebuild -project ios/LiveChat/LiveChat.xcodeproj -scheme LiveChat \
  -destination 'platform=iOS Simulator,name=iPhone 17 Pro,OS=26.5' build
```

或在 Xcode 里分别选两个模拟器 Run。

### 4.2 登录

1. **Pro**：手机号 `+8613800000001` → 获取验证码 → `123456` → 登录  
2. **Pro Max**：手机号 `+8613800000002` → 同上  
3. 两边各自记下首页 **`user_id`**

### 4.3 建 1:1 并发送（0039）

1. 在 A 的「打开 1:1」填 **B 的 user_id**  
2. 点「创建/打开会话」→ 输入文本发送 → status 到 **`accepted`**

### 4.4 对端增量同步（0040）

1. 确认 **outbox-consumer** 在跑（A 发送后几秒内 fanout 写完 sync_events）  
2. 在 **B** 首页点 **「手动同步」**（或杀进程重开 / 切后台再回前台）  
3. 期望：
   - 同步区出现 `已同步 +N · cursor …`
   - 会话列表出现与 A 的会话，preview 为刚发的文本  
4. 打开该会话：气泡正文可见，副标题为 **`server_message_id`**（形如 `msg_conv_…_000001`）

若 B 同步 `+0` 且仍无消息：先等 1–2 秒再点同步；仍无则检查 outbox-consumer 日志是否消费了 `message_created`。

---

## 5. 包级自动化测试（无模拟器）

```bash
cd ios/Packages/ChatDomain && swift test
cd ../ChatInfrastructure && swift test
cd ../ChatApplication && swift test
cd ../ChatPresentation && swift test
```

覆盖：消息状态机、GRDB 迁移 / sync 游标单调推进 / 入站消息幂等、AppServices 组装、Auth/Chat reducer（含 sync banner）。

---

## 6. 常见失败速查

| 现象 | 可能原因 |
|------|----------|
| 获取验证码立刻红字 / 400 | 手机号不是 E.164（缺 `+` 或位数不对） |
| 429 获取验证码 | 触发手机号或 IP 频控 |
| 登录失败 Connection refused | message-service 未起或不是 `:8080` |
| 打开会话失败 | `peer_user_id` 非数字、是自己、或对端用户未注册 |
| 发送一直 failed | 服务端挂了 / 非会话成员；看错误文案 |
| B 同步 +0、无消息 | **outbox-consumer 未启** / fanout 未完成 / 未登录 |
| 同步卡住或红字 | token 失效；退出重登 |
| 重装后仍登录 | Keychain 未清；点「退出」或删 App 再装 |

---

## 7. 能力边界（当前）

| 已有 | 未有 |
|------|------|
| OTP 登录 + Keychain | WebSocket 实时（0041） |
| 1:1 direct + 本地优先发文本 | Push 静默唤醒（0042） |
| 增量 sync（启动 / 回前台 / 手动） | 图片消息 |
| Feature 分拆的 TGReduxKit | |

文档随 0041 补「实时推送」步骤。
