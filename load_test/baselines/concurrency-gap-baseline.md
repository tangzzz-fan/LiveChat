# LiveChat 高并发能力差距与模拟基线（2026-07-21）

> 对照 Spec 01/05/07/12、tickets 0019/0020、工程问题库，用数量级模拟 + 缺口清单指导后续压测/演练。
> 详细条目见 [15-high-concurrency-failure-modes.md](../../docs/engineering-problems/15-high-concurrency-failure-modes.md)。

## 目标回顾

项目定位（Spec 00/01）：学习高并发、弱网、多端一致性，而非商业 SLA。容量假设用于架构取舍训练：峰值连接 5万–20万、写入 1千–1万 msg/s、热点群每秒数百消息。

## 已具备的防护

| 能力 | 证据 |
|------|------|
| Outbox 耐久 | 工程问题 01 + messages/outbox 同事务 |
| 会话序号单写点 | `nextval` / conversation_seq |
| 客户端重连退避 | `gateway/reconnect.go` |
| 接入连接限流 | `gateway/ratelimit.go`（本轮补齐 Spec 05 §6.2） |
| 群分级扇出 + 热点保护 | `fanout/service.go` |
| 压测框架骨架 | `load_test/` 五场景 |
| 故障演练手册 | `docs/chaos/01`–`06`（本轮补齐 02–06） |

## 数量级模拟（无需起服务）

```
A. 重连风暴
   C=10_000 devices 无 jitter 同秒重连 → ~10k handshake/s
   ±500ms jitter 仍可能 ~20k/s 尖峰；退避后尖峰下降一个数量级
   → 验证：load_test reconnect_storm + chaos 05

B. 群写扩散
   N=200, R=100 msg/s → sync_events ≈ 20_000/s
   → 验证：group_fanout + chaos 06；观察 hot_group ZCARD 与 ErrGroupBusy

C. Outbox 背压
   Consumer 暂停 T 秒、写入 W msg/s → pending ≈ W*T
   → 验证：chaos 02；关注发送 API 是否仍 200（当前会）与恢复追平时间
```

## 开放缺口（按学习优先级）

1. 发送侧按 `outbox_pending` 反压 429  
2. 幂等窗口缓存（高并发重试）  
3. 在本机跑通基线并写入 `load_test/baselines/`  
4. 多 Gateway 节点 failover（单机无法完整验证 chaos 05）

## 建议下次动手顺序

1. `make run-*` 起齐三服务 → `python load_test/run.py --quick --all`  
2. 选 chaos `01-redis-outage` 填一份 `_postmortem-template`  
3. 再跑 `group_fanout` 对照热点保护日志  
