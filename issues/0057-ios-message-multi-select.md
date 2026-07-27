---
id: "0057"
title: "iOS 消息多选模式：批量删除 / 转发入口"
status: complete
labels: ["done", "p1"]
blocked_by: ["0056"]
created_at: 2026-07-27
---

# 0057 — 消息多选模式

## Context

依赖 [0056](0056-ios-message-long-press-menu.md) 的菜单入口。本票做会话内多选态与批量本地操作；真正「转发到另一会话」落在 [0058](0058-ios-message-forward-share.md)。

## What to build

- [x] 长按「选择」进入多选；选中集合进 Redux
- [x] 多选工具栏：批量本地删除；转发占位（提示 0058）
- [x] 勾选视觉；点击切换选中
- [x] 退出多选清空 selection

## Acceptance criteria

- [x] 可从长按菜单进入多选并选中当前条
- [x] 可多选若干条；批量本地删除后投影收敛
- [x] 退出多选后 UI 回到普通浏览
- [x] 转发入口存在（完整逻辑 blocked_by 0058）

## Related

- `ChatFeature` selection state · `ChatViews` · Spec 13 §9
