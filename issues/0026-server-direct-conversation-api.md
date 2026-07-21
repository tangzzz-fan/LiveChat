---
id: "0026"
title: "服务端 1:1 建会话 API"
status: open
labels: ["ready-for-agent", "p1"]
parent: "0021"
blocked_by: []
created_at: 2026-07-21
---

# 0026 — 服务端 1:1 建会话 API

## Parent

[0021 - iOS 客户端架构骨架](0021-ios-client-architecture-skeleton.md)（解锁 iOS 私聊体验；也可独立服务端改进）

## What to build

提供创建 direct conversation 的 HTTP API，避免 iOS 用「两人小群」workaround。端到端：已登录用户 A 指定 peer 用户 B → 获得稳定 `conversation_id` → 双方均可发消息且成员校验通过；重复调用同一对用户返回同一会话（幂等）。

## Acceptance criteria

- [ ] 鉴权接口创建/获取 1:1 会话（路径与请求体在实现时与 Spec / API 参考对齐并文档化）
- [ ] 幂等：同一无序用户对只对应一个 direct conversation
- [ ] 写入 `conversations`（type=direct）与双方 `conversation_members`
- [ ] 非好友/非法 peer 的错误语义明确（至少：用户不存在 → 4xx）
- [ ] 集成测试覆盖：创建、重复创建、发消息成员校验
- [ ] 更新 `docs/API参考.md` 去掉「无 1:1 API」缺口说明

## Blocked by

None — can start immediately（可与 0022–0025 并行）。

## 技术难点与注意事项

- 会话 ID 生成策略需稳定可复现（例如规范化 user_id 对后哈希/排序拼接），避免双端各建一条。
- 与群会话 ID 命名空间区分（现有 `conv_grp_` 前缀）。
