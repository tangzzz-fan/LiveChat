---
id: "0037"
title: "iOS SPM 脚手架：清空重写 + 本地依赖；HITL 创建 Xcode 工程"
status: in-progress
labels: ["ready-for-human", "p0"]
parent: "0035"
blocked_by: ["0036"]
created_at: 2026-07-26
---

# 0037 — iOS SPM 脚手架（含 HITL：Xcode 工程）

## Parent

[0035 - iOS 客户端从零重写](0035-ios-client-rewrite.md)  
设计源：[0036](0036-ios-rewrite-design.md) / [docs/ios-client-rewrite.md](../docs/ios-client-rewrite.md) / Spec 13

## What to build

清空旧 `ios/` 后搭起可编译的模块骨架：多 SPM package + 薄 App 入口；本地 path 挂上 GRDB / swift-protobuf / TGReduxKit；占位 `WebSocketTransport` 与 Store 根装配说明。

**端到端**：在模拟器上能跑出一个空壳 App（启动页即可），`import GRDB` / `SwiftProtobuf` / `TGReduxKit` 能通过编译。

## Acceptance criteria

- [x] 旧 `ios/AppCore`、`ios/Packages` 等内容已移除或整体替换；`ios/README.md` 重写为新工程说明
- [x] SPM packages 至少覆盖 Spec 13 分层：`ChatDomain`、`ChatInfrastructure`、`ChatApplication`、`ChatPresentation`
- [x] `Package.swift` 用 **path** 依赖本地 `TG Libraries`（GRDB / swift-protobuf / TGReduxKit）
- [x] Infrastructure 侧有 `WebSocketTransport` + `URLSessionWebSocketTransport` 空壳（可编译）
- [x] Presentation 侧能创建根 `Store` 空状态（`swift test` 已过）
- [x] proto 生成脚本 + README 步骤写明（`ios/scripts/gen_proto.sh`；需本机 `brew install protobuf swift-protobuf`）
- [ ] **HITL（必须由你完成）**：创建 Xcode App 工程（iOS 17+，SwiftUI），把薄 App target 挂上上述 packages；Agent 在你确认工程路径后再 `xcodebuild` 联编验收
- [x] 开发期本机 HTTP：ATS / `NSAllowsLocalNetworking` 说明已写入 `ios/README.md`

## Blocked by

- [0036](0036-ios-rewrite-design.md)（已 complete）

## 进度（Agent）

四层 package `swift test` 均已通过。阻塞项仅剩：**你创建 Xcode 工程**（步骤见 `ios/README.md`）。

## HITL — 现在请你创建 Xcode 工程

1. Xcode → New Project → App（SwiftUI，iOS 17+，名 `LiveChat`）  
2. 保存到：`/Users/bigapple/Developments/LiveChat/ios/LiveChat/`  
3. Add Local Package：优先 `ios/Packages/ChatPresentation`  
4. 用 `ios/App/Sources/LiveChatApp.swift` 替换默认 App 入口  
5. 配 ATS `NSAllowsLocalNetworking`  
6. 回复：`工程已建好：ios/LiveChat/LiveChat.xcodeproj`

## 技术难点与注意事项

- 标签 `ready-for-human`：联编 AC 等你建完工程。  
- 不要把消息全量数组放进根 State。  
- 不引入 Starscream / Alamofire。
