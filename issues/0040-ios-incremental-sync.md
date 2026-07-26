---
id: "0040"
title: "iOS 增量同步：SyncExecutor + 游标"
status: open
labels: ["ready-for-agent", "p0"]
parent: "0035"
blocked_by: ["0039"]
created_at: 2026-07-27
---

# 0040 — 增量同步 SyncExecutor

## Parent

[0035](0035-ios-client-rewrite.md)

## What to build

实现增量 sync：按 `last_event_seq` 拉事件、分批 apply 到 GRDB、游标单调推进。端到端：A 发消息后 B 即使无 WS，拉 sync 也能在本地看到消息。

## Acceptance criteria

- [ ] SyncRepository Live + SyncExecutor（启动、回前台、手动触发）
- [ ] cursor 仅在成功 apply 后推进；缺口/失败不丢事件
- [ ] 分批 apply + 让出主线程；首屏优先可见会话
- [ ] 双模拟器：A 发送 → B sync → B 聊天页出现对应 `server_message_id`
- [ ] 横切：sync 洪流不卡死 UI（可用 Instruments 或主观帧率说明）

## Blocked by

- [0039](0039-ios-local-first-send-direct.md)

## 技术难点与注意事项

- WS 与 sync 统一经领域事件写 DB（可为后续 0041 预留入口）。
- 参考 Spec 13 §7、§8 与 ios-high-load-client「sync 追赶洪流」。
