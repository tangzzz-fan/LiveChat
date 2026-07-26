# iOS App 测试方法（截至 0039）

面向本机联调：已完成 OTP 登录（0038）与本地优先发文本 / 1:1 会话（0039）。  
同步 / WebSocket 实时（0040+）尚未落地，**对端不会自动出现对方消息**，除非后续走 sync。

相关：[`API参考.md`](./API参考.md) · [`ios/README.md`](../ios/README.md) · [`ios-client-rewrite.md`](./ios-client-rewrite.md)

---

## 1. 前置条件

| 项 | 要求 |
|----|------|
| PostgreSQL | 本机 `localhost:5432`，已 `make migrate-up` |
| Redis | 本机 `localhost:6379` |
| message-service | 默认 `http://127.0.0.1:8080`（App `APIConfig` 同此） |
| gateway | 0039 可不启；WS 留给 0041 |
| 模拟器 | 推荐 **iPhone 17 Pro** + **iPhone 17 Pro Max**（iOS 26.5） |
| Bundle ID | `com.tango.LiveChat` |

服务端启动（仓库根或 `livechat-server/`，以 Makefile 为准）：

```bash
cd livechat-server
make migrate-up
make run-message-service   # :8080
# 可选：make run-gateway / make run-outbox-consumer
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

无效示例（会 400 `invalid phone_e164 format`）：

- `15551234567`（无 `+`）
- `+123`（过短）
- `+`、`not-a-phone`

### 2.2 验证码（本地 mock）

- Redis 侧 mock 码通常为 **`123456`**
- 本地开发下 verify 对任意 **6 位数字** 也可能接受（以当前服务端实现为准）
- UI 提示文案：`本地 mock 验证码通常为 123456`

### 2.3 频控（踩坑）

| 维度 | 限制 | 表现 |
|------|------|------|
| 同手机号 | ≤ 约 3 次/小时 request_code | `429` |
| 同 IP | ≤ 约 20 次/小时 request_code | `429` |

联调时：**不要**每次换号狂刷 request_code；双机用固定两号即可。撞频控后换一个未用过的 E.164，或等窗口过期 / 清 Redis 相关 key（仅本机开发）。

---

## 3. 设备与登录态

| 项 | 行为 |
|----|------|
| `device_id` | 首次启动写入 Keychain，形如 `ios-<uuid>`；**退出登录不清除** |
| 多模拟器 | 每台模拟器各自 Keychain → 天然不同 `device_id` |
| 同号多设备 | 同一 `phone_e164`、不同 `device_id` 可同时在线 |
| 冷启动 | 有 token 则自动进入已登录页（bootstrap） |
| 退出 | 清 token / user_id，保留 device_id |

登录成功后首页会显示：

- `user_id`（整数，给对方填「打开 1:1」用）
- `device_id`
- 「刷新设备」→ `GET /v1/devices`（当前账号下设备列表；当前设备带 `*`）

---

## 4. 推荐联调剧本（双模拟器）

### 4.1 编译与安装

```bash
xcodebuild -project ios/LiveChat/LiveChat.xcodeproj -scheme LiveChat \
  -destination 'platform=iOS Simulator,name=iPhone 17 Pro,OS=26.5' build

xcrun simctl boot "iPhone 17 Pro"
xcrun simctl boot "iPhone 17 Pro Max"
APP=$(find ~/Library/Developer/Xcode/DerivedData/LiveChat-*/Build/Products/Debug-iphonesimulator \
  -maxdepth 1 -name LiveChat.app | head -1)
xcrun simctl install "iPhone 17 Pro" "$APP"
xcrun simctl install "iPhone 17 Pro Max" "$APP"
xcrun simctl launch "iPhone 17 Pro" com.tango.LiveChat
xcrun simctl launch "iPhone 17 Pro Max" com.tango.LiveChat
```

或在 Xcode 里分别选两个模拟器 Run。

### 4.2 登录

1. **Pro**：手机号 `+8613800000001` → 获取验证码 → `123456` → 登录  
2. **Pro Max**：手机号 `+8613800000002` → 同上  
3. 两边各自记下首页 **`user_id`**（例如 A=`12`，B=`13`）

### 4.3 建 1:1 并发送（0039）

1. 在 A 的「打开 1:1」填 **B 的 user_id**（纯数字，不要加 `+`）  
2. 点「创建/打开会话」→ 进入聊天页  
3. 输入文本发送；气泡下 status 应变为 `queued` → `sending` → **`accepted`**  
4. （可选）B 用 A 的 `user_id` 同样打开会话；**此时 B 本地未必有 A 刚发的消息**（无 sync/WS）

自己填自己的 `user_id` → 服务端 `400`（cannot open with yourself）。  
填不存在的 id → `404`。

### 4.4 发送队列 / 429（可选）

- 断网或停 message-service 再发：本地应留下 `queued`/`sending`/`failed`  
- 恢复后点「重试队列」或再触发发送路径，pending 可续跑  
- 服务端 outbox 背压时 send 可能 `429`：客户端按 `Retry-After` 退避，**同一 `client_message_id` 重试**，状态保持 sending（不立即 failed）

---

## 5. 包级自动化测试（无模拟器）

```bash
cd ios/Packages/ChatDomain && swift test
cd ../ChatInfrastructure && swift test
cd ../ChatApplication && swift test
cd ../ChatPresentation && swift test
```

覆盖：消息状态机、GRDB 迁移、AppServices 组装、Auth/Chat reducer 组合（含 logout 清空 chat）。

---

## 6. 常见失败速查

| 现象 | 可能原因 |
|------|----------|
| 获取验证码立刻红字 / 400 | 手机号不是 E.164（缺 `+` 或位数不对） |
| 429 获取验证码 | 触发手机号或 IP 频控 |
| 登录失败 Connection refused | message-service 未起或不是 `:8080` |
| 打开会话失败 | `peer_user_id` 非数字、是自己、或对端用户未注册 |
| 发送一直 failed | 服务端挂了 / 非会话成员；看错误文案 |
| 对端看不到消息 | **预期**：0040 sync / 0041 WS 未做 |
| 重装后仍登录 | Keychain 未清；点「退出」或删 App 再装 |

清模拟器 App 数据：长按图标删除 App 后重装（会新 `device_id`）。

---

## 7. 能力边界（当前）

| 已有 | 未有 |
|------|------|
| OTP 登录 + Keychain | 增量 sync（0040） |
| 1:1 direct + 本地优先发文本 | WebSocket 实时（0041） |
| 本端 GRDB + 发送队列 | Push 静默唤醒（0042） |
| Feature 分拆的 TGReduxKit | 图片消息 |

文档随 0040+ 补「对端可见」步骤。
