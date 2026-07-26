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

完成脚手架与功能垂直切片，使 iOS 客户端成为：

1. 纯 SPM 多 package + 薄 App target  
2. GRDB 本地优先 + swift-protobuf 协议  
3. TGReduxKit 管视图真相、GRDB 管数据真相  
4. `WebSocketTransport` 默认原生 WS；前台长连、后台 APNs 唤醒  

端到端（本父票关闭时）：至少一条正确性主链路（登录→发消息→对端可见）闭环；高负载横切项写进各功能票 AC。

## Acceptance criteria

- [x] 子票 0036、0037 均 `complete`
- [x] Spec 13 与 `docs/ios-client-rewrite.md` 决策一致（含后台模型与 Redux 边界）
- [x] 旧 iOS 功能票已 `superseded`，INDEX 指向新父票
- [x] 功能垂直切片在 0037 后另开（0038–0042；不复活 0022–0028）
- [x] 导航文档指向本文档与新票
- [ ] 0038–0041 完成（主链路可演示）；0042 完成或明确延期说明

## Blocked by

None — 子票可立即启动。

## 子票

| ID | 标题 | 状态 |
|----|------|------|
| [0036](0036-ios-rewrite-design.md) | 设计落入 Spec + 决策文档 | complete |
| [0037](0037-ios-spm-scaffold.md) | SPM 脚手架 + Xcode 工程 | complete |
| [0038](0038-ios-auth-otp-keychain-login.md) | OTP 登录 + Keychain | open |
| [0039](0039-ios-local-first-send-direct.md) | 本地优先发文本 + 1:1 | open |
| [0040](0040-ios-incremental-sync.md) | 增量 sync | open |
| [0041](0041-ios-websocket-realtime.md) | WS 实时投递 | open |
| [0042](0042-ios-push-token-silent-sync.md) | Push + 静默 sync | open |

依赖：`0038 → 0039 → 0040 → 0041`；`0042` blocked by `0040`+`0041`。图片消息本轮不拆。

## 技术难点与注意事项

- Starscream 默认不引入；传输层必须可替换。
- 高负载验收横切见 `docs/ios-high-load-client.md`。
- 双模拟器默认：iPhone 17 Pro + iPhone 17 Pro Max（iOS 26.5）。
