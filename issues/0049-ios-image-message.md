---
id: "0049"
title: "iOS 图片消息：上传 + 发送 + 展示（高负载 #9）"
status: complete
labels: ["done", "p1"]
parent: "0043"
blocked_by: ["0044"]
created_at: 2026-07-27
---

# 0049 — 图片消息

## Parent

[0043](0043-ios-high-load-leftover.md)

## Notes

服务端媒体 API 已就绪（0014：initiate / status / complete / download auth）。本票补客户端；缩略图优先、取消离屏加载对齐高负载 #9。  
**后端：不阻塞。**

## What to build

- 选图 → 上传 → 发 image 类型消息 → 聊天气泡展示缩略图
- 磁盘/内存缓存策略（可薄）
- 滚动离屏取消解码/下载

## Acceptance criteria

- [x] 双端可见图片消息（或至少本端发送 + 对端 sync/WS 可见）— `MediaAPI` + `messageType=image` content attachment；本端缓存、对端 download/auth
- [x] 不以原图直载卡死列表 — `MessageImageBubble` 降采样 + `ImageMediaCache` + 离屏取消
- [x] high-load #9 文档更新

## Blocked by

- [0044](0044-ios-chat-list-seq-window.md)
