---
id: "0045"
title: "iOS ValueObservation 去抖投影（突发投递不掉帧）"
status: complete
labels: ["p0"]
parent: "0043"
blocked_by: ["0044"]
created_at: 2026-07-27
---

# 0045 — ValueObservation 去抖

## Parent

[0043](0043-ios-high-load-leftover.md)

## What to build

对齐高负载 #1：GRDB `ValueObservation`（或等价）观察可见会话/消息窗查询，**16–33ms 去抖**后 `dispatch` 进 Store。  
去掉（或降级）「每批 DB 变更后 middleware 手写 refresh」为主路径。

WS / sync 仍只写 GRDB；禁止每帧直接打 Store。

## Acceptance criteria

- [x] 活跃会话消息窗由 observation 驱动更新
- [x] 去抖窗口可配置（默认约 16–33ms）
- [x] 突发投递（可用本地连续 insert 模拟）不导致每条一次全量 UI 重构的主路径
- [x] 对照 `ios-high-load-client.md` #1 勾选实现说明

## 实现备注

- `LocalProjectionObserver`：会话列表 + 消息窗 ValueObservation，默认 16ms 去抖
- WS `databaseChanged` 不再手刷 Store；打开会话 / 加载更早时绑定 observation

## Blocked by

- [0044](0044-ios-chat-list-seq-window.md)
