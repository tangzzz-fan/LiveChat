---
id: "0058"
title: "iOS 消息转发 / 系统分享"
status: ready-for-agent
labels: ["ready-for-agent", "p2"]
blocked_by: ["0056"]
created_at: 2026-07-27
---

# 0058 — 转发与系统分享

## Context

单条菜单（0056）与可选多选（0057）之后，补齐「发到另一会话」与系统级分享。当前协议无独立 `forward` 消息类型时，采用**新发一条内容等价消息**（文本原样 / 图片复用 media_id 或再上传），并在本地 `content` 或 UI 标明「转发」可选。

## What to build

- 单条 / 多选 →「转发」→ 选目标会话（列表或 sheet）→ 本地优先发送
- 「分享」：`ShareLink` / `UIActivityViewController`（文本明文；图片用本地缓存或临时文件）
- 明确幂等：`client_message_id` 新建；不复用原 message_id
- 文档：产品语义（转发 ≠ 服务端引用消息）写入 Spec 13 或 study-guide

## Acceptance criteria

- [ ] 单条可转发到另一已存在会话并出现在目标会话
- [ ] 系统分享至少覆盖文本；图片有合理降级
- [ ] 多选转发：要么支持批量逐条发送，要么明确仅支持单条并禁用多选转发

## Depends

- 建议与 0057 联调；若 0057 未完成，可仅做单条转发

## Related

- `MessageSendExecutor` · `MediaAPI` · 会话列表选人 UI
