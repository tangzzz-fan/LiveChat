---
id: "0010"
title: "阶段二：用户可感知能力 — 认证、群聊、媒体与推送"
status: ready-for-agent
labels: ["ready-for-agent"]
created_at: 2026-07-20
---

# PRD: 阶段二 — 用户可感知能力（认证、群聊、媒体与推送）

## Problem Statement

Phase 1 已经完成消息正确性骨架，`livechat-server/` 具备 1:1 文本消息发送、实时投递、离线同步、会话摘要和已读收敛能力。

但从真实用户视角看，当前系统仍缺少 Phase 2 所要求的可感知能力：

- 账号体系仍停留在 mock auth，缺少设备会话管理和吊销闭环
- 无法创建和使用群聊
- 无法发送图片等媒体消息
- 设备离线或 App 进入后台后，没有系统级推送唤醒链路

这意味着当前系统虽然具备消息主链路，但还不是一个完整的 P0 即时通信产品闭环。

## Solution

围绕 `Specs/03`、`Specs/07`、`Specs/08`、`Specs/09` 落地 Phase 2 的 5 个垂直切片：

1. 认证收敛、设备会话管理与 Push Token 注册
2. 群会话创建、成员管理与群事件投影
3. 群消息扇出、分级策略与热点群保护
4. 图片消息直传、缩略图生成与授权下载
5. 离线推送编排、后台唤醒、去重与 Badge 更新

所有切片继续复用 `Spec 02` 中已经定义的 `Message`、`MessageReceipt`、`Conversation`、`SyncCursor` 和消息生命周期语义，避免 Phase 2 再次发明状态模型。

## User Stories

1. As a User, I want to verify my phone number and manage my logged-in devices, so that I can trust which devices can access my account.
2. As a User, I want to create a group conversation and manage its members, so that I can chat with multiple people in one place.
3. As a User, I want group messages to arrive in real time for online members and still sync correctly for offline members, so that the conversation remains complete.
4. As a User, I want to send and receive image messages with thumbnails, so that media sharing feels native rather than bolted on.
5. As a User, I want to receive a push notification when I am offline or in the background, so that I know new messages arrived.
6. As an Operator, I want hot groups and push delivery to be observable and rate-limited, so that Phase 2 features do not destabilize the Phase 1 baseline.

## Ticket Breakdown

- `0011` 认证收敛 + 设备会话管理 + Push Token 注册
- `0012` 群会话创建 + 成员管理 + 群事件投影
- `0013` 群消息扇出 + 分级策略 + 热点群保护
- `0014` 图片消息直传 + 缩略图 + 授权下载
- `0015` 离线推送编排 + 后台唤醒 + 去重
