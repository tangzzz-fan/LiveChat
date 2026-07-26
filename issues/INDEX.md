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
| [0017](0017-storage-tiering-cache-layer.md) | Phase 3 P1：存储分层与通用缓存层 | complete | `done`, `p1` | 2026-07-21 |
| [0018](0018-observability-histogram-tracing-alerts.md) | Phase 3 P0：可观测性升级 — Histogram 指标、分布式追踪与告警规则 | complete | `done`, `blocked-by:0016` | 2026-07-21 |
| [0019](0019-load-test-framework-baseline.md) | Phase 3 P0：压测框架与容量基线报告 | complete | `done`, `blocked-by:0011,0012,0013,0018` | 2026-07-21 |
| [0020](0020-chaos-engineering-runbooks.md) | Phase 3 P0：故障演练手册与恢复流程 | complete | `done`, `blocked-by:0018` | 2026-07-21 |
| [0021](0021-ios-client-architecture-skeleton.md) | Phase 3 P1：iOS 客户端架构骨架 | complete | `done`, `p1` | 2026-07-21 |
| [0022](0022-ios-auth-otp-keychain-login-ui.md) | iOS 登录：OTP + Keychain + 最小登录 UI | superseded | `wontfix`, `superseded→0035` | 2026-07-21 |
| [0023](0023-ios-local-first-send-grdb-http.md) | iOS 本地优先发消息：GRDB + HTTP send + 建群拿会话 | superseded | `wontfix`, `superseded→0035` | 2026-07-21 |
| [0024](0024-ios-incremental-sync-executor.md) | iOS 增量同步：SyncExecutor + 多端补拉 | superseded | `wontfix`, `superseded→0035` | 2026-07-21 |
| [0025](0025-ios-websocket-realtime-delivery.md) | iOS 实时投递：WebSocket 握手 + MESSAGE_DELIVERY | superseded | `wontfix`, `superseded→0035` | 2026-07-21 |
| [0026](0026-server-direct-conversation-api.md) | 服务端 1:1 建会话 API | complete | `done`, `p1` | 2026-07-21 |
| [0027](0027-ios-image-media-send-display.md) | iOS 图片消息：上传 + 发送 + 展示 | superseded | `wontfix`, `superseded→0035` | 2026-07-21 |
| [0028](0028-ios-push-token-silent-sync.md) | iOS 推送 Token 注册 + 静默唤醒触发 sync | superseded | `wontfix`, `superseded→0035` | 2026-07-21 |
| [0029](0029-high-load-im-validation.md) | 高负载 IM 验证：实践手册、压测硬化、发送背压与 iOS 抗压方案 | complete | `done`, `p0` | 2026-07-26 |
| [0030](0030-load-practice-playbook.md) | 高负载业界实践模拟手册（load-practice） | complete | `done`, `p0` | 2026-07-26 |
| [0031](0031-harden-load-test-chaos.md) | 压测与混沌演练硬化：stub、基线数字、chaos 04 注入 | complete | `done`, `p0` | 2026-07-26 |
| [0032](0032-send-side-outbox-backpressure.md) | 发送侧背压：outbox pending 超阈返回 429 | complete | `done`, `p0` | 2026-07-26 |
| [0033](0033-ios-high-load-client-design.md) | iOS 高负载/弱网客户端方案与坑点（文档先行） | complete | `done`, `p0` | 2026-07-26 |
| [0034](0034-loadtest-protobuf-ws-handshake.md) | 压测客户端补齐 protobuf WS 握手：覆盖握手之后的行为 | complete | `done`, `p1` | 2026-07-26 |
| [0035](0035-ios-client-rewrite.md) | iOS 客户端从零重写：SPM 多包 + GRDB + TGReduxKit + 薄 App | open | `ready-for-agent`, `p0` | 2026-07-26 |
| [0036](0036-ios-rewrite-design.md) | iOS 重写设计：Spec 13 修订 + 决策文档（Redux/WS/后台） | complete | `done`, `p0` | 2026-07-26 |
| [0037](0037-ios-spm-scaffold.md) | iOS SPM 脚手架：清空重写 + 本地依赖；HITL 创建 Xcode 工程 | complete | `done`, `p0`, `blocked-by:0036` | 2026-07-26 |
| [0038](0038-ios-auth-otp-keychain-login.md) | iOS 登录：OTP + Keychain + 最小登录 UI（重写） | complete | `done`, `p0` | 2026-07-27 |
| [0039](0039-ios-local-first-send-direct.md) | iOS 本地优先发文本 + 1:1 会话（GRDB + HTTP） | open | `ready-for-agent`, `p0`, `blocked-by:0038` | 2026-07-27 |
| [0040](0040-ios-incremental-sync.md) | iOS 增量同步：SyncExecutor + 游标 | open | `ready-for-agent`, `p0`, `blocked-by:0039` | 2026-07-27 |
| [0041](0041-ios-websocket-realtime.md) | iOS WebSocket 实时投递：protobuf 握手 + MESSAGE_DELIVERY | open | `ready-for-agent`, `p0`, `blocked-by:0040` | 2026-07-27 |
| [0042](0042-ios-push-token-silent-sync.md) | iOS Push Token + 静默唤醒触发 sync | open | `ready-for-agent`, `p0`, `blocked-by:0040,0041` | 2026-07-27 |
