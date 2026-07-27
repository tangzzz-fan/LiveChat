---
id: "0062"
title: "iOS 聊天页键盘：顶起最新消息 + 点空白失焦"
status: complete
labels: ["done", "p1", "ux"]
blocked_by: ["0044"]
created_at: 2026-07-27
---

# 0062 — 聊天键盘 UX

## What to build

- [x] 键盘唤出时，消息列表滚到最新，底部气泡不被挡住
- [x] 点击消息区（键盘已展示）时释放输入焦点、收起键盘
- [x] Composer 用 `safeAreaInset` / `FocusState`；列表 `scrollDismissesKeyboard`

## Acceptance criteria

- [x] 点输入框 → 键盘升起后滚到最新（`keyboardDidShow` + focus onChange）
- [x] 点消息列表 / 气泡 → 键盘收起
- [x] 发送/收到新消息时仍自动滚到底（既有行为保留）

## Related

- `ChatThreadView` · `docs/ios-app-testing.md` §11
