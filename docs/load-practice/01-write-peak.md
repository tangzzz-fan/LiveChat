# 01 — 写高峰

## 1. 问题是什么

短时间大量 `POST /v1/messages/send`：每条消息 ≈ 1 次事务（messages + outbox）+ `conversation_seq` SEQUENCE。

放大因子（单会话）：

```
send_qps ≈ DB_tx/s ≈ nextval(conversation_seq)/s ≈ outbox_insert/s
```

多会话并行时 SEQUENCE 压力分散，但 DB 连接与 WAL 仍线性涨。

## 2. 业界常见方案

- 会话内序号批量预分配 / 分片  
- 发送侧全局限流与按用户配额  
- Outbox 异步解耦实时路径（本仓已用）

## 3. 本仓现状

| 项 | 状态 | 锚点 |
|----|------|------|
| 幂等写入 + Outbox | Implemented | `internal/messages` |
| 会话 SEQUENCE | Implemented（串行单写点） | migrations / messages service |
| 发送侧全局背压 | Implemented（0032） | `internal/backpressure`，pending 超阈 → 429 + `Retry-After` |
| 压测场景 | Implemented（0031） | `load_test/scenarios/send_message.py`（走 `/v1/groups` 建会话） |

## 4. 如何模拟

```bash
# 服务已启动后
cd load_test
.venv/bin/python run.py --scenario send_message --concurrency 50 --duration 30

# 快速
.venv/bin/python run.py --scenario send_message --quick
```

同一会话高压（学习用）：保持低并发但对**同一** `conversation_id` 狂发，观察延迟尖峰。

## 5. 观察什么

| 信号 | 预期 |
|------|------|
| `curl :8080/metrics` 中 send/延迟相关 | P95 随并发上升 |
| `curl :8082/metrics` → `outbox_pending_count` | 短暂上升后被消费拉回 |
| HTTP 5xx | 应接近 0 |
| HTTP 429 + `send_backpressure_rejected_total` | 消费跟不上时出现，属保护而非故障 |
| Postgres 连接 / 锁等待 | 同会话写高峰时可能升高 |

本机实测：并发 10 / 10s 下 167 req/s、P95 约 98ms、pending 峰值 8。数字见 [`local-measured-baseline.md`](../../load_test/baselines/local-measured-baseline.md)。

## 6. 通过标准

- 压测结束错误率可接受（学习环境：无大面积 5xx；429 单独计量）  
- 消息最终可 sync / 在线可见（consumer 未暂停）  
- 记录一份基线到 `load_test/baselines/`（0031 已完成首轮）  
