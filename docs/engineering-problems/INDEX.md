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
| 01 | [消息不丢：写入与投递之间的一致性裂缝](01-message-durability-outbox.md) | `durability`, `idempotency` | Outbox 模式解决 DB 写入与 MQ 投递的非原子性问题 |
| 02 | [消息顺序：弱网重试下的乱序风险](02-message-ordering-sequence.md) | `ordering`, `idempotency` | PostgreSQL SEQUENCE 作为会话内单调递增序号的单一写点 |
| 03 | [重连风暴：大面积断连后的正反馈雪崩](03-reconnection-storm.md) | `connection`, `scale` | 客户端退避 + jitter + 服务端限流的分层防御 |
| 04 | [多端撕裂：同一账号的多设备状态不一致](04-multi-device-consistency.md) | `consistency`, `offline` | MAX 收敛 + 服务端单源 + 强制使用 conversation_seq |
| 05 | [离线消息缺口：断连期间的消息如何高效补回](05-offline-gap-detection.md) | `offline`, `consistency` | 两层同步（全局事件流 + 会话补拉）+ cursor 管理 |
| 06 | [服务端"消息已接收"不等于客户端"消息已送达"](06-message-lifecycle-stages.md) | `durability`, `consistency` | 三阶段生命周期：Accepted → Delivered → Read |
| 07 | [来自 DDIA 的可移植概念](07-ddia-concepts.md) | *（跨领域理论）* | 可靠性、事务隔离、分区、流处理、线性一致、端到端原则等 11 个概念在 LiveChat 项目中的映射 |
| 08 | [适应性学习 Roadmap](adaptive-learning-roadmap.md) | *（学习路线图）* | 10 个已识别但尚未在代码中落地的高并发概念：gRPC 投递、背压、分片、热点群聊、连接迁移、写扩散 vs 读扩散、Copy-on-Write、结构化日志、Clock Skew、幂等窗口 |
