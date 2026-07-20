# 多端撕裂：同一账号的多设备状态不一致

标签: `consistency`, `offline`

## 问题是什么

同一账号的设备 A（iPhone）和设备 B（iPad）同时对同一会话做操作——A 标记已读，B 发送消息——如果没有收敛规则，两端最终的会话状态（未读数、最后一条消息、已读位置）永远不一致。

## 典型场景

```
设备 A: 			已经读了会话 conv-1 的前 50 条消息
设备 B: 			离线中（在飞机上）
→ 设备 A 上报已读到 seq=50
→ 服务端写入 sync_event {type: message_read, last_read_seq: 50}
→ 
设备 B 开机: 		连接 → 同步
→ 收到 sync_event {last_read_seq: 50}
→ 但 B 的本地 last_read_seq = 30（上次同步时的状态）
→ 如果不做 MAX 收敛 → B 的未读 badge 从 20 变成 20（错误！）
```

## 通用分析思路

1. **识别冲突操作是什么**：已读推进、未读数、会话排序——哪些是"增量"操作（只会变大/变小），哪些是"替换"操作？
2. **确认收敛算子**：
   - 单调递增的 → `MAX`（last_read_seq、event_seq）
   - 从服务端重算的 → 直接覆盖（unread_count）
   - 有明确写序号的 → 按序号的最后值
3. **检查反例**：有没有场景下 MAX 会导致错误收敛？如果有，说明需要更复杂的方案（如 CRDT）。

核心原则：**优先找能用 MAX 或 LAST-WRITER-WINS 解决的简单算子**。不要一上来就上 CRDT。

## 当前项目方案

### 已读位置：MAX 收敛（Spec 06 §5.1）

```sql
UPDATE sync_cursors
SET last_event_seq = GREATEST(sync_cursors.last_event_seq, $new_val)
```

- `last_read_seq = MAX(本地值, 同步事件中的值)`
- MAX 满足交换律和结合律——无论事件到达顺序如何，最终值一致。
- 单调性保证：已读位置只能向前，不能回退。

### 未读数：服务端单源（Spec 06 §5.2）

- `conversation_summaries.unread_count` 是唯一计算源。
- 新消息 → `unread_count + 1`（对除发送者外的所有成员）。
- MarkRead → `unread_count = 0`。
- 客户端通过 sync_event 获取更新——不自己推理未读数。

### 消息排序：强制使用服务端 conversation_seq（Spec 06 §5.3）

- 客户端渲染永远 `ORDER BY conversation_seq ASC`。
- 不按 `server_received_at` 排序——跨服务器的微秒级时钟偏差会导致逆序。

代码：`livechat-server/internal/sync/service.go` — `UpdateCursor()` 使用 `GREATEST`。

## 替代方案及取舍

| 方案 | 优点 | 缺点 |
|------|------|------|
| **MAX / LWW**（当前） | 简单、确定、易验证 | 只适用单调递增的数据 |
| **CRDT (RGA/Treedoc)** | 可处理任意冲突 | 复杂、消息排序不适用 |
| **OT (Operational Transformation)** | Google Docs 方案 | 需要中心化协调、延迟高 |
| **客户端投票** | 无中心节点 | 学习成本高、延迟不可控 |

## 踩坑记录

- MAX 收敛前提是：**被更新的值永远不会变小**。如果由于某种 bug sync_cursor 被更新为 9999，MAX 会把所有设备都推到 9999，且永远不会恢复。Cursor 没有"回退"机制。当前方案依赖 DB unique constraint 和正确的业务逻辑防止错误写入。
- `GREATEST` vs 应用层 MAX：让 DB 做 GREATEST 更原子化（不需要 SELECT + 比较 + UPDATE），但要注意 NULL 处理：`GREATEST(0, NULL) = NULL`——所以 DDL 中所有 cursor 列都 `NOT NULL DEFAULT 0`。
