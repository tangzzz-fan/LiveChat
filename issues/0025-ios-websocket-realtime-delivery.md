---
id: "0025"
title: "iOS 实时投递：WebSocket 握手 + MESSAGE_DELIVERY"
status: open
labels: ["ready-for-agent", "p1"]
parent: "0021"
blocked_by: ["0024"]
created_at: 2026-07-21
---

# 0025 — iOS 实时投递：WebSocket 握手 + MESSAGE_DELIVERY

## Parent

[0021 - iOS 客户端架构骨架](0021-ios-client-architecture-skeleton.md)

## What to build

实现 WebSocketRepository：连接 Gateway、Protobuf 握手、心跳、接收 MESSAGE_DELIVERY，经统一远程事件入口写入本地 DB 并刷新 UI。端到端：A、B 同时在线时，A 发送文本后 B **秒级**在聊天页看到消息，无需手动下拉同步。

## Acceptance criteria

- [ ] 可配置 `wsURL`（默认 `ws://127.0.0.1:8081/ws`）
- [ ] 发送 HANDSHAKE_REQ（带 access_token）；处理 HANDSHAKE_RESP（含 `latest_event_seq`、心跳间隔）
- [ ] 心跳按协商间隔；断线指数退避 + jitter（与 Spec 05 / 服务端退避语义一致）
- [ ] 收到 MESSAGE_DELIVERY → RemoteEvent → 本地 insert-if-needed
- [ ] 握手后的 `latest_event_seq` 可触发一次 sync 对齐缺口
- [ ] 双端在线演示：实时投递成功；断线后仍可靠 0024 sync 补齐
- [ ] 同 device 重复连接被踢时能按 should_reconnect 恢复

## Blocked by

- [0024 - iOS 增量同步：SyncExecutor + 多端补拉](0024-ios-incremental-sync-executor.md)

## 技术难点与注意事项

- 帧格式为仓库 Protobuf `WsFrame`；可用 SwiftProtobuf 生成或最小子集手写，但需与 Gateway 互通。
- 接入限流：短时间狂连可能 429 / Error 4029，客户端必须退避。
- UI 仍不得直连 socket；只消费 DB / ViewModel。
