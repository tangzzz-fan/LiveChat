# LiveChat Load Test Framework

基于 Python asyncio 的压测框架，覆盖 5 个核心场景。

## 依赖安装

```bash
pip install httpx websockets rich
# 或
pip install -r requirements.txt
```

## 快速开始

```bash
# Sanity check（10 并发，10 秒）
python run.py --scenario send_message --concurrency 10 --duration 10 --quick

# 完整压测
python run.py --scenario send_message --concurrency 100 --duration 60

# 所有场景
python run.py --all --concurrency 50 --duration 30

# 输出 JSON 报告
python run.py --scenario connect --output json
```

## 场景

| 场景 | 说明 | 需要 Phase |
|------|------|-----------|
| `send_message` | 文本消息发送压测 | Phase 1 |
| `connect` | 登录 + WebSocket 连接建立 | Phase 1 |
| `group_fanout` | 群消息扇出压测（200 人群） | Phase 2 |
| `sync_backfill` | 离线同步回补 | Phase 1 |
| `reconnect_storm` | 重连风暴 | Phase 1 |

## 基线报告

首次完整压测结果保存在 `baselines/` 目录，后续压测可与基线 diff 对比。

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
--quick         CI 模式：10 并发 10 秒
```
