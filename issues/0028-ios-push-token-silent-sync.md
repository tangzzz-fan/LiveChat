---
id: "0028"
title: "iOS 推送 Token 注册 + 静默唤醒触发 sync"
status: open
labels: ["ready-for-agent", "p1"]
parent: "0021"
blocked_by: ["0024"]
created_at: 2026-07-21
---

# 0028 — iOS 推送 Token 注册 + 静默唤醒触发 sync

## Parent

[0021 - iOS 客户端架构骨架](0021-ios-client-architecture-skeleton.md)

## What to build

实现 PushRepository：向服务端注册 APNs device token；在收到远程通知（含静默）时走统一 RemoteEvent 入口触发 SyncExecutor，而不是把推送当消息真相源。端到端：token 成功注册到 `devices`；模拟/真实推送到达后客户端发起增量同步并更新本地 DB。

## Acceptance criteria

- [ ] 登录后请求通知权限并上传 push token（`POST /v1/devices/push-token`）
- [ ] token 更新（重装/更换）可覆盖写
- [ ] 通知处理进入统一事件总线 → 触发 sync（不直接把 payload 当完整消息写入，除非与 Spec 一致的最小字段）
- [ ] 与 Spec 13 §7 优先级一致：WS > sync 唤醒 > 可见点击启动
- [ ] 无真实 APNs 时可用开发开关模拟「收到静默推送」以验收 sync 触发
- [ ] 文档说明 mock APNs 与真机推送的边界

## Blocked by

- [0024 - iOS 增量同步：SyncExecutor + 多端补拉](0024-ios-incremental-sync-executor.md)

## 技术难点与注意事项

- 服务端推送编排当前为 mock：本票以「注册 + 客户端唤醒路径」为主，不要求生产级 APNs 投递率。
- Push 是触发器；消息真相仍在 sync / WS。
