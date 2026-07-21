# LiveChat API 参考

面向客户端（含 iOS）的接口清单。实现以 `livechat-server/internal/api/router.go` 与 Gateway 为准；规格见 `Specs/03`–`09`、`05`。

**Base URL（本机）**

| 服务 | 地址 |
|------|------|
| HTTP（message-service） | `http://localhost:8080` |
| WebSocket（gateway） | `ws://localhost:8081/ws` |

**通用约定**

- JSON：`Content-Type: application/json`
- 鉴权：`Authorization: Bearer <access_token>`（标注「需 JWT」的接口）
- 错误体：`{"error":"<message>"}`；设备吊销时可能含 `code: "device_revoked"`
- JWT Claims：`user_id`、`device_id`、`sv`（session_version）
- 追踪（可选）：请求头 `X-Trace-Id`；未传则服务端生成

---

## 1. 认证

### 1.1 推荐：两步 OTP（Phase 2）

#### `POST /v1/auth/request_code` — 无需 JWT

```json
{ "phone_e164": "+8613800000001" }
```

成功 `200`：

```json
{ "retry_after_sec": 30, "expires_in_sec": 300 }
```

- Mock：验证码固定写入 Redis 为 `123456`（本地任意 6 位也可在 verify 阶段被接受，以当前实现为准）
- 频控：同手机号 ≤3 次/小时；同 IP ≤20 次/小时 → `429`

#### `POST /v1/auth/verify_code` — 无需 JWT

```json
{
  "phone_e164": "+8613800000001",
  "verification_code": "123456",
  "device_id": "ios-iphone-A",
  "platform": "ios"
}
```

成功 `200`：

```json
{
  "access_token": "eyJ...",
  "refresh_token": "...",
  "expires_in": 3600,
  "user_id": 1
}
```

- 首次手机号自动建用户；同 `device_id` 再登会推进 `session_version`
- **多 iOS 设备**：同一 `user_id`、不同 `device_id` 各拿一份 token，即可多端同时在线

#### `POST /v1/auth/refresh` — 无需 JWT（用 refresh_token）

```json
{ "refresh_token": "..." }
```

成功返回新的 `access_token` / `refresh_token`（旋转语义以当前实现为准）。

### 1.2 兼容：单步 register / login（Phase 1，仍可用，建议新客户端不用）

- `POST /v1/auth/register`
- `POST /v1/auth/login`

请求字段与 `verify_code` 类似（`phone_e164`、`verification_code`、`device_id`、`platform`）。

---

## 2. 设备

均需 JWT。

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/v1/devices` | 当前用户设备列表（含 `session_version`） |
| `POST` | `/v1/devices/{did}/revoke` | 吊销设备（递增 sv，旧 JWT 立即失效） |
| `POST` | `/v1/devices/push-token` | 注册/更新 APNs token |

#### `POST /v1/devices/push-token`

```json
{
  "device_id": "ios-iphone-A",
  "push_token": "<apns device token hex/base64>",
  "environment": "sandbox"
}
```

---

## 3. 消息与会话

均需 JWT。

### 3.1 `POST /v1/messages/send`

```json
{
  "client_message_id": "ios-A-uuid-1",
  "conversation_id": "conv_grp_xxx",
  "message_type": "text",
  "content": "{\"text\":\"hello\"}"
}
```

成功 `200`（字段以 `messages.Send` 返回为准，通常含）：

```json
{
  "server_message_id": "...",
  "conversation_seq": 12,
  "is_duplicate": false,
  "server_received_at_ms": 1710000000000
}
```

| 约束 | 说明 |
|------|------|
| 幂等 | `(sender_user_id, client_message_id)` 唯一；重试返回已有消息且 `is_duplicate=true` |
| 成员 | 非会话成员 → `403` |
| `message_type=image` | `content` 须为 JSON，且含 `attachment.object_key` / `mime_type` / `size_bytes` |

> **缺口（客户端需注意）**：当前 **没有** `POST /v1/conversations` 创建 1:1 私聊。可用：
> 1. 建群 `POST /v1/groups`（返回 `conversation_id`）作为多人/双人会话；或  
> 2. 开发环境手工插入 `conversations` + `conversation_members`（见测试辅助）。

### 3.2 `GET /v1/conversations`

Query：`limit`（默认 50）、`offset`。

返回会话摘要列表（投影表 `conversation_summaries`）。

### 3.3 `GET /v1/conversations/{cid}/messages`

Query：`from_seq`（默认 0）、`limit`（默认 50，最大 100）。

按 `conversation_seq` 升序补拉历史；非成员 → `403`。

### 3.4 `GET /v1/sync/events`

Query：`cursor`（上次已消费的 `event_seq`，默认 0）、`limit`（默认 100，最大 200）。

```json
{
  "events": [ /* SyncEvent */ ],
  "has_more": false,
  "latest_event_seq": 42,
  "server_time_ms": 1710000000000
}
```

成功拉取后服务端会更新该 **device** 的 sync cursor。多设备各自维护游标（Spec 06 / Spec 13）。

---

## 4. 群组

均需 JWT。

| 方法 | 路径 | Body / 说明 |
|------|------|-------------|
| `POST` | `/v1/groups` | `{ "name", "description?" }` → `201` `{ group, conversation_id }` |
| `POST` | `/v1/groups/{gid}/members` | `{ "user_ids": [2,3] }` |
| `DELETE` | `/v1/groups/{gid}/members/{uid}` | 管理员踢人 |
| `POST` | `/v1/groups/{gid}/leave` | 自己退群 |
| `GET` | `/v1/groups/{gid}/members` | 成员列表 |

群会话 ID 形如：`conv_grp_<group_id>`（以 `group.Service.GetConversationID` 为准）。

---

## 5. 媒体

需 JWT（分片上传/下载 URL 为预签名，路径上自带 `exp`/`sig`）。

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/v1/media/upload/initiate` | 申请 upload_id、object_key、presigned_urls |
| `GET` | `/v1/media/upload/{uploadID}/status` | 分片完成情况 |
| `POST` | `/v1/media/upload/{uploadID}/complete` | `{ object_key, parts[] }` |
| `POST` | `/v1/media/download/auth` | `{ object_key, conversation_id }` → 短期下载 URL |
| `PUT` | `/media/upload-part/{uploadID}/{partNumber}?exp=&sig=` | 上传分片（HMAC） |
| `GET` | `/media/download/{encodedKey}?exp=&sig=` | 下载对象（HMAC） |

