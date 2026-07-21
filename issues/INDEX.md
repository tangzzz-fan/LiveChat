# Issues

| ID | Title | Status | Labels | Created |
|----|-------|--------|--------|---------|
| [0001](0001-phase-1-message-correctness-skeleton.md) | 阶段一：消息正确性骨架 — Message Service + Gateway 落地实现 | complete | `done` | 2026-07-20 |
| [0002](0002-scaffold-migrations-auth.md) | 项目脚手架 + DB 迁移 + Mock Auth | complete | `done` | 2026-07-20 |
| [0003](0003-message-send-api.md) | 消息发送 API + 幂等写入 + Outbox 事件 | complete | `done` | 2026-07-20 |
| [0004](0004-gateway-websocket-handshake.md) | Gateway：WebSocket 握手 + 心跳 + 用户路由注册 | complete | `done` | 2026-07-20 |
| [0005](0005-outbox-consumer.md) | Outbox 消费者：事件拉取、重试、死信 | complete | `done` | 2026-07-20 |
| [0006](0006-fanout-realtime-delivery.md) | 实时投递（Fanout）：Outbox → Gateway → WebSocket 推送 | complete | `done` | 2026-07-20 |
| [0007](0007-offline-sync-api.md) | 离线同步：增量事件 API + 游标管理 + 序号缺口检测 | complete | `done` | 2026-07-20 |
| [0008](0008-conversation-summaries.md) | 会话摘要投影 + 会话列表 API | complete | `done` | 2026-07-20 |
| [0009](0009-read-receipts-observability.md) | 已读回执 + 多端一致性收敛 + 可观测性 | complete | `done` | 2026-07-20 |
| [0010](0010-phase-2-user-visible-capabilities.md) | 阶段二：用户可感知能力 — 认证、群聊、媒体与推送 | complete | `done` | 2026-07-20 |
| [0011](0011-auth-device-sessions-push-token.md) | 认证收敛 + 设备会话管理 + Push Token 注册 | complete | `done` | 2026-07-20 |
| [0012](0012-group-conversation-membership-events.md) | 群会话创建 + 成员管理 + 群事件投影 | complete | `done`, `blocked-by:0011` | 2026-07-20 |
| [0013](0013-group-fanout-tiering-hot-group-protection.md) | 群消息扇出 + 分级策略 + 热点群保护 | complete | `done`, `blocked-by:0012` | 2026-07-20 |
| [0014](0014-image-media-upload-thumbnail-download.md) | 图片消息直传 + 缩略图 + 授权下载 | complete | `done` | 2026-07-20 |
| [0015](0015-offline-push-background-wakeup-dedupe.md) | 离线推送编排 + 后台唤醒 + 去重 | complete | `done`, `blocked-by:0011` | 2026-07-20 |
| [0016](0016-security-baseline-audit.md) | Phase 3 P0：安全基线加固与审计收敛 | complete | `done`, `blocked-by:0011` | 2026-07-21 |
| [0017](0017-storage-tiering-cache-layer.md) | Phase 3 P1：存储分层与通用缓存层 | ready-for-agent | `ready-for-agent`, `p1` | 2026-07-21 |
| [0018](0018-observability-histogram-tracing-alerts.md) | Phase 3 P0：可观测性升级 — Histogram 指标、分布式追踪与告警规则 | complete | `done`, `blocked-by:0016` | 2026-07-21 |
| [0019](0019-load-test-framework-baseline.md) | Phase 3 P0：压测框架与容量基线报告 | complete | `done`, `blocked-by:0011,0012,0013,0018` | 2026-07-21 |
| [0020](0020-chaos-engineering-runbooks.md) | Phase 3 P0：故障演练手册与恢复流程 | complete | `done`, `blocked-by:0018` | 2026-07-21 |
| [0021](0021-ios-client-architecture-skeleton.md) | Phase 3 P1：iOS 客户端架构骨架 | ready-for-agent | `ready-for-agent`, `p1` | 2026-07-21 |
