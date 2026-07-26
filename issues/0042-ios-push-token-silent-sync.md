---
id: "0042"
title: "iOS Push Token + 静默唤醒触发 sync"
status: open
labels: ["ready-for-agent", "p0"]
parent: "0035"
blocked_by: ["0040", "0041"]
created_at: 2026-07-27
---

# 0042 — Push Token + 静默唤醒 sync

## Parent

[0035](0035-ios-client-rewrite.md)

## What to build

注册 APNs push token；Silent Push / 点击可见推送唤醒后**只**跑增量 sync（预算内），符合「后台不硬撑 WS」。

## Acceptance criteria

- [ ] `POST /v1/devices/push-token` 注册/更新成功
- [ ] 处理后台通知入口 → SyncExecutor（与 0040 同一路径）
- [ ] 后台不依赖 WS 收消息；文档/代码注释与 Spec 13 §8.2 一致
- [ ] 模拟器或真机至少一种路径可演示：唤醒后 sync 补齐缺口（可用服务端 mock push / 本地注入）
- [ ] 横切：后台预算内不做重 UI / 大媒体

## Blocked by

- [0040](0040-ios-incremental-sync.md)
- [0041](0041-ios-websocket-realtime.md)（避免与实时路径双入口未收敛）

## 技术难点与注意事项

- 模拟器 push 能力有限；允许用本地调用 `handleRemoteNotification` 的集成测试代替真 APNs。
- 图片消息不在本票范围。
