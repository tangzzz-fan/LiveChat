# 高负载实践模拟手册（load-practice）

对照 Spec 01 **学习型**容量假设：用可控流量验证放大因子与降级语义，**不**宣称打满峰值连接/写入。

| 编号 | 场景 | 文档 | 主要工具 |
|------|------|------|----------|
| 01 | 写高峰 | [01-write-peak.md](01-write-peak.md) | `load_test` send_message |
| 02 | 热点群 / 写扩散 | [02-hot-group-fanout.md](02-hot-group-fanout.md) | group_fanout + chaos 06 |
| 03 | 重连风暴 | [03-reconnect-storm.md](03-reconnect-storm.md) | reconnect_storm |
| 04 | Outbox 背压（消费积压） | [04-outbox-backpressure.md](04-outbox-backpressure.md) | chaos 02 |
| 05 | Redis 降级 | [05-redis-outage.md](05-redis-outage.md) | chaos 01 |
| 06 | DB 暂停 | [06-db-pause.md](06-db-pause.md) | chaos 03 |
| 07 | Gateway kill | [07-gateway-kill.md](07-gateway-kill.md) | chaos 05 |
| 08 | 离线 sync 积压 | [08-sync-backfill.md](08-sync-backfill.md) | sync_backfill（0031 硬化后） |

## 前置条件

1. 本机 PostgreSQL + Redis 已启动  
2. `make migrate-up`（在 `livechat-server/`）  
3. 分别启动 message-service / gateway / outbox-consumer（建议先 `make build` 再跑二进制，避免 `go run` 被终端回收）  
4. 压测依赖装在虚拟环境里：

```bash
cd load_test
python3 -m venv .venv && .venv/bin/pip install -r requirements.txt
```

```bash
# 快速烟雾（五场景）
cd load_test && .venv/bin/python run.py --quick --all

# 指标
curl -s http://localhost:8080/metrics | head
curl -s http://localhost:8082/metrics | grep outbox
```

## 与其他文档的关系

- **方法论总纲（复盘 / 对外讲述）**：[`../验证链路设计方法论.md`](../验证链路设计方法论.md)
- 概念与缺口总览：[`../engineering-problems/15-high-concurrency-failure-modes.md`](../engineering-problems/15-high-concurrency-failure-modes.md)
- 故障注入细则：[`../chaos/`](../chaos/)
- 推进计划与票：[`../高负载IM验证计划.md`](../高负载IM验证计划.md) · issues `0030`–`0034`
- iOS 客户端抗压（非容量打压）：[`../ios-high-load-client.md`](../ios-high-load-client.md)（0033）

## 统一通过标准（学习型）

- 行为符合该场景「预期系统行为」，指标方向正确  
- 恢复后 `bash livechat-server/scripts/chaos/health-check.sh` 通过（适用 chaos 场景）  
- **不**以 Spec 01 绝对数字为门禁  

## 本机实测基线

已跑通五场景，数字与关键观察见 [`load_test/baselines/local-measured-baseline.md`](../../load_test/baselines/local-measured-baseline.md)。

两个必读结论：

1. 单机压测最先撞到 **Gateway 接入限流（每 IP 5 conn/s）**，`connect` 的 98.9% 属 admission-limited 而非故障。  
2. 重连 jitter 直接决定恢复成功率（500ms jitter 下 2/10，无 jitter 0/10）。  
