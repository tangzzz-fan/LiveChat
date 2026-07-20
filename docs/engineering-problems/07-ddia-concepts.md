# DDIA 概念图谱

本文档梳理《Designing Data-Intensive Applications》(Martin Kleppmann) 中与 LiveChat 项目相关的核心概念，标注在项目中的应用位置和学习价值。

## 映射总览

| DDIA 章节 | 核心概念 | 当前应用 | 适用阶段 |
|-----------|---------|---------|---------|
| Ch3: Storage & Retrieval | B-Tree vs LSM-Tree, SSTable | messages 表索引设计 (conversation_id, conversation_seq) | 已应用 |
| Ch5: Replication | Leader-based, Read-after-write | — | P2 |
| Ch6: Partitioning | 分区键, 热点行, Rebalancing | 当前单 DB 未分区, 但 sync_events 已按 user_id 分片 (DDL) | P2 |
| Ch7: Transactions | ACID, Isolation levels | Outbox 事务 (messages + outbox_events 原子写) | 已应用 |
| Ch8: The Trouble with Distributed Systems | 不可靠网络, 部分失败, 超时 | 重连风暴 (问题 03), HTTP 响应丢失 (问题 06) | 已应用 |
| Ch9: Consistency & Consensus | Linearizability, CAP | 多端已读收敛, 服务端 seq 分配单调性 (问题 04) | 已应用 |
| Ch10: Batch Processing | MapReduce | — | 暂不需要 |
| Ch11: Stream Processing | Event sourcing, CDC, At-least-once | Outbox 消费者模式 (问题 01) | 已应用 |
| Ch12: The Future of Data Systems | 端到端原则, 审计 | — | P3 |

## 关键概念详解

### 1. 可靠性：容错 vs 韧性

DDIA 的核心原则：**系统即使在不可靠的组件上运行，也保持正确和可用。**

这直接影响项目的设计决策：
- Outbox 模式 → 防止写入和投递之间的部分失败 (问题 01)
- 客户端幂等键 → 防止重试产生重复消息 (问题 06)
- 服务端 seq 不回退 → 防止网络分区后序号冲突

### 2. 事务隔离级别与消息系统

DDIA 将 ACID 的事务隔离级别分为：Read Committed → Snapshot Isolation → Serializable。

聊天系统的特殊之处：
- 需要**单会话内 Serializable**（消息不能重复、不能丢、顺序不能错）
- 但**跨会话不需要 Serializable**（不同会话之间完全独立）
- Outbox 写就在这个交集上——messages + outbox_events 要在同一个 Serializable 事务中，但不需跨会话

### 3. 分区 (Partitioning)

DDIA 的核心问题：数据怎么分片，热点怎么处理。

当前项目的体现：
- `sync_events` DDL 已预留 `PRIMARY KEY (user_id, event_seq)`（按 user 分片）
- `messages` 按 `conversation_id` 分片（自然分区键）
- 热点群聊是分区策略的核心挑战 → 这是 P2 (Spec 07) 的学习重点
- 二次索引问题：分区后 conversation_summaries 的 `ORDER BY last_message_at` 跨分区查询变慢

### 4. Stream Processing vs Batch Processing

DDIA 区分：
- **Batch Processing**：定期全量处理（MapReduce）
- **Stream Processing**：事件驱动、增量处理（Kafka, CDC）

Outbox 消费者是一个简化版的 Stream Processing 引擎：
- 事件源：outbox_events 表
- 消费者：worker pool 并发处理
- Exactly-once 语义：通过幂等 (ON CONFLICT DO NOTHING) + At-least-once 投递 + 消费者去重 实现
- 窗口操作：ConversationSummary 的 unread_count 维护是简单的聚合状态

### 5. Linearizability (线性一致性)

DDIA 中最强的一致性模型：使多副本系统表现得像只有一个副本。

在聊天系统中的体现：
- conversation_seq 的单调递增 (SEQUENCE) → Linearizability
- 已读位置的 MAX 收敛 → 不是 Linearizability，而是 Eventual Consistency + CRDT-like merge
- 单会话内的消息顺序 → Linearizability（所有写同一个 SEQUENCE）

### 6. Leaderless Replication (P2 相关内容)

消息系统通常不用 Raft/Paxos 做复制，而是：
- 消息本身 → 异步复制（多副本最终一致）
- 序号分配 → 强一致（单一 SEQUENCE 或 Raft 选主）
- 路由信息 → 缓存（Redis TTL）

### 7. End-to-End Argument

DDIA 引用的端到端原则：**可靠性等功能不能只在通信系统的较低层实现，还必须在端点应用程序中实现。**

聊天系统的体现：
- 服务端保证消息不丢、不重（低层）
- 客户端仍然要本地去重（端点）——服务端只能保证一次持久化，但投递路径可能有重复
- 客户端的乐观更新（先本地入队再发请求）也是端到端思想

### 8. Derived Data vs Source of Truth

DDIA 区分：
- **System of Record (真理源)**：消息本身（messages 表）
- **Derived Data (衍生数据)**：ConversationSummary, sync_events, outbox_events

项目要求：衍生数据从真理源重建，不能反过来。ConversationSummary 丢了可以从 messages 重建；messages 丢了没有其他源能重建。

### 9. Backpressure (背压)

分布式系统中，生产者产出快于消费者处理时，需要背压机制。

在 Outbox Consumer 中：
- 当前的 worker pool (4 workers) 是固定大小——没有动态背压
- pending 积压 > 10,000 → 触发告警 (P0 级别)，但未自动限流
- 这是 P2 的学习重点：队列满了怎么办？

### 10. Coordination Avoidance (避免协调)

DDIA 的重要洞察：设计时应主动询问"我真的需要协调吗？"而不是默认所有操作都协调。

项目的体现：
- 已读位置用 MAX 收敛，不需要两阶段提交
- 每个会话独立 SEQUENCE，互不干扰
- 用户路由表用 Redis TTL 自动过期，不需要中心化 GC

### 11. The "Unbundling" of Databases

DDIA 的"数据库解绑"概念：传统单体数据库的功能（存储、索引、查询、缓存、消息队列）正在被拆分为独立组件。

当前项目的架构就是这个思想的实例：
- PostgreSQL：真理存储 + 事务
- Redis：路由缓存（内存、TTL、自动过期）
- Outbox 表：自建的"消息队列"
- ConversationSummary：物化视图（自建，非 PG 原生 MVIEW）
