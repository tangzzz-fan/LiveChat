# 本机实测基线（0031）

> 学习型本机数字，**不代表** Spec 01 容量（5万–20万连接 / 1千–1万 msg/s）已达成。  
> 复现命令见每行「备注」；场景说明见 [`docs/load-practice/`](../../docs/load-practice/README.md)。

## 环境

| 项 | 值 |
|----|----|
| 日期 | 2026-07-26 |
| git SHA | `95e3d15` |
| 机器 | MacBook Pro 18,3（arm64，10 核） |
| OS | macOS 26.5.2 |
| 依赖 | 本机 PostgreSQL 16 + Redis 7（均 `ok`） |
| 服务 | message-service `:8080`、gateway `:8081`、outbox-consumer `:8082`（编译后二进制） |
| 压测 | `load_test/.venv`（httpx / websockets / rich），`--quick` = 并发 10、10s |

## 结果

| 场景 | 并发 | 时长 | 请求数 | 吞吐 | P50 | P95 | P99 | 错误率 | 备注 |
|------|------|------|--------|------|-----|-----|-----|--------|------|
| send_message | 10 | 10s | 1585 | 157.9 req/s | 43.6ms | 110.1ms | 201.7ms | 0.6% | 群会话建会话，无 psql 依赖 |
| connect | 10 | 10s | 4997 | 499.0 req/s | 5.4ms | 10.5ms | 58.3ms | 0.0% | **accepted=54 / throttled=4943（98.9%）**，见下 |
| group_fanout | 10 | 10s | 641 | 63.8 req/s | 143.2ms | 170.1ms | 220.6ms | 0.6% | members=10（`--max-members` 可抬） |
| sync_backfill | 10 | 10s | 693 | 68.8 req/s | 126.0ms | 217.2ms | 236.6ms | 0.0% | 预灌 ≥50 条后从 cursor=0 拉 limit=50 |
| reconnect_storm | 10 | 10s | — | — | — | — | — | — | 逐请求指标无意义，见「重连风暴」 |

复现：

```bash
cd load_test
python3 -m venv .venv && .venv/bin/pip install -r requirements.txt
.venv/bin/python run.py --scenario send_message --quick
.venv/bin/python run.py --scenario connect --quick
.venv/bin/python run.py --scenario group_fanout --quick
.venv/bin/python run.py --scenario sync_backfill --quick
.venv/bin/python run.py --scenario reconnect_storm --quick
```

## 关键观察

### 1. 接入限流是本机最先撞到的天花板（预期行为）

Gateway 按 Spec 05 §6.2 限流：**每 IP 5 conn/s、每用户 2 conn/s**（`internal/gateway/ratelimit.go`）。

单机压测源自同一 IP，因此：

- 尝试 ~499 次/s → **仅 ~5.4 次/s 被接纳**（10s 内 accepted=54）
- 其余 98.9% 是 **admission-limited**，已在场景里与真实故障区分，不计入 `Errors`
- 结论：限流生效；**不能**用本机 connect 吞吐推断「可支撑多少连接」

### 2. 重连风暴：jitter 明显影响恢复成功率

同为 10 条连接、同一 IP：

| 配置 | 首次建连 | 风暴重连成功 | 耗时 |
|------|----------|--------------|------|
| `--jitter-ms 500`（默认） | 5/10 | **2/10** | 0.50s |
| `--no-jitter` | 5/10 | **0/10** | 0.01s |

无 jitter 时全部撞在同一瞬间 → 令牌桶耗尽 → 全部被拒。这正是工程问题 03 的正反馈机制在小规模上的可观测版本。

注意：`reconnect_storm` 的风暴在 `setup()` 里一次性测量，`execute()` 是廉价空转，所以报告里的 `reqs/rps/P50` **不要解读为业务吞吐**。

### 3. OTP 频控约束了压测脚本设计

`request_code` 有同 IP ~20 次/小时限制。因此 `connect` / `send_message` / `sync_backfill` 都改为 **setup 预建小用户池并复用**，而不是每次迭代注册新用户（旧实现导致 87.7% 假错误）。

### 4. 写路径与扇出延迟量级

- 1:1/群文本发送：P95 约 110ms（含幂等 INSERT + outbox 同事务 + seq 分配）
- 10 人群扇出：P95 约 170ms；成员数抬高会放大 `sync_events` 写入
- sync 回补（50 条/页）：P95 约 217ms

未触发热点群保护（`HotGroupMsgThreshold` 为 60s 内 50 条，本轮速率与成员数下未越线）。

## 待补（不在本轮）

- `--max-members` 抬到能触发 `ErrGroupBusy` 的量级，记录热点保护对照
- 与 chaos 02（outbox 暂停）联跑，作为 0032 发送侧背压的前后对照
- 多机/多 IP 压测（本机单 IP 无法绕过接入限流）

## 相关文档

- 概念差距：[`concurrency-gap-baseline.md`](./concurrency-gap-baseline.md)
- 演练手册：[`docs/load-practice/`](../../docs/load-practice/README.md)
- 高负载计划：[`docs/高负载IM验证计划.md`](../../docs/高负载IM验证计划.md)
