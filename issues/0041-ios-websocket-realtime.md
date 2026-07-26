---
id: "0041"
title: "iOS WebSocket 实时投递：protobuf 握手 + MESSAGE_DELIVERY"
status: open
labels: ["ready-for-agent", "p0"]
parent: "0035"
blocked_by: ["0040"]
created_at: 2026-07-27
---

# 0041 — WebSocket 实时投递

## Parent

[0035](0035-ios-client-rewrite.md)

## What to build

`gen_proto` 生成物入库；经 `WebSocketTransport`（默认 URLSession）完成握手、应用层心跳、MESSAGE_DELIVERY 批量落库。端到端：前台两机实时可见。进后台断开；回前台重连（指数退避 + jitter + single-flight）。

## Acceptance criteria

- [ ] `ios/scripts/gen_proto.sh` 可跑；`*.pb.swift` 入库并编进 ChatInfrastructure
- [ ] 握手成功；心跳按协商间隔；断线用应用层心跳 + NWPathMonitor 兜底
- [ ] 投递事件先批量写 GRDB，再观察刷新 UI；禁止每帧 dispatch 进 Store
- [ ] 双模拟器前台：A 发送 → B 不手动 sync 也能实时看到
- [ ] 进后台断开（或不依赖保活）；回前台重连 + 立刻 sync（复用 0040）
- [ ] 横切：突发投递 observation 去抖 16–33ms；重连不形成本地风暴

## Blocked by

- [0040](0040-ios-incremental-sync.md)

## 技术难点与注意事项

- 不引入 Starscream；传输可替换。
- 与 0040 共用落库入口，避免双写竞态。
