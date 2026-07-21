---
id: "0027"
title: "iOS 图片消息：上传 + 发送 + 展示"
status: open
labels: ["ready-for-agent", "p1"]
parent: "0021"
blocked_by: ["0025"]
created_at: 2026-07-21
---

# 0027 — iOS 图片消息：上传 + 发送 + 展示

## Parent

[0021 - iOS 客户端架构骨架](0021-ios-client-architecture-skeleton.md)

## What to build

实现 MediaRepository：选图 → initiate/分片上传/complete → 发送 `message_type=image`（content 含 attachment 元数据）→ 对端经 download auth 拉取并展示（可用缩略图优先）。端到端：A 发送一张图，B 在聊天中看到图片气泡（或占位后加载完成）。

## Acceptance criteria

- [ ] 走通现有 media 上传/完成/下载授权 API
- [ ] 本地消息状态机覆盖 image 发送中/失败
- [ ] 聊天 UI 能展示本地/远端图片（允许先缩略图或占位）
- [ ] 非会话成员无法下载（依赖服务端 403，客户端友好提示）
- [ ] 大图/超限错误可理解（对齐服务端 50MB 等约束）

## Blocked by

- [0025 - iOS 实时投递：WebSocket 握手 + MESSAGE_DELIVERY](0025-ios-websocket-realtime-delivery.md)

## 技术难点与注意事项

- 本机对象存储：真机需指向 Mac 局域网 baseURL；模拟器可用 localhost。
- 上传与发消息解耦：先 complete 再 send image，避免无 object_key 的消息。
