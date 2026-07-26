# 07 — Gateway kill

## 1. 问题是什么

单网关进程被杀：所有 WS 断开。客户端应重连；消息正确性依赖 Outbox + Sync，不依赖单条长连接。

## 2. 业界常见方案

- 多 Gateway + 无状态路由（Redis）  
- 客户端退避重连 + sync 补洞  
- 优雅摘流 vs kill（本演练偏后者）

## 3. 本仓现状

| 项 | 状态 | 锚点 |
|----|------|------|
| 演练 | Implemented | `docs/chaos/05-gateway-pod-failure.md` |
| 注入 | Implemented | `gateway-kill.sh` |
| 多节点 failover | Missing | 单机只能验证重连与 sync |

## 4. 如何模拟

```bash
export CHAT_ENV=dev
# 先建立若干 WS（可用 reconnect 或手工客户端）
bash livechat-server/scripts/chaos/gateway-kill.sh

# 重启 gateway
cd livechat-server && make run-gateway

# 客户端重连 + 必要时 sync
bash livechat-server/scripts/chaos/health-check.sh
```

可叠加：

```bash
cd load_test && python run.py --scenario reconnect_storm --quick
```

## 5. 观察什么

| 信号 | 预期 |
|------|------|
| WS 断开 | 立即 |
| kill 期间 send HTTP | 仍可走 message-service |
| 重连成功率 | 受限流与退避影响 |
| sync | 可补上断线窗口事件 |

## 6. 通过标准

- 杀网关不丢已 Accepted 消息  
- 重启后重连或 sync 可达  
- 明确记录：未验证跨节点导流  
