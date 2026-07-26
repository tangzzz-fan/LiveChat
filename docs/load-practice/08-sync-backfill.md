# 08 — 离线 sync 积压 / 回补

## 1. 问题是什么

设备游标落后很多：`GET /v1/sync/events` 需多页拉取。洪流会打满客户端主线程与服务端读路径。

```
回补成本 ≈ (server_seq - device_cursor) / page_size 次请求
```

## 2. 业界常见方案

- 分页 + `has_more`  
- 服务端单次上限  
- 客户端分批 apply、首屏优先（见 0033）

## 3. 本仓现状

| 项 | 状态 | 锚点 |
|----|------|------|
| Sync API + 游标 | Implemented | `internal/sync` |
| 分页上限 | Implemented | sync 限页 |
| 压测场景 | **Hardened（0031）** | `sync_backfill.py` 灌消息后拉 cursor=0 |
| iOS SyncExecutor | Missing | 0024 / 0033 |

## 4. 如何模拟

**当前（0031 前）— 手工：**

```bash
# 1) 用 A 设备狂发或 group_fanout 灌消息
cd load_test && python run.py --scenario send_message --concurrency 20 --duration 30

# 2) B 设备用落后 cursor 拉 sync（需有效 JWT）
curl -s -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/v1/sync/events?after_seq=0&limit=100"
```

**目标命令（0031 后）：**

```bash
cd load_test
python run.py --scenario sync_backfill --concurrency 20 --duration 30
```

## 5. 观察什么

| 信号 | 预期 |
|------|------|
| 响应 `has_more` | 落后大时为 true |
| 延迟 / 页耗时 | 随积压增大 |
| 服务端 CPU / DB | 读放大 |
| 客户端（若接 iOS） | 主线程是否卡顿 |

## 6. 通过标准

- 游标单调推进，不丢不重（幂等去重）  
- 0031 后 `--quick` 含 sync_backfill 绿  
- 学习结论写入基线报告  
