---
id: "0015"
title: "离线推送编排 + 后台唤醒 + 去重"
status: ready-for-agent
labels: ["ready-for-agent"]
parent: "0010"
blocked_by: ["0011"]
created_at: 2026-07-20
updated_at: 2026-07-21
---

# 0015 — 离线推送编排 + 后台唤醒 + 去重

## Parent

[0010 - 阶段二：用户可感知能力](0010-phase-2-user-visible-capabilities.md)

## What to build

在消息投递链路末端增加推送决策与服务端 Push Orchestrator：目标设备不在线时，根据设备状态选择 Silent Push 或 Visible Push，通过 APNs HTTP/2 发送推送请求；服务端做频控、节流和 Badge 计算；处理 APNs 的错误反馈（BadDeviceToken → 标记失效）；客户端在 `sync_trigger` 的引导下触发增量同步而非把推送内容当作消息真相。

端到端行为：User A 发消息给离线的 User B → Outbox consumer 完成 fanout 后发现 B 不在线 → Push Orchestrator 查出 B 的设备 Push Token → 判断 App 在后台发 Silent Push、App 被杀发 Visible Push → 构造 APNs payload → 调用 APNs HTTP/2 API → 写入 `push_events` 表 → B 的手机收到推送后 App 被唤醒 → App 根据 `sync_trigger.latest_event_seq` 触发 `GET /v1/sync/events` → 消息写入本地 DB → 同一会话的后续消息在 30s 频控窗口内合并（不发重复的 visible push） → APNs 返回 BadDeviceToken 时标记该 token 失效。

## Acceptance criteria

- [ ] 消息投递后，如果目标设备不在线且存在 Push Token → Push Orchestrator 创建 `push_event` 记录并构造 APNs 请求
- [ ] 后台场景：发送 Silent Push（`content-available: 1, apns-priority: 5`）
- [ ] 被杀/不可后台恢复场景：发送 Visible Push（`alert + sound + badge`），隐私文案 `"🔒 新消息"`
- [ ] Push payload 包含 `sync_trigger { latest_event_seq, reason }` 字段，客户端据此触发增量同步
- [ ] 同一会话的 multiple 消息在 30 秒频控窗口内不重复产生 visible push（合并为 "N 条新消息" 或只发一次 silent push）
- [ ] Badge 值 = 未读会话总数（或有未读消息的会话数），随每次推送更新
- [ ] 用户若已静音某群，该群消息不触发 visible push（仍可触发 silent push）
- [ ] APNs 返回 `410 Unregistered` 或 `BadDeviceToken` → 自动将该设备的 `push_token` 清空
- [ ] 推送失败不影响消息投递链路——消息已写入 sync_event，客户端上线后通过同步补拉
- [ ] `push_events` 表记录 push_type（silent/visible）、apns_status（sent/rejected/error）和时间
- [ ] 在线设备（WebSocket 已连接）不触发推送——Gateway 的路由表已足够判断在线状态

## Blocked by

- [0011 - 认证收敛 + 设备会话管理 + Push Token 注册](0011-auth-device-sessions-push-token.md)（需要 `devices.push_token` 字段和 `POST /v1/devices/push-token` 端点）

## 技术难点与注意事项

### 1. 在线状态判断的时序窗口

**问题：** 在"查询路由表发现在线"到"推送决策"之间，设备可能刚好断开。或者"查询发现离线"到"发送推送"之间，设备刚好重连。两种情况都会导致重复消息。

**方案：**
- 在线优先：先检查 Redis 路由表，在线就 WebSocket 投递，不推送
- 离线推送后客户端重连 → 增量同步拉取消息 → 消息已有 `server_message_id`，客户端本地去重（UNIQUE 约束）
- 推送不做去重保证（APNs 本身不保证 exactly-once），真相以同步为准

**这就是 Spec 09 的核心原则：推送是触发器，消息真相以同步链路为准。**

### 2. APNs Provider 的实现

**问题：** APNs 使用 HTTP/2 + JWT 认证（Token-Based Authentication），不是简单的 HTTP/1.1。

