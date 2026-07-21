# 06 — 热点群消息洪峰

## 场景描述

大群在短时间内产生极高消息速率，触发热点群保护，避免写扩散与实时投递拖垮 DB / Fanout / Gateway。

**影响组件：** Fanout（`isHotGroup` / `ErrGroupBusy`）、Outbox Consumer、Redis `hot_group:{gid}`

## 注入方式

推荐用压测场景（比手工发消息更稳）：

```bash
cd load_test
python run.py --scenario group_fanout --concurrency 50 --duration 30
```

或手工：在 ≥50 人活跃群内，60s 滑动窗口内推送 > `HotGroupMsgThreshold`（当前 50）条消息。

配合观察：

```bash
redis-cli ZCARD "hot_group:<group_id>"
```

## 预期系统行为

1. 消息发送 API：HTTP 层仍可 Accepted（写入 messages + outbox）
2. Fanout 检测到热点后返回 `ErrGroupBusy`
3. Outbox Consumer 对 `ErrGroupBusy` **不重试**（刻意丢弃本次 fanout 实时路径），避免积压放大
4. 非热点群 / 私聊延迟应基本不受影响（隔离）
5. 客户端最终仍可通过 sync / 会话拉取拿到消息（保护的是扇出放大，不是删除消息体）

> 注意：当前实现是“热点时跳过本次 fanout 处理并 ack outbox”。演练时必须核对 **消息体是否仍可通过 GetMessages/sync 可见**；若发现可见性缺口，记入复盘 gap。

## 观察指标

| 指标 | 预期变化 |
|------|----------|
| Redis `ZCARD hot_group:*` | > 50 |
| 日志 `hot group event dropped` | 出现 |
| 非热点会话 send P95 | 接近基线 |
| DB CPU / outbox pending | 不应无界雪崩 |

## 恢复步骤

停止压测流量，等待 60s+ 滑动窗口过期：

```bash
redis-cli DEL "hot_group:<group_id>"
bash livechat-server/scripts/chaos/health-check.sh
```

## 验收标准

- [ ] 热点触发后系统进程未崩溃
- [ ] 非热点私聊仍可发送且延迟可控
- [ ] 热点群消息在保护解除后用户侧可收敛（或明确记录当前实现的可见性语义）
- [ ] Outbox 未因热点进入无限重试死循环

## 关联

- Spec 07 / Spec 12 §7
- 工程问题 [10-group-fanout-write-amplification](../engineering-problems/10-group-fanout-write-amplification.md)
- 代码：`internal/fanout/service.go`（`HotGroupMsgThreshold=50`）
