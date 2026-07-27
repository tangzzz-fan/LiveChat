---
id: "0056"
title: "iOS 消息长按菜单：复制 / 本地删除 / 失败重试"
status: complete
labels: ["done", "p1"]
blocked_by: ["0044"]
created_at: 2026-07-27
---

# 0056 — 消息长按单条菜单

## Context

会话气泡目前仅展示内容与己方状态符，无 `contextMenu` / 长按交互。先做单条菜单，作为后续多选（0057）与转发（0058）的交互骨架。

## What to build

- [x] `ChatThreadView` 消息行：`contextMenu`（或等价长按）
- [x] 菜单项（P1 最小集）：
  - **复制**：文本消息复制到剪贴板；图片可后置
  - **删除（本地）**：仅删本机投影，不声称服务端撤回（若无 recall API）
  - **重试**：仅 `failed` 单条 `queued` + 触发 `processPending`
- [x] Action → Middleware → Port/`MessageStore`，避免 View 直连 DB
- [x] 文档：`docs/ios-app-testing.md` 补手动步骤

## Acceptance criteria

- [x] 长按任意可见消息弹出菜单
- [x] 复制文本可用；本地删除后列表立即收敛（ValueObservation）
- [x] `failed` 可单条重试（不依赖工具栏整队「重试」）
- [x] 菜单项按状态/方向合理禁用（对方消息无「重试」；图片无复制）

## Out of scope

多选、系统分享、转发、服务端撤回/编辑（见 0057–0059）

## Related

- `ChatViews` · `ChatFeature` · `ChatMiddleware` · Spec 13 §9 · `MessageStore.deleteLocalMessage`
