---
id: "0039"
title: "iOS 本地优先发文本 + 1:1 会话（GRDB + HTTP）"
status: complete
labels: ["complete", "p0"]
parent: "0035"
blocked_by: ["0038"]
created_at: 2026-07-27
---

# 0039 — 本地优先发文本 + 1:1 会话

## Parent

[0035](0035-ios-client-rewrite.md)

## What to build

登录后：调 `POST /v1/conversations/direct` 建/取 1:1 会话；发送文本消息先写 GRDB（queued→sending→accepted），再 HTTP send；最小会话列表 + 聊天页。本票**不要求** WebSocket。

同时将 TGReduxKit 按 Feature 拆分（Auth / Chat），根 `AppState` 只做组合，避免 Action/Store 堆在单文件。

## Acceptance criteria

- [x] `EnsureDirect` 客户端对接（`ConversationAPI.ensureDirect`）
- [x] 发消息：本地先落库再请求；`client_message_id` 幂等；状态机 queued→sending→accepted/failed
- [x] 有界发送队列（`MessageSendExecutor` max 100）；HTTP 429 尊重 Retry-After + jitter
- [x] 会话列表与聊天页：Store 只持摘要行 + 可见窗口；全量在 GRDB
- [x] App 联编成功；本机 message-service 上 direct+send API 冒烟通过
- [x] 弱网续跑：`retryQueuedTapped` / 队列内 sending 可再 process；429 不转 failed

## Blocked by

- [0038](0038-ios-auth-otp-keychain-login.md)

## 验收记录（2026-07-27）

- Redux：`Features/Auth`、`Features/Chat` + `combineReducers`/`pullback`；logout 清空 chat
- Infra：`LocalDatabase` 持久化、`MessageSendExecutor`、`ConversationAPI`/`MessageAPI`、HTTP 429
- `swift test`（四层包）+ `xcodebuild` iPhone 17 Pro 绿
- 手动：登录后填对方 `user_id` → 打开会话 → 发送，本端状态到 accepted

## 技术难点与注意事项

- 不上 Moya；继续薄 `URLSession` `HTTPClient`
- ValueObservation 去抖留给后续票；本票发送后主动 reload 可见窗口
