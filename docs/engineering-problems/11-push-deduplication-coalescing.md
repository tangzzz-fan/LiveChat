# 推送不重复：在线投递与离线推送之间的重复消息问题

标签: `push`, `consistency`, `offline`

## 问题是什么

设备切换网络时，WebSocket 断开后的 100ms 内，服务端可能同时触发了推送和重连。如果处理不当，同一消息在 WebSocket 和推送中各出现一次，用户看到重复通知。

更根本的问题是：APNs 不保证 exactly-once——消息发送时目标可能正在重连、推送可能已经发出但设备尚未收到、设备收到推送时 App 已经在线。**推送通道和长连接通道是两个独立的投递路径，它们天然存在竞态。**

## 典型场景

```
User A 发送消息 → fanout →
  1. 发现 User B 设备离线 → 发送 Silent Push (APNs)
  2. 同时 User B 设备刚好重连 → WebSocket 投递 MESSAGE_DELIVERY
  3. B 通过 WebSocket 收到消息
  4. 500ms 后 APNs 推送到达 → App 被唤醒 → 触发 sync
  5. sync 返回同一条消息 → 重复写入本地 DB
```

**为什么 WhatsApp 能做到不重复？**
- 推送只是触发器（`sync_trigger`），不是消息载体
- 客户端收到推送后不直接展示消息，而是发起增量同步
- 本地 DB 对 `server_message_id` 有 UNIQUE 约束，重复写入被忽略

## 通用分析思路

1. **推送不是消息载体，是触发器**。推送 payload 不应包含消息内容——只包含"你有新消息，请来同步"的提示。
2. **真相以 sync 为准**。推送到达后，client 发起 `GET /sync/events`，从权威源（DB）拉取最新状态。
3. **客户端去重**。本地 DB 的 `server_message_id` UNIQUE 约束确保同一消息只存一次。
4. **频控窗口合并**。同一会话的多条消息在 30s 窗口内合并为 1 条推送。

## 当前项目方案

LiveChat 采用"推送 = 触发器 + sync 真相"模型（0015 实现）：

```
Push payload:
{
  "aps": {
    "content-available": 1,   // Silent Push
    "badge": 3
  },
  "sync_trigger": {
    "latest_event_seq": 15200,
    "reason": "new_message"
  }
}
```

**不推送消息内容的原因：**
1. APNs payload 最大 4KB——消息内容可能超限
2. 隐私：推送通知可能被手机预览显示
3. 正确性：推送不保证送达，消息内容以 sync 为准

**频控窗口（Redis）：**
```
Key: push_window:{user_id}:{conversation_id}
Value: {push_type: "silent"|"visible", last_push_at_ms, msg_count}
TTL: 30 seconds

规则:
  - 窗口内已有 visible push → 不发送（合并计数）
  - 窗口内已有 silent push > 60s → 升级为 visible
  - 无窗口 → 发送 silent push（优先静默，不影响用户）
```

**Silent Push 优先策略：**
- 首次离线 → Silent Push（`content-available: 1`, `apns-priority: 5`）
- 60s 后仍未上线 → Visible Push（`alert + sound + badge`）
- 静音群 → 不发 Visible Push

## 替代方案及取舍

| 方案 | 重复风险 | 延迟 | 复杂度 | LiveChat 选择 |
|------|---------|------|--------|--------------|
| 推送携带消息内容 | 高（竞态双写） | 低 | 低 | 不采用 |
| 推送 = 触发器 + sync | 低（客户端去重） | 中（需 sync） | 中 | **✅ 采用** |
| 推送前检查在线状态锁 | 低 | 低 | 高（分布式锁） | P1 可选 |

## 踩坑记录

1. **"Silent Push 被 iOS 节流"是平台限制，无法绕过**：低电量模式、后台 App 刷新关闭时，iOS 可能延迟或丢弃 Silent Push。P0 无法解决——消息不会丢，用户打开 App 时全量同步。
2. **推送频控的 Redis key 泄漏**：`push_window` key 的 TTL 是 30s，如果 Redis 故障，key 不会被创建，频控失效。失效的后果是"可能多发送几条推送"而非"丢消息"——这是可接受的降级行为。
3. **BadDeviceToken 需要异步清理**：APNs 返回 410 时，标记 `devices.push_token = ''`。如果在推送循环中同步清理会拖慢整个批次，独立处理更好。

### 代码位置

- `internal/push/orchestrator.go` → DecideAndPush（决策树）
- `internal/push/orchestrator.go` → sendSilent / sendVisible（payload 构建）
- `internal/push/orchestrator.go` → HandleBadDeviceToken（token 失效处理）