**方案：**
- 使用 Apple 的 `github.com/sideshow/apns2` 库（或手写 HTTP/2 client）
- 认证方式：JWT（APNs Auth Key），不是证书（P0 开发环境用 mock）
- **P0 关键简化：APNs 调用用 mock 实现**——不真的连接 Apple 服务器，而是：
  - `APNsClient` 接口定义 `Send(ctx, deviceToken, payload) (apnsID, status, error)`
  - `MockAPNsClient` 实现：总是返回 `200 OK`，记录日志
  - P1 替换为真实 APNs HTTP/2 client

**坑点：** APNs 的 sandbox vs production 有两个不同的 endpoint（`api.sandbox.push.apple.com` vs `api.push.apple.com`）。Mock client 不需要关心，但接口设计要预留 endpoint 选择。

### 3. 设备 online/background/killed 的判断

**问题：** 服务端无法精确知道 iOS App 是在后台还是被杀。这需要客户端协同。

**方案（P0 简化）：**
- 在线：WebSocket connected → 不推送
- 离线：WebSocket disconnected → 服务端**总是先发 Silent Push**（`content-available: 1`）
- Visible Push：作为 fallback——如果同一 conversation 在 60s 内已发过 silent push 但设备仍未上线 → 发 visible push（`alert + sound`）
- 用 Redis 记录 `last_silent_push:{user_id}:{conversation_id}` → TTL 60s

**替代方案：** 客户端在 WebSocket 断开前发送"进入后台"通知（`APP_BACKGROUND` 帧）。但 WebSocket 可能异常断开，这个信号不可靠。P0 不依赖客户端信号。

**坑点：** Silent Push 可能被 iOS 节流（低电量模式、后台 App 刷新关闭等）。这是 Apple 平台限制，P0 无法解决。消息不会丢失——下次 App 打开时全量同步。

### 4. 频控的精确性

**问题：** "30 秒内同会话不重复发 visible push"——这个窗口的判断需要带状态。

**方案：** Redis key `push_window:{user_id}:{conversation_id}`，value = `{last_push_type, last_push_at_ms, message_count}`，TTL 30s。

新消息到来时：
1. 查询 push_window key
2. 如果存在且 `last_push_type == 'visible'` → 不重复发 visible push，更新 message_count
3. 如果不存在或已过期 → 发送推送，创建 push_window key
4. 如果 message_count > 1 → Silient Push 升级为 Visible Push（"N 条新消息"）

### 5. Badge 计算

**问题：** badge 值应该等于"有未读消息的会话数"还是"所有会话的未读消息总数"？

**WhatsApp 做法：** 有未读消息的会话数。iOS 角标显示的是会话数，不是消息数。

**P0 实现：**
```
SELECT COUNT(*) FROM conversation_summaries 
WHERE user_id = $1 AND unread_count > 0 AND is_hidden = false
```
每次推送时查询此值，写入 `aps.badge`。

**坑点：** 如果推送发送后、App 唤醒前有其他会话的消息到达，badge 可能略微不准。这是 APNs 的 Best-Effort 特性，可以接受。

### 6. Push Orchestrator 在 Outbox 流程中的位置

**问题：** 推送决策应该在 fanout 的哪个环节触发？

**方案：** 在 Outbox consumer 的 fanout 完成之后。当前 Phase 1：
```
Outbox Consumer → fanout.Service.Fanout() → 在线投递 + sync_events
```

Phase 2 增加：
```
Outbox Consumer → fanout.Service.Fanout() → 在线投递 + sync_events
                                         → pushOrchestrator.DecideAndPush()
```

`DecideAndPush` 接收 fanout 的结果（哪些 member 在线、哪些离线），对离线且有 push_token 的设备发起推送。

**坑点：** fanout 和 push 不应在同一个 goroutine 串行——fanout 的延迟会拖慢推送。用 goroutine + channel 异步触发推送。

### 7. 涉及的关键文件

- `internal/push/` — 新包：orchestrator.go、apns.go（接口+mock）、decision.go（决策树）
- `internal/outbox/consumer.go` — fanout 完成后调用 Push Orchestrator
- `internal/fanout/service.go` — Fanout 返回投递结果（哪些设备在线/离线）
- `migrations/` — push_events 表已存在；devices 表 push_token 在 0011 补齐
- `internal/domain/types.go` — 补 PushEvent 结构体
- `configs/` — push 频控窗口、静音开关等配置
