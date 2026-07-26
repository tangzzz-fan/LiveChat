---
id: "0037"
title: "iOS SPM 脚手架：清空重写 + 本地依赖；HITL 创建 Xcode 工程"
status: open
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

- [ ] 旧 `ios/AppCore`、`ios/Packages` 等内容已移除或整体替换；`ios/README.md` 重写为新工程说明
- [ ] SPM packages 至少覆盖 Spec 13 分层：`ChatDomain`、`ChatInfrastructure`、`ChatApplication`、`ChatPresentation`（命名可微调，职责对齐）
- [ ] `Package.swift`（或等价）用 **path** 依赖：
  - `../../TG Libraries/GRDB.swift`（或文档记载的相对路径）
  - `../../TG Libraries/swift-protobuf`
  - `../../TG Libraries/TGReduxKit`
- [ ] Infrastructure 侧有 `WebSocketTransport` 协议 + `URLSessionWebSocketTransport` 空壳（可先 no-op / TODO，但类型可编译）
- [ ] Presentation 侧能创建根 `Store` 空状态（证明 TGReduxKit 链路通）
- [ ] proto 生成脚本或 README 步骤写明（生成物可入库）
- [ ] **HITL（必须由你完成）**：创建 Xcode App 工程（iOS 17+，SwiftUI），把薄 App target 挂上上述 packages；Agent 在你确认工程路径/工程已建好后再继续联编验收
- [ ] 开发期本机 HTTP：Info.plist ATS 例外或等价说明写进 `ios/README.md`

## Blocked by

- [0036](0036-ios-rewrite-design.md)（已 complete）

## HITL — 请你创建 Xcode 工程时

到本票开工时 Agent 会再次提醒。你需要：

1. Xcode → New Project → App（SwiftUI，iOS 17+，产品名建议 `LiveChat`）  
2. 将工程放在仓库约定位置（建议 `ios/LiveChat/` 或与 Agent 对齐的路径）  
3. File → Add Package Dependencies → **Add Local…** 指向各 package；或用 XcodeGen/`project.yml`（若本票选定该方式）  
4. 把结果路径告诉 Agent，再跑编译验收  

Agent **不会**在未通知你的情况下假定 `.xcodeproj` 已存在。

## 技术难点与注意事项

- 标签 `ready-for-human`：本票阻塞在 Xcode 工程创建；Agent 可先写 package 骨架，但联编 AC 等你建完工程。  
- 不要把消息全量数组放进根 State（即使是空壳示例也不要示范反模式）。  
- 不引入 Starscream / Alamofire。
