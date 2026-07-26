# 03 — 重连风暴

## 1. 问题是什么

大量设备几乎同时 WebSocket 握手（网关重启、网络抖动恢复）。

```
无 jitter：握手尖峰 ≈ C 设备 / 1s
有 ±J ms jitter：峰值被摊开，但仍可能打穿限流与 Redis 路由写
```

## 2. 业界常见方案

- 客户端指数退避 + jitter  
- 接入层按 IP/user 令牌桶  
- 连接 shed / 排队

## 3. 本仓现状

| 项 | 状态 | 锚点 |
|----|------|------|
| 退避算法 | Implemented（服务端库） | `internal/gateway/reconnect.go` |
| IP/user 连接限流 | Implemented | `internal/gateway/ratelimit.go` |
| Python 场景 | Partial | `reconnect_storm.py`（有 jitter 参数） |
| iOS 接入退避 | Missing | 见 0033 / 后续实现票 |

## 4. 如何模拟

```bash
cd load_test
# 有 jitter（默认）
python run.py --scenario reconnect_storm --concurrency 50 --duration 20 --jitter-ms 500

# 对比：关闭 jitter，观察失败/限流更尖
python run.py --scenario reconnect_storm --concurrency 50 --duration 20 --no-jitter
```

## 5. 观察什么

| 信号 | 预期 |
|------|------|
| Gateway 429 / 拒绝升级 | 超限时出现 |
| 握手成功率 | 有 jitter 通常更高更平滑 |
| Redis 路由写 QPS | 风暴窗口内尖峰 |
| `:8081/metrics` | 连接相关计数抖动 |

## 6. 通过标准

- 限流生效时系统可恢复，非持续 5xx 雪崩  
- 有/无 jitter 对比能看出峰值差异（学习结论写入复盘）  
- health-check 在稳态后通过  
