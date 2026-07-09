# SPEC-003 — 消息核心链路：不丢、不重、有序、离线可达

> 状态: Draft | Milestone: M1 | 依赖: SPEC-001, 002 | 被依赖: 004, 005, 006, 007, 008, 010

## 1. 背景与动机（Why）

IM 的信任底线：**用户按下发送，这条消息就必须到达，且只到达一次，且顺序正确
——无论对方在线、离线、还是三天后换了个网络才上线。** 本 spec 实现这条保证的
服务端部分（Message Service），是全项目的语义核心。

## 2. 核心挑战与典型解法

### 挑战 A：收件箱模型 —— 写扩散 vs 读扩散（IM 存储的第一大分野）

| 模型 | 机制 | 代价 | 采用者 |
|------|------|------|--------|
| 写扩散 (fan-out on write) | 发 1 条消息，给**每个接收者**的收件箱各写一条引用 | 写放大 N 倍；读极快（只查自己的收件箱） | 微信、WhatsApp（1:1 与小群） |
| 读扩散 (fan-out on read) | 消息只写会话时间线 1 次；读时聚合我参与的所有会话 | 写省；读放大（登录要扫所有会话） | Discord（超大频道） |

**决定：1:1 与小群用写扩散。** 理由：IM 读写比 >> 1（一条消息被读多次：
列表页、会话页、多设备），为读优化是共识；且写扩散让"每用户一个收件箱 +
一个游标"的同步模型极其简单。大群的读扩散切换阈值在 SPEC-006 讨论。

```sql
CREATE TABLE inbox (
  user_id       TEXT   NOT NULL,
  inbox_seq     BIGINT NOT NULL,  -- 每用户单调递增（收件箱游标的轴）
  conv_id       TEXT   NOT NULL,
  conv_seq      BIGINT NOT NULL,  -- 指向 messages 表
  PRIMARY KEY (user_id, inbox_seq)
);
```

注意有**两层序列号**：`conv_seq` 保证会话内展示顺序；`inbox_seq` 是每用户
同步游标（跨会话）。混淆这两者是自研 IM 最常见的设计事故。

### 挑战 B：发送管道的事务边界

发消息 = ①幂等检查 ②定序 ③写 messages ④写 N 条 inbox ⑤ACK 发送者 ⑥推送在线接收者。
哪些必须在一个事务里？

**解法：①~④ 单个 PG 事务（M1 规模下毫秒级）；⑤ 事务提交后立即回 ACK；
⑥ 是 best-effort 异步推送——推丢了无所谓，收件箱才是 source of truth，
客户端游标同步兜底一切。** 这就是"推拉结合"：推送求快，拉取求全。

时序（happy path）：

```
client ──send_msg──► gateway ──gRPC──► msgsvc
                                        │ tx: 幂等检查→conv_seq++→messages→inbox×N
                                        ├─ACK(conv_seq)──► gateway ──► sender   (~ms)
                                        └─async: 查 Redis 路由 → 在线者所在网关 gRPC push
                                                 离线者: 什么都不做(收件箱已有) + 触发推送(008)
```

### 挑战 C：客户端怎么知道自己没漏消息？

推送是不可靠的（连接断开瞬间的消息、网关重启、推送 RPC 失败）。

**解法：游标同步协议（整个系统可靠性的兜底）**

1. 客户端本地持久化 `last_synced_inbox_seq`；
2. 每次建连/重连成功后必发 `SyncRequest{since: last_synced_inbox_seq}`；
3. 服务端返回 `(since, since+N]` 的收件箱条目（分页，每页 200）；
4. 客户端落库后回 `AckDelivered{inbox_seq}`，推进服务端已投递水位；
5. **在线时收到的实时 push 同样携带 `inbox_seq`**：客户端发现跳号
   （收到 105 但本地水位 100）→ 立即触发一次增量 sync 补 101~104。

这个"断线重连后 sync 一次 + 在线跳号检测"的组合，就是消息不丢的完整证明链。

### 挑战 D：投递状态回执

sent（服务端已收）在挑战 B 的 ACK 里已解决。delivered（对方设备已收）：
接收方客户端落库后发 `AckDelivered`，msgsvc 更新水位并向**发送方**推
一条回执事件（合并批量推，避免回执流量 ≈ 消息流量 ×2）。read 回执在 SPEC-007。

## 3. 详细设计要点

- msgsvc 无状态多实例：定序靠 PG 行级锁（`UPDATE ... RETURNING`），天然安全；
  未来换 Redis INCR + WAL 的优化路径写进文档即可。
- 未读数：`unread:{uid}:{conv}` Redis INCR，读会话时清零回写。不追求强一致
  （业界也不追求——微信未读数偶尔漂移后靠打开会话校准）。
- 历史消息分页：`conv_seq < cursor ORDER BY conv_seq DESC LIMIT 50`，
  正好吃 `(conv_id, conv_seq)` 主键索引。
- 收件箱条目 90 天后归档删除（分区 drop），历史消息永久保留在 messages。

## 4. 范围

**In**：msgsvc 服务、上述协议全部服务端实现、API 服务的会话/联系人 CRUD、
未读数。
**Out**：群聊扇出优化（006）、已读回执与输入中（007）、推送触发的实现（008，
本 spec 只留 hook）、客户端实现（004）。

## 5. 验收标准（全部可实验证明）

1. **不丢**：压测注入 10w 条消息期间随机 `kill -9` gateway 与 msgsvc 各 3 次，
   结束后校验 harness 断言：每个接收者收件箱条目 = 应收条数，客户端本地库
   与服务端逐条比对一致。
2. **不重**：同一 `client_msg_id` 人为重发 5 次 → messages 表恰 1 行、
   接收端 UI 恰 1 条、5 次 ACK 返回相同 `conv_seq`。
3. **有序**：单会话并发 10 发送者×各 1k 条，任一接收端最终 `conv_seq`
   连续无空洞、无乱序（自动断言）。
4. **离线**：接收者离线期间注入 5k 条（跨 3 个会话）→ 上线一次 sync 全量补齐，
   分页正确，耗时 < 5s。
5. 单 msgsvc 实例吞吐 ≥ 1,000 msg/s（含 2 接收者写扩散），发送 ACK p99 < 100ms
   （本地环境基准，记录硬件条件）。

## 6. 测试计划

幂等/定序/游标逻辑单测覆盖 > 85%；集成测试：testcontainers 起 PG+Redis 跑
全链路小规模场景；验收 1~4 编入 loadtest 工具的 correctness 模式（SPEC-005），
可一键重跑。
