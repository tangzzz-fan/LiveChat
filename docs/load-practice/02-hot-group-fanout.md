# 02 — 热点群 / 写扩散

## 1. 问题是什么

群消息写扩散：1 条消息 → N 条 `sync_events`（及在线投递）。

```
sync_events/s ≈ members × msg/s
例：200 × 100 = 20_000/s
```

热点群会把 Outbox/Fanout/DB 一起打满。

## 2. 业界常见方案

- WhatsApp 类：写扩散上限 + 大群降级  
- Telegram 类：偏读扩散  
- 热点检测 + 限流 / 丢实时保持久

## 3. 本仓现状

| 项 | 状态 | 锚点 |
|----|------|------|
| 分级扇出 | Implemented | `internal/fanout/service.go` |
| 热点 `ErrGroupBusy` | Implemented | fanout + outbox ack 不重试 |
| 压测 | Partial | `group_fanout.py`（成员数默认封顶 ~20，0031 抬阈值） |
| 演练手册 | Implemented | `docs/chaos/06-hot-group-flood.md` |

## 4. 如何模拟

```bash
cd load_test
python run.py --scenario group_fanout --concurrency 30 --duration 30

# 对照演练（造热点流量 + 观察保护）
# 见 docs/chaos/06-hot-group-flood.md
```

0031 完成后：用可配置成员数抬到可触发热点阈值。

## 5. 观察什么

| 信号 | 预期 |
|------|------|
| 日志 `hot group` / `ErrGroupBusy` | 达阈值后出现 |
| `outbox_pending_count` | 洪峰时上升 |
| Redis `hot_group:*`（若使用） | 标记热点 |
| 发送方 HTTP | 可能忙错或仍写成功但实时降级（按实现） |

## 6. 通过标准

- 触发保护时系统不雪崩（Postgres 不拖死）  
- **消息体仍持久化**；演练时单独验证「保护 ≠ 丢消息」  
- 恢复后正常群可继续收发  
