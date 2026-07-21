---
id: "0023"
title: "iOS 本地优先发消息：GRDB + HTTP send + 建群拿会话"
status: open
labels: ["ready-for-agent", "p1"]
parent: "0021"
blocked_by: ["0022"]
created_at: 2026-07-21
---

# 0023 — iOS 本地优先发消息：GRDB + HTTP send + 建群拿会话

## Parent

[0021 - iOS 客户端架构骨架](0021-ios-client-architecture-skeleton.md)

## What to build

实现本地 DB 为真相源的发送闭环：登录后通过建群获得 `conversation_id`，发送文本时先写入本地 `queued`/`sending`，再 HTTP `messages/send`，成功后更新为 `accepted` 并带上 `server_message_id` / `conversation_seq`。会话列表与消息列表只从 GRDB 观察刷新。

端到端：用户 A 建群并邀请 B（或两人同群）→ A 发「hello」→ A 本机列表立即可见发送中/已接受状态；B 侧本票可不要求实时，可用后续 sync 票验证。

## Acceptance criteria

- [ ] GRDB 对 `messages` / `conversation_summaries` 提供真实读写（替换 stub）
- [ ] `MessageSendExecutor`（或等价 Use Case）：enqueue → 本地写库 → HTTP send → 更新状态；幂等使用稳定 `client_message_id`
- [ ] 通过群 API 创建会话并写入本地 summary（在 1:1 API 就绪前作为会话来源）
- [ ] 最小 UI：会话列表 + 聊天页输入发送；UI 只订阅本地 DB
- [ ] 失败路径：`failed` 状态可见，可手动重试（至少一种）
- [ ] 非法成员/403 时本地状态收敛正确，不留下永久 sending

## Blocked by

- [0022 - iOS 登录：OTP + Keychain + 最小登录 UI](0022-ios-auth-otp-keychain-login-ui.md)

## 技术难点与注意事项

- 服务端暂无 1:1 建会话 HTTP：用建群 workaround；0026 落地后可切换。
- `content` 为 JSON 字符串（如 `{"text":"..."}`），与现有 API 对齐。
- 状态机合法转移遵循 Spec 13 / ChatDomain `MessageStatus`。
