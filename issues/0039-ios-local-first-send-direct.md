---
id: "0039"
title: "iOS 本地优先发文本 + 1:1 会话（GRDB + HTTP）"
status: open
labels: ["ready-for-agent", "p0"]
parent: "0035"
blocked_by: ["0038"]
created_at: 2026-07-27
---

# 0039 — 本地优先发文本 + 1:1 会话

## Parent

[0035](0035-ios-client-rewrite.md)

## What to build

登录后：调 `POST /v1/conversations/direct` 建/取 1:1 会话；发送文本消息先写 GRDB（queued→sending→accepted），再 HTTP send；最小会话列表 + 聊天页。本票**不要求** WebSocket；对端可用后续 sync 票或服务端查库验证。

## Acceptance criteria

- [ ] `EnsureDirect` 客户端对接已完成的服务端 0026 API；幂等、双方顺序无关
- [ ] 发消息：本地先落库再请求；`client_message_id` 幂等；状态机符合 Spec 13
- [ ] 有界发送队列（actor / 等价）；HTTP 429 尊重 Retry-After 退避
- [ ] 会话列表与聊天页读 GRDB 投影（或 ValueObservation）；Store 不持全量消息
- [ ] 双模拟器：两端登录后可各建与对方的会话并成功发送（本端 UI 显示 accepted）
- [ ] 横切：弱网/断网时消息留在 queued/sending，恢复后可续跑（最小演示）

## Blocked by

- [0038](0038-ios-auth-otp-keychain-login.md)

## 技术难点与注意事项

- 单 `DatabaseQueue` + WAL；禁止主线程同步重查询。
- 高负载：见 [ios-high-load-client.md](../docs/ios-high-load-client.md) 发送队列与 GRDB 写并发条目。
