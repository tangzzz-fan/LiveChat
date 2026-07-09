# SPEC-006 — 群聊：扇出放大与大群治理

> 状态: Draft | Milestone: M2 | 依赖: SPEC-003 | 被依赖: 007(群回执), 011(Sender Keys)

## 1. 背景与动机（Why）

群聊把 1:1 的每个成本乘以 N：一条消息 × 500 成员 = 500 次收件箱写 + 500 次
路由查询 + 500 次在线推送。**扇出放大是 IM 后端的第一性能问题**，也是
写扩散/读扩散权衡真正显形的地方。

## 2. 核心挑战与典型解法

### 挑战 A：写扩散的放大账单

500 人群、群内 10 msg/s → 5,000 inbox 写/s + 5,000 次推送判定/s，仅一个群。
业界参照：WhatsApp 群上限 1,024（写扩散撑得住的规模），微信 500，
Discord 频道无上限（因此被迫读扩散）。

**决定：**
- 群成员 ≤ 1,024（产品约束即架构约束——上限就是从这来的，学习点本身）；
- 保持写扩散，但扇出**异步化**：发送事务只写 messages + 群的 `conv_seq`
  （发送者 ACK 不等扇出），扇出任务进队列由 worker 批量执行；
- **读扩散路径作为文档章节阐述**（Discord 模型：只写会话时间线，
  客户端按会话拉取，`inbox` 退化为"会话已更新"信号），并说明切换阈值
  （单群 > 5k 成员或写放大 > 集群写容量 30% 时）。

### 挑战 B：异步扇出的正确性

发送者拿到 ACK 时扇出还没完成，如果 worker 挂了？

**解法：扇出任务持久化（outbox pattern 的服务端版）**：发送事务同时写一行
`fanout_jobs(msg_id, status)`；worker 领任务 → 批量写 inbox（单群一条 SQL
`INSERT ... SELECT FROM group_members`，不是 500 次单写）→ 标完成。
worker 崩溃 → 任务重新可见 → 重做。

**幂等的精确机制（评审 C2 修正）**：inbox 主键 `(user_id, inbox_seq)`
挡不住重做——重做会分配**新的** `inbox_seq`，新主键不冲突，同一条消息
就被投递两次。真正的幂等键是"同一消息对同一用户"，即 SPEC-003 DDL 中的：

```sql
CONSTRAINT uq_inbox_msg UNIQUE (user_id, conv_id, conv_seq)
```

扇出 SQL 写成 `INSERT ... SELECT ... ON CONFLICT ON CONSTRAINT uq_inbox_msg
DO NOTHING`——重做时已投递的行被约束吸收，只补写缺失的行。不改主键
（游标轴保持 `inbox_seq`），只加唯一约束，M1 的 DDL 已包含。
客户端游标同步（SPEC-003）天然兜底扇出延迟：慢了会到，不会丢。

### 挑战 C：在线推送的批量化

500 成员分散在 2 个网关上 → 不是 500 次 RPC，而是**按网关分组**：
查路由表（Redis pipeline MGET）→ 按 gateway_id 聚合 → 每网关一次
`PushToUsers(uids, frame)` 批量 RPC。放大系数从 O(成员数) 降到 O(网关数)。

### 挑战 D：群元数据与权限

- `groups` / `group_members(group_id, user_id, role, joined_at)`；
- 经典边界：**新成员能否看到入群前的历史？**（WhatsApp 不能，Telegram 可选）
  决定：不能——收件箱从入群时刻开始投递，历史消息按 `joined_at` 过滤。
  这个语义顺便简化了 E2EE（011 里新成员天然无旧密钥）；
- 退群/被踢后停止投递；系统消息（xxx 加入群聊）走同一消息管道
  （`MessageContent.system`），复用全部可靠性机制。

## 3. 范围

**In**：群 CRUD 与成员管理 API、异步扇出管道、批量推送、iOS 群会话 UI
（复用 004 的会话页，加成员列表/群名）。
**Out**：群已读回执（007）、群 E2EE（011）、@提及与消息引用（backlog）、
入群邀请链接（backlog）。

## 4. 验收标准

1. 500 人群（压测器造 500 成员，一半"在线"）发 1 条消息：全部在线成员
   E2E p99 < 1s；发送者 ACK 延迟与 1:1 场景无显著差异（< 2x，异步化的证明）。
2. 扇出 worker 在任务进行中被 `kill -9` → 重启后补齐，correctness 模式
   断言 500 收件箱全部到达、无重复。
3. 10 个 500 人群同时各注入 5 msg/s（= 2.5w inbox 写/s）持续 5 分钟，
   PG 无锁堆积、扇出队列深度有界（指标为证）。
4. 新成员入群后拉历史，看不到 `joined_at` 之前的消息（API 测试断言）。
5. 文档：读扩散模型与切换阈值章节，评审通过。

## 5. 测试计划

扇出幂等与权限过滤单测；testcontainers 集成测试覆盖入群/退群/踢人时序；
验收 1~3 为 loadtest 新场景 `group_500.yaml`。
