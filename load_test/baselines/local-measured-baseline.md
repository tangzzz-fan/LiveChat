# 本机实测基线占位（0031）

> 在 message-service / gateway / outbox-consumer 已启动后执行：
>
> ```bash
> cd load_test
> python run.py --quick --all --output markdown
> ```
>
> 将生成的 `baselines/baseline-*.md` 关键本文件，或在下表填入数字。

| 场景 | concurrency | duration | reqs | rps | P50 ms | P95 ms | err% | 备注 |
|------|-------------|----------|------|-----|--------|--------|------|------|
| send_message | 10 | 10s | — | — | — | — | — | 群 API 建会话 |
| connect | 10 | 10s | — | — | — | — | — | 真 WS upgrade |
| group_fanout | 10 | 10s | — | — | — | — | — | `--max-members` 默认 50 |
| sync_backfill | 10 | 10s | — | — | — | — | — | 先灌消息再拉 cursor=0 |
| reconnect_storm | 10 | 10s | — | — | — | — | — | 默认 jitter 500ms |

机器：_（填写 OS / CPU）_  
git SHA：_（填写）_  
日期：_

## 与差距说明文档

概念差距见 [concurrency-gap-baseline.md](./concurrency-gap-baseline.md)。本文件用于**实测数字**，不以 Spec 01 峰值为门禁。
