---
id: "0036"
title: "iOS 重写设计：Spec 13 修订 + 决策文档（Redux/WS/后台）"
status: complete
labels: ["complete", "p0"]
parent: "0035"
blocked_by: []
created_at: 2026-07-26
---

# 0036 — iOS 重写设计落入 Spec + 决策文档

## Parent

[0035 - iOS 客户端从零重写](0035-ios-client-rewrite.md)

## What to build

把已拍板与最佳实践修订写入 Spec 与决策文档，作为脚手架与后续功能票的唯一设计源。不写业务 Swift 实现。

产出：

- [Specs/13-iOS客户端架构设计.md](../Specs/13-iOS客户端架构设计.md) 修订  
- [docs/ios-client-rewrite.md](../docs/ios-client-rewrite.md)

## Acceptance criteria

- [x] Spec 13 §2.1：工程形态（SPM 多包、GRDB、swift-protobuf、TGReduxKit、清空重写、本地 path 依赖）
- [x] Spec 13 §4.2–4.3：ValueObservation 去抖 + Redux/GRDB 边界与反模式
- [x] Spec 13 §6：`WebSocketTransport` 协议；默认原生 URLSessionWebSocketTask
- [x] Spec 13 §8.2：改为前台长连 / 后台断开 + APNs 唤醒（废除「后台保活 WS + 120s 心跳」）
- [x] `docs/ios-client-rewrite.md`：拍板表、本地库版本、WS 对比结论、HITL Xcode 说明、票序
- [x] 导航：多端评估 / ios README / 架构总览可链到本文（随 INDEX 一并更新）

## Blocked by

None — can start immediately.

## 技术难点与注意事项

- 设计变更先 Spec 后文档/票，符合仓库约定。
- 不在本票创建 Xcode 工程或清空 `ios/`（属 0037）。
