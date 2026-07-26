# 06 — DB 暂停 / 主库不可用

## 1. 问题是什么

PostgreSQL 不可用时，发送与 sync 写入全部失败。学习环境用 pause/stop 模拟，验证失败语义与恢复后一致性，而非真 HA failover。

## 2. 业界常见方案

- 主从切换 / 连接池重试  
- 客户端本地队列（iOS local-first，见 0033）  
- 明确 5xx / 超时，避免双写脑裂

## 3. 本仓现状

| 项 | 状态 | 锚点 |
|----|------|------|
| 演练手册 | Implemented | `docs/chaos/03-db-primary-failover.md` |
| 注入脚本 | Implemented | `db-pause.sh` / `db-resume.sh` |
| 真 HA | Missing | 单机学习环境不做 |

## 4. 如何模拟

```bash
export CHAT_ENV=dev
bash livechat-server/scripts/chaos/db-pause.sh

# send / sync 应失败或超时
curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8080/health || true

bash livechat-server/scripts/chaos/db-resume.sh
bash livechat-server/scripts/chaos/health-check.sh
```

## 5. 观察什么

| 信号 | 预期 |
|------|------|
| HTTP 5xx / 超时 | 写入路径失败 |
| 进程是否崩溃 | 应可恢复，不要求完美优雅 |
| resume 后 | 新写入成功；旧 in-flight 需幂等 |

## 6. 通过标准

- 符合 chaos 03 验收条目  
- resume 后可继续发消息  
- 不把「单机 stop postgres」误写成生产 failover 已验证  
