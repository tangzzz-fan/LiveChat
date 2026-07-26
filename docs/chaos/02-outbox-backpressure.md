# 02 — Outbox Consumer 堵塞 / 背压

## 场景描述

Outbox Consumer 进程被暂停或处理过慢时，`outbox_events` 积压增长。消息已持久化（Accepted），但实时投递与 sync 投影滞后。

**影响组件：** Outbox Consumer、Fanout、Gateway 实时投递、Sync 事件流

## 注入方式

```bash
# 暂停 outbox-consumer（SIGSTOP，进程仍在但不再调度）
bash livechat-server/scripts/chaos/outbox-pause.sh

# 或者手动：注意要匹配二进制本身，不要停到 shell 包装进程
# kill -STOP $(pgrep -x outbox-consumer)
```

前置：先 `make build` 再运行 `./outbox-consumer`。用 `go run` 时进程名是临时文件，`pgrep -x` 匹配不到。

## 预期系统行为

1. `outbox_pending_count` / pending 行数上升
2. 在线 WebSocket 收不到新消息（Fanout 未消费）
3. 客户端离线同步游标也停滞（sync_events 由 Fanout 写入）
4. Consumer 恢复后追平积压，消息最终可达（最终一致，非实时）
5. 发送侧行为取决于是否开启背压（ticket 0032）：
   - **无背压**：send 持续 200，pending 单调上升，积压无上限
   - **有背压**：pending 越过阈值后 send 返回 429 + `Retry-After`，积压被封顶

**关键验证：** 暂停期间写入的消息在恢复后全部被消费，无死信、无丢失。

## 观察指标

| 指标 | 预期变化 |
|------|----------|
| `outbox_pending_count` | 上升（有背压时在阈值附近封顶） |
| `outbox_lag_seconds` | 上升 |
| `ws` 投递相关计数 | 停滞或骤降 |
| HTTP send 5xx | 应保持接近 0 |
| `send_backpressure_rejected_total`（:8080） | 有背压时上升 |
| `http_requests_total{path="/v1/messages/send",status="429"}` | 有背压时上升 |

## 背压对照演练（ticket 0032）

开启背压后重启 message-service（阈值调低便于观察）：

```bash
cd livechat-server && make build
SEND_BACKPRESSURE_PENDING_THRESHOLD=50 \
SEND_BACKPRESSURE_RETRY_AFTER_SEC=5 \
SEND_BACKPRESSURE_SAMPLE_MS=1000 \
./message-service
```

三阶段：

```bash
bash livechat-server/scripts/chaos/outbox-pause.sh
cd load_test && .venv/bin/python run.py --scenario send_message --quick
bash ../livechat-server/scripts/chaos/outbox-resume.sh
cd load_test && .venv/bin/python run.py --scenario send_message --quick
```

### 本机实测（2026-07-26，阈值 50）

| 阶段 | send 结果 | pending | 说明 |
|------|-----------|---------|------|
| consumer 正常 | 167 rps，err 0.6% | 0–8 | 消费跟得上写入 |
| consumer 暂停 | 193 rps 尝试，**95.2% 返回 429** | 停在 **88** | 背压封顶积压，未继续增长 |
| consumer 恢复 | 82 rps，err 0.2% | 回到 **0** | 无需重启进程即自动放行 |

两点值得注意：

- pending 停在 88（>阈值 50）而不是精确等于阈值，因为采样间隔 1s 内仍会放行一批写入。背压是**限制积压增速**，不是硬上限。
- 未开背压时，同样的注入会让 send 一路 200、pending 无上限增长——这是 429 想避免的失败模式。

## 恢复步骤

```bash
bash livechat-server/scripts/chaos/outbox-resume.sh
bash livechat-server/scripts/chaos/health-check.sh
```

观察 pending 在恢复后 60s 内下降。

## 验收标准

- [ ] 恢复后 60s 内 `outbox_pending_count` 接近 0（或回到注入前基线）
- [ ] 注入期间发送的消息可通过 sync / 实时投递到达
- [ ] 无死信堆积；重试计数未异常飙升
- [ ] 消息发送 API 在注入期间保持可用（无背压时 200；有背压时 429 + `Retry-After`，非 5xx）
