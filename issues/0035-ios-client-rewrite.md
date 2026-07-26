---
id: "0035"
title: "iOS 客户端从零重写：SPM 多包 + GRDB + TGReduxKit + 薄 App"
status: open
labels: ["ready-for-agent", "p0"]
parent: null
blocked_by: []
created_at: 2026-07-26
---

# 0035 — iOS 客户端从零重写（父票）

## Parent

无。承接 Spec 13 修订、[0033](0033-ios-high-load-client-design.md) 高负载方案，以及多端接入评估。旧实现票 **0022–0025 / 0027–0028** 标记 `superseded`；服务端 [0026](0026-server-direct-conversation-api.md) 保留。

决策正文：[docs/ios-client-rewrite.md](../docs/ios-client-rewrite.md)

## What to build

完成子票 `0036`–`0037`（设计与脚手架），再按依赖拆功能垂直切片，使 iOS 客户端成为：

1. 纯 SPM 多 package + 薄 App target  
2. GRDB 本地优先 + swift-protobuf 协议  
3. TGReduxKit 管视图真相、GRDB 管数据真相  
4. `WebSocketTransport` 默认原生 WS；前台长连、后台 APNs 唤醒  

端到端（本父票关闭时）：脚手架可编译；至少一条正确性主链路（登录→发消息→对端可见）有独立功能票闭环；高负载横切项写进各功能票 AC。

## Acceptance criteria

- [ ] 子票 0036、0037 均 `complete`
- [ ] Spec 13 与 `docs/ios-client-rewrite.md` 决策一致（含后台模型与 Redux 边界）
- [ ] 旧 iOS 功能票已 `superseded`，INDEX 指向新父票
- [ ] 功能垂直切片在 0037 后另开（不复活 0022–0028 文件做实现）
- [ ] 导航文档（多端评估、`ios/README`、架构总览）指向本文档与新票

## Blocked by

None — 子票可立即启动。

## 子票

| ID | 标题 | 说明 |
|----|------|------|
| [0036](0036-ios-rewrite-design.md) | 设计落入 Spec + 决策文档 | 文档 |
| [0037](0037-ios-spm-scaffold.md) | SPM 脚手架 + **你建 Xcode 工程** | HITL |

## 技术难点与注意事项

- Xcode App 工程创建是 **HITL**（见 0037），Agent 不得假装已生成完整 `.xcodeproj` 签名工程。
- Starscream 默认不引入；传输层必须可替换。
- 高负载验收横切见 `docs/ios-high-load-client.md`。
