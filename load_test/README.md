# LiveChat Load Test Framework

基于 Python asyncio 的压测框架，覆盖 6 个核心场景。

## 依赖安装

系统 Python 通常没有 httpx，用虚拟环境：

```bash
cd load_test
python3 -m venv .venv
.venv/bin/pip install -r requirements.txt
```

## 生成 WebSocket protobuf 绑定

WebSocket 场景需要真实的 protobuf 帧，schema 唯一来源是 `livechat-server/proto/`：

```bash
./gen_proto.sh          # 或在 livechat-server/ 执行 make loadtest-proto
```

生成物在 `core/gen/`（不入库）。**协议变更后必须重跑**，否则压测客户端会静默失效——曾经的 JSON 占位握手让网关直接回 `expected HANDSHAKE_REQ`，而场景只统计了 upgrade，问题一直没被发现（issue 0034）。

## 前置：服务怎么起

先 `make build` 再跑二进制。`go run` 启动的进程会被终端会话回收，且进程名是临时文件，chaos 脚本的 `pgrep -x` 匹配不到。

```bash
cd ../livechat-server && make build
./message-service & ./gateway & ./outbox-consumer &
```

## 快速开始

```bash
# Sanity check（10 并发，10 秒）
.venv/bin/python run.py --scenario send_message --quick

# 完整压测
.venv/bin/python run.py --scenario send_message --concurrency 100 --duration 60

# 所有场景
.venv/bin/python run.py --all --concurrency 50 --duration 30

# 输出 JSON 报告
.venv/bin/python run.py --scenario connect --output json
```

## 场景

| 场景 | 说明 | 关键产出 |
|------|------|----------|
| `send_message` | 文本消息发送（走 `/v1/conversations/direct`） | 写路径吞吐与延迟 |
| `connect` | WebSocket 建连 | 三态计数：upgrade 被限流 / 握手被拒 / 握手成功 |
| `realtime_delivery` | A 发 → B 的 WS 收到 `MESSAGE_DELIVERY` | **端到端投递率与延迟** |
| `group_fanout` | 群消息扇出（`--max-members` 可调） | 写扩散放大 |
| `sync_backfill` | 离线同步回补 | 分页拉取延迟 |
| `reconnect_storm` | 重连风暴（jitter 对照） | 恢复成功率 |

### 读结果时的两个坑

- **`connect` 的高"限流比例"不是故障**。Gateway 限每 IP 5 conn/s，单机压测必然大部分被拒，已单独计入 `upgrade_throttled` 而非 `Errors`。
- **`reconnect_storm` 的 `rps/P50` 不是业务吞吐**。风暴在 `setup()` 里一次性测量，`execute()` 是空转。

## 故障演练脚本

```bash
.venv/bin/python drills/chaos01_redis_outage.py
```

自动完成 Redis 注入与恢复，用真握手连接验证「握手成功但投递收不到」的静默降级。对应 [chaos 01](../docs/chaos/01-redis-outage.md)。

## 基线报告

- 实测基线：[`baselines/local-measured-baseline.md`](baselines/local-measured-baseline.md)
- 差距说明：[`baselines/concurrency-gap-baseline.md`](baselines/concurrency-gap-baseline.md)
- 每次运行还会落一份带时间戳的 `baselines/baseline-*.md`（已 gitignore）

## 参数

```
--base-url      消息服务地址（默认 http://localhost:8080）
--ws-url        网关 WebSocket 地址（默认 ws://localhost:8081/ws）
--concurrency   并发虚拟用户数
--duration      压测持续时长（秒）
--scenario      压测场景
--all           运行所有场景
--output        输出格式（markdown/json）
--jitter-ms     重连风暴抖动（ms，默认 500）
--no-jitter     重连风暴不添加抖动
--max-members   group_fanout 成员数上限（默认 50）
--quick         CI 模式：10 并发 10 秒
```
