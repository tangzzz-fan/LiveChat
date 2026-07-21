# 高并发故障模式：IM 系统里什么会先崩

标签: `scale`, `connection`, `fanout`, `observability`

## 问题是什么

学习型 IM 很容易“功能都做完了”，但在**并发连接、热点写、积压背压**同时出现时，最先坏掉的往往不是业务逻辑，而是资源放大链路：连接握手、会话序号单写点、群写扩散、Outbox 消费速度。

## 典型场景（对照 Spec 01 容量假设做数量级推演）

| 场景 | 输入假设 | 放大后压力 | 本仓库现状 |
|------|----------|------------|------------|
| 重连风暴 | 网关重启，1 万设备同时握手 | 握手 QPS 尖峰 + Redis 路由写 | 客户端退避已实现；**IP/user 连接限流已落地（见下）**；压测场景 `reconnect_storm` |
| 文本写入高峰 | 1k–10k msg/s | DB 事务 + SEQUENCE + outbox | 单会话 `nextval` 串行；无发送侧全局限流 |
| 群写扩散 | 200 人群 × 100 msg/s | ~2 万 sync_events/s | 三级扇出 + 热点 `ErrGroupBusy` |
| Outbox 背压 | Consumer 停 / 变慢 | pending 线性涨，实时投递滞后 | Consumer 可追平；**发送侧尚未按 pending 反压 429** |
| 缓存失效叠加 | 热点 key 过期 + 高 QPS | DB 被打穿 | 工程问题 12 有策略；需靠压测验证 |

### 简易数量级模拟（不依赖跑服务）

```
热点群: N=200 members, R=100 msg/s
  messages:     100/s
  outbox:       100/s
  sync_events:  200 * 100 = 20_000/s   ← 写扩散主导
  若再 × 在线投递 RPC: 近似 online_ratio * 20_000

重连风暴: C=10_000 devices, 无 jitter 同秒重连
  握手尖峰 ≈ 10_000/s
  有 ±500ms jitter 后峰值 ≈ 10_000 / 0.5s ≈ 20_000/s 仍高
  指数退避后第 1 秒有效到达率可降一个数量级（依赖客户端守规矩）
```

结论：**群规模与重连同步性**比“单接口 QPS”更容易先打穿学习环境的 Postgres/Redis。

## 通用分析思路

1. **先画放大因子**：1 次用户动作变成几次 DB/Redis/网络调用？
2. **找单写点**：`conversation_seq` SEQUENCE、热点行、单 Consumer lease。
3. **区分正确性降级与可用性降级**：消息不丢但延迟升高，是否可接受？
4. **用同一套场景既压测又演练**：`load_test` 场景应对齐 `docs/chaos`。

## 当前项目方案

| 能力 | Spec / Ticket | 代码 / 文档 |
|------|---------------|-------------|
| 客户端重连退避 + jitter | Spec 05 §6.1 | `internal/gateway/reconnect.go` |
| 接入侧连接限流 | Spec 05 §6.2 | `internal/gateway/ratelimit.go`（每 IP / 每 user） |
| 热点群保护 | Spec 07, 0013 | `fanout.isHotGroup` + `ErrGroupBusy` |
| 压测框架 | Spec 12 §6, 0019 | `load_test/`（5 场景） |
| 故障演练 | Spec 12 §7, 0020 | `docs/chaos/01`–`06` |
| Outbox 积压可观测 | Spec 12 | Consumer `Metrics()` pending/lag |

### 仍开放的缺口（学习优先级）

1. **发送侧背压**：`outbox_pending` 超阈时 HTTP 429（见 adaptive-learning-roadmap §2）
2. **幂等窗口缓存**：高并发重试时减轻 unique 冲突压力（roadmap §10）
3. **基线报告**：`load_test/baselines/` 需在本机服务拉起后跑一轮填数
4. **Gateway 多节点 failover**：chaos 05 在单机只能验证重连，不能验证跨节点导流

## 替代方案及取舍

| 方案 | 优点 | 缺点 |
|------|------|------|
| 只靠垂直扩容 | 实现简单 | 学不到放大因子，掩盖设计问题 |
| 全链路限流（接入+业务+扇出） | 保护最强 | 调参复杂，易误伤正常突发 |
| 热点隔离 + 写扩散分级（当前主路径） | 与 WhatsApp 类规模匹配 | 大群实时性变差，产品需接受 |
| 读扩散为主（Telegram 向） | 写入恒定 | sync/游标模型更重，P0 未选 |

## 踩坑记录

- Ticket 0019/0020 曾标记 complete，但 chaos 手册曾只落地 Redis 一篇、多个压测场景仍是 stub——**文档完成 ≠ 可演练**。本轮已补齐手册并充实 `reconnect_storm` / `group_fanout`。
- 热点群 `ErrGroupBusy` 时 Outbox **直接 ack 不重试**：演练时必须单独验证“消息体是否仍对用户可见”，避免把“保护系统”误当成“消息丢了还以为正常”。
- 压测机 asyncio 调度延迟会污染端到端投递延迟绝对值；对比基线用相对变化，不要迷信单次 P95 数字。
