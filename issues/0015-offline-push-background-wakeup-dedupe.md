---
id: "0015"
title: "离线推送编排 + 后台唤醒 + 去重"
status: ready-for-agent
labels: ["ready-for-agent"]
parent: "0010"
blocked_by: ["0011"]
created_at: 2026-07-20
---

## Parent

[0010 - 阶段二：用户可感知能力](0010-phase-2-user-visible-capabilities.md)

## What to build

落地 `Spec 09` 的 P0 推送触发器模型：当目标 `Device` 不在线或 App 处于后台时，由服务端决定 Silent Push 或 Visible Push，客户端通过后台同步和去重规则恢复消息，而不是把推送当作消息真相。

端到端行为：User A 向离线的 User B 发送消息后，Push Orchestrator 查出 B 的 Push Token 并发送 Silent Push 或 Visible Push；B 的 App 被唤醒后根据本地 `SyncCursor` 触发增量同步，把消息写入本地存储；如果同一消息随后又通过 WebSocket 到达，客户端不会重复提醒或重复记账。

具体交付：

- Push Orchestrator 服务或模块
- 设备在线 / 后台 / 被杀场景的推送决策树
- Silent Push 与 Visible Push 的 payload 构建
- 30 秒会话级频控、去重和 Badge 计算
- `push_events` 表与失败令牌处理
- App 侧后台唤醒后的同步触发协议

## Acceptance criteria

- [ ] 目标设备离线且存在 Push Token 时，消息产生后会创建一条 `push_event`
- [ ] 后台场景优先发送 Silent Push，被杀或不可后台恢复场景发送 Visible Push
- [ ] 同一会话在频控窗口内不会为每条消息都生成一条 visible push
- [ ] 客户端收到推送后触发增量同步，且同一条消息不会因推送和 WebSocket 双通道而重复展示
- [ ] Badge 值等于未读会话或未读消息的约定总数，且会随推送更新
- [ ] APNs 返回 BadDeviceToken 或等价错误时，服务端会把该 Push Token 标记为失效
- [ ] 推送仍然只是触发器，消息真相继续以消息存储和同步链路为准

## Blocked by

- [0011 - 认证收敛 + 设备会话管理 + Push Token 注册](0011-auth-device-sessions-push-token.md)