Initiate 示例字段：`mime_type`、`size_bytes`、`file_name`、`width`、`height`、`conversation_id`。

---

## 6. WebSocket 协议（Gateway）

- URL：`ws://localhost:8081/ws`
- 帧：Protobuf `WsFrame`（见 `livechat-server/proto/`），二进制 WebSocket message
- **连接限流**：每 IP ≤5 新连接/s，每 user ≤2 新连接/s（超限 HTTP `429` 或 ErrorFrame `4029`）

### 6.1 关键 Opcode

| Opcode | 值 | 方向 | 用途 |
|--------|-----|------|------|
| HANDSHAKE_REQ | `0x0001` | C→S | `HandshakeRequest`（含 `access_token`） |
| HANDSHAKE_RESP | `0x0002` | S→C | `session_id`、`latest_event_seq`、心跳间隔 |
| HEARTBEAT / ACK | `0x0003` / `0x0004` | 双向 | 保活 |
| ACK | `0x0005` | C→S | 投递/已读确认 → Gateway 转 gRPC ProcessAck |
| ERROR / DISCONNECT | `0x0006` / `0x0007` | S→C | 错误 / 踢下线 |
| MESSAGE_DELIVERY | `0x1001` | S→C | 实时消息 |
| MESSAGE_STATUS | `0x1002` | S→C | 状态更新 |
| SYNC / CONV UPDATE | `0x2001` / `0x2002` | S→C | 同步提示 / 会话更新 |

同 `user_id` + `device_id` 新连接会踢掉旧 Session（Error `4002`，`should_reconnect`）。

客户端重连退避：见 `internal/gateway/reconnect.go`（Spec 05 §6.1）。

---

## 7. 运维

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/health` | DB / Redis 探活 |
| `GET` | `/metrics` | expvar 指标 |

---

## 8. 多端最小闭环（curl 草图）

```bash
# 设备 A
curl -s -X POST localhost:8080/v1/auth/request_code -H 'Content-Type: application/json' \
  -d '{"phone_e164":"+8613800001001"}'
TOKEN_A=$(curl -s -X POST localhost:8080/v1/auth/verify_code -H 'Content-Type: application/json' \
  -d '{"phone_e164":"+8613800001001","verification_code":"123456","device_id":"ios-A","platform":"ios"}' \
  | jq -r .access_token)

# 设备 B（同一手机号 = 同一用户多端）
TOKEN_B=$(curl -s -X POST localhost:8080/v1/auth/verify_code -H 'Content-Type: application/json' \
  -d '{"phone_e164":"+8613800001001","verification_code":"123456","device_id":"ios-B","platform":"ios"}' \
  | jq -r .access_token)

# 建群拿 conversation_id，再用 TOKEN_A 发消息；TOKEN_B 侧 WS 收投递或 GET /v1/sync/events
```

更完整的客户端架构落点见 [`iOS多端接入评估与实现.md`](./iOS多端接入评估与实现.md)。
