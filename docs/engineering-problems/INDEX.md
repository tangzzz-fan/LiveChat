# 工程问题库

本目录收录聊天系统设计与实现中遇到的典型工程问题，每篇按统一结构记录：

```markdown
# 问题标题

## 问题是什么
一句话描述现象或风险。

## 典型场景
- 什么条件下触发
- 为什么聊天系统中特别容易出现

## 通用分析思路
不绑定具体技术的拆解方法——怎么从现象定位到根因。

## 当前项目方案
在本项目 LiveChat 中的具体解法，关联 spec 编号与代码位置。

## 替代方案及取舍
如果不用当前方案，还有什么选择？各自代价。

## 踩坑记录
实现过程中实际遇到的意外、误解或补救（后续追加）。
```

创建文件时请遵守以下分类标签，确保每个文件只聚焦一个问题：

| 标签 | 领域 |
|------|------|
| `ordering` | 消息顺序、时钟、序号模型 |
| `idempotency` | 重复检测、幂等键设计 |
| `durability` | 消息不丢、持久化保证 |
| `consistency` | 多端一致、最终收敛、冲突解决 |
| `connection` | 长连接、心跳、重连、路由 |
| `fanout` | 群聊扩散、热点优化 |
| `offline` | 离线恢复、补拉策略 |
| `push` | 推送唤醒、通知去重 |
| `scale` | 容量规划、分片、限流 |
| `observability` | 监控、告警、排障 |
| `security` | 认证、加密、密钥管理 |

## 目录

| # | 标题 | 标签 | 摘要 |
|---|------|------|------|
| 01 | [消息不丢：写入与投递之间的一致性裂缝](01-message-durability-outbox.md) | `durability`, `idempotency` | Outbox 模式解决 DB 写入与 MQ 投递的非原子性问题。在写入数据库和投递消息队列之间没有原子性保证。项目用 messages + outbox_events 在同一事务中写入来保证两者原子可见。 |
| 02 | [消息顺序：弱网重试下的乱序风险](02-message-ordering-sequence.md) | `ordering`, `idempotency` | 客户端因弱网重试 M1 超时，M2/M3 先到服务端分配了序号，导致 M1 排在 M2 后面。SEQUENCE 单写点串行分配 seq 确保会话内序号严格单调。 |
| 03 | [重连风暴：大面积断连后的正反馈雪崩](03-reconnection-storm.md) | `connection`, `scale` | 网关重启后数万设备同时重连 → 网关过载 → 更多超时 → 再次重连。客户端退避 + jitter + 服务端限流的分层防御打破正反馈。 |
| 04 | [多端撕裂：同一账号的多设备状态不一致](04-multi-device-consistency.md) | `consistency`, `offline` | 设备 A 标记已读到 seq=50，离线设备 B 只有 seq=30。MAX 收敛 + 服务端单源 `unread_count` + 强制使用 `conversation_seq` 排序解决。 |
| 05 | [离线消息缺口：断连期间的消息如何高效补回](05-offline-gap-detection.md) | `offline`, `consistency` | 设备离线 3 小时产生 500 条消息，恢复时需检测缺口、高效拉取、不重不乱序。两层同步（全局事件流 + 会话消息补拉）+ cursor 管理解决。 |
| 06 | [服务端"消息已接收"不等于客户端"消息已送达"](06-message-lifecycle-stages.md) | `durability`, `consistency` | 收到 HTTP 200 只表示消息已持久化，不代表对端已收到。三阶段生命周期 Accepted → Delivered → Read + 独立 ACK 闭合这个语义差异。 |
| 07 | [来自 DDIA 的可移植概念](07-ddia-concepts.md) | *（跨领域理论）* | 可靠性、事务隔离、分区、流处理、线性一致、端到端原则等 11 个 DDIA 概念在 LiveChat 项目中的映射（含 P0 代码位置和 P2 扩展点）。 |
| 08 | [设备吊销与会话版本号：为什么不能只靠 JWT 过期](08-session-version-device-revocation.md) | `security`, `consistency` | JWT 无状态 vs 有状态吊销的矛盾。Session Version 方案：JWT 携带 sv claim，DB 存当前版本，中间件对比，吊销时递增。含 3 种替代方案对比 + 3 条踩坑记录。 |
| 09 | [两步认证的状态管理：验证码不能放在客户端回传](09-two-step-auth-code-storage.md) | `security`, `idempotency` | request_code → verify_code 两个独立 HTTP 请求之间，验证码必须存在服务端（Redis），决不能放在 JWT 里让客户端回传。含 Mock OTP 策略和频控设计。 |
| 10 | [群消息写扩散：1 条消息 N 倍写入的代价与控制](10-group-fanout-write-amplification.md) | `fanout`, `scale`, `consistency` | 200 人群 1 条消息 → 199 倍写入放大。三级分层（小群全写扩散 / 中群混合 / 大群读扩散）+ 热点群 Redis Sorted Set 滑动窗口保护。含 WhatsApp vs Telegram 架构对比。 |
| 11 | [推送不重复：在线投递与离线推送之间的重复消息问题](11-push-deduplication-coalescing.md) | `push`, `consistency`, `offline` | WebSocket 断开瞬间同时触发推送和重连，同一消息在两个通道各出现一次。推送 = 触发器而非消息载体 + sync 真相 + 客户端去重 + 30s 频控窗口合并。 |
| 12 | [缓存三大坑：穿透、击穿、雪崩的场景与对策](12-cache-penetration-blast-avalanche.md) | `scale`, `observability` | 缓存穿透（NULL object 30s TTL）、击穿（SET NX 互斥锁 + double-check）、雪崩（TTL ±20% 随机抖动）。三层防护 + 监控命中率 + 降级路径。 |
| 13 | [下载 URL 的签名安全：为什么不能直接暴露对象存储路径](13-download-url-hmac-signing.md) | `security`, `durability` | 媒体 URL 不能直接暴露存储路径——每次下载需校验会话成员资格。HMAC-SHA256 签名 URL + 过期时间 + `hmac.Equal` 常量时间比较。含 S3 Presigned URL vs 自签名对比。 |
| 14 | [跨服务 Trace 传递：为什么 slog 里有 trace_id 仍拼不成完整调用链](14-cross-service-trace-propagation.md) | `observability` | HTTP → gRPC → Outbox → Fanout 的 trace_id/span_id 传递与拼接。 |
| 15 | [高并发故障模式：IM 系统里什么会先崩](15-high-concurrency-failure-modes.md) | `scale`, `connection`, `fanout` | 对照 Spec 01 容量假设做放大因子推演；串联重连风暴、热点群、Outbox 背压与现有压测/演练缺口。 |
| — | [适应性学习 Roadmap](adaptive-learning-roadmap.md) | *（学习路线图）* | 已识别的高并发概念与落地状态（随实现更新）。 |
