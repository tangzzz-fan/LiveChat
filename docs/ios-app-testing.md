# iOS App 测试方法（截至 0041）

面向本机联调：OTP 登录（0038）、本地优先发文本 / 1:1（0039）、增量 sync（0040）、**WebSocket 实时（0041）**。

相关：[`API参考.md`](./API参考.md) · [`ios/README.md`](../ios/README.md) · [`ios-client-rewrite.md`](./ios-client-rewrite.md)

---

## 1. 前置条件

| 项 | 要求 |
|----|------|
| PostgreSQL | 本机 `localhost:5432`，已 `make migrate-up` |
| Redis | 本机 `localhost:6379` |
| message-service | `http://127.0.0.1:8080` |
| **outbox-consumer** | 必开（fanout → sync_events + 在线投递） |
| **gateway** | **0041 必开**：`ws://127.0.0.1:8081/ws` |
| 模拟器 | iPhone 17 Pro + Pro Max |
| Bundle ID | `com.tango.LiveChat` |

```bash
cd livechat-server
make migrate-up
make run-message-service    # :8080
make run-outbox-consumer
make run-gateway            # :8081 WebSocket
```

Proto 生成（变更 `ws_frame.proto` 后）：

```bash
# brew install protobuf swift-protobuf
./ios/scripts/gen_proto.sh
# 输出：ios/Packages/ChatInfrastructure/Sources/ChatInfrastructure/Generated/ws_frame.pb.swift
```

---

## 2. 手机号与验证码

推荐：`+8613800000001` / `+8613800000002`，验证码 `123456`。须 E.164（以 `+` 开头）。

---

## 3. 推荐联调剧本

### 3.1 登录

双模拟器分别登录上述两号，记下各自 `user_id`。首页应出现 `WS 已连接 · …`。

### 3.2 实时投递（0041）

1. A 打开与 B 的 1:1，发送文本 → `accepted`  
2. **B 保持前台、不点手动同步**  
3. 期望：B 会话列表 / 聊天页很快出现消息，气泡副标题为 `server_message_id`  
4. 进后台：B 显示 `WS 已断开（后台）`；回前台：重连 + 立刻 sync

### 3.3 增量同步兜底（0040）

离线期间漏收的消息：回前台或「手动同步」仍可补齐。

---

## 4. 包级测试

```bash
cd ios/Packages/ChatDomain && swift test
cd ../ChatInfrastructure && swift test
cd ../ChatApplication && swift test
cd ../ChatPresentation && swift test
```

---

## 5. 常见失败速查

| 现象 | 可能原因 |
|------|----------|
| WS 一直连接中 / 握手失败 | gateway 未启；token 失效 |
| B 收不到实时消息 | outbox-consumer 未启；B 在后台已断开 |
| 同步 +0 无消息 | fanout 未写 sync_events |
| 登录 Connection refused | message-service 未起 |

---

## 6. 能力边界（当前）

| 已有 | 未有 |
|------|------|
| OTP + 本地优先发送 | Push 静默唤醒（0042） |
| 增量 sync | 图片消息 |
| 前台 WS 实时 + 后台断开 | |
