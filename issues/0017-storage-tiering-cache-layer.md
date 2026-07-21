---
id: "0017"
title: "存储分层与通用缓存层（P1 学习扩展）"
status: complete
labels: ["complete", "p1"]
parent: "0010"
blocked_by: []
created_at: 2026-07-21
---

# 0017 — 存储分层与通用缓存层（P1 学习扩展）

## Parent

Phase 3: 规模化与工程质量（P1 学习扩展），对应 Spec 11。

> **本票属于 P1 学习扩展，不在 Phase 3 的 P0 紧急范围内。P0 的 0016+0018+0019+0020 完成后再启动。**

## What to build

在当前 Redis 仅用于网关路由的基础上，抽象通用 Cache 接口层，系统化落地 Spec 11 的缓存策略：对 conversation_summary、device list、限流计数等热点数据建立分层缓存；实现穿透防护（NULL object 缓存）、击穿防护（Redis SET NX 互斥锁）、雪崩防护（TTL 随机抖动）；分片键选择文档化（代码注释 + 设计文档），即使当前不拆库也要让分片假设显式可见。

端到端行为：当大量客户端同时查询同一个 conversation_summary 时 → 第一次查询从 DB 加载并写入 Redis → 后续查询命中 Redis 缓存（5min TTL）→ 如果 Redis 不可用则自动回源 DB → 当 conversation_summary 被更新时，缓存被主动失效而非等 TTL 自然过期 → 运维可以从缓存命中率 metrics 中看到各级缓存的效率。

## Acceptance criteria

- [ ] `internal/cache/` 包：通用 Cache 接口 `{Get(ctx, key) ([]byte, error), Set(ctx, key, value, ttl) error, Del(ctx, keys...) error, Exists(ctx, key) (bool, error)}`
- [ ] Redis 实现 + 本地内存实现（用于测试）+ Noop 实现（用于降级）
- [ ] Cache-Aside 模式封装：`cache.GetOrLoad(ctx, key, ttl, loader func() (T, error))`
- [ ] 穿透防护：对于 DB 中不存在的 key，缓存空值标记（`__NULL__`），TTL 30s
- [ ] 击穿防护：`GetOrLoad` 内部对同一 key 的并发回源用 `SET NX lock:{key} EX 5` 互斥
- [ ] 雪崩防护：TTL 加随机抖动 `±20%`（`actualTTL = baseTTL * (0.8 + 0.4 * rand.Float64())`）
- [ ] 缓存指标：命中率 counter `cache_hit_total` / `cache_miss_total`（Prometheus）
- [ ] `conversation_summaries` 查询接入缓存层：`conv_summary:{user_id}:{conversation_id}`，5min TTL
- [ ] `device_sessions` 查询接入缓存层：`devices:{user_id}`，1min TTL
- [ ] conversation_summary 更新时主动删除对应缓存 key（write-through/invalidate）
- [ ] 分片键选择文档 `docs/adr/0002-shard-key-selection.md`：记录 messages/sync_events/conversation_summaries/outbox_events/group_members 的分片键选择理由、与查询模式的对齐、与 Spec 11 §4.1 的对照
- [ ] 所有分片键常量出现在对应代码包中（如 `messages.ShardKey = "conversation_id"`），防止未来业务代码乱用分片键

## Blocked by

None — can start immediately（P1，可在任何时候启动）。

## 技术难点与注意事项

### 1. 超卖与数据一致性

**问题：** 先写 DB，再删缓存（Cache Invalidation）。如果"查缓存→miss→读DB→写缓存"和"写DB→删缓存"并发执行，可能出现旧数据写入缓存。

**方案（P0）：**
```
Writer: DB.Write() → Cache.Del(key)
Reader: Cache.Get(key) 
        → if miss: DB.Read() → Cache.Set(key, value, ttl)  // 可能写入旧值
        → if hit: return
```

这个模式在极少数的竞态条件下可能返回短暂过时的数据。对 conversation_summary（5min TTL）来说，可接受。严格一致性需要分布式锁或 CDC，P0 不追求。

**另一种更简单的方案：** 直接 `SET EX` 短 TTL（如 1min），不主动失效。写入方不做任何缓存操作，读方等 TTL 自然过期。这是最安全的——只是"新数据可见"延迟最多 1min。

### 2. 缓存 key 的命名规范

**问题：** 不同模块随意命名 key 会导致冲突和难以追踪。

**方案：** 统一命名规范：`{domain}:{primary_key}:{sub_key}`。如：
- `conv_summary:1:conv_abc`（user_id=1, conversation_id=conv_abc）
- `device:1`（user_id=1）
- `rate_limit:request_code:192.168.1.1`

所有 key prefix 定义为常量（如 `cache.PrefixConvSummary = "conv_summary"`），禁止硬编码。

### 3. Redis 不可用时的降级

**问题：** Redis 故障时，所有 `Get` 返回 miss、`Set` 静默失败。请求全部穿透到 DB。

**方案：**
- `RedisCache` 内部捕获 Redis 连接错误，记录 `cache_degraded` 指标，返回 miss（不 panic）
- 业务代码不需要写降级逻辑——`GetOrLoad` 自动回源 DB
- 增加熔断可选逻辑：Redis 连续失败 N 次后，暂时跳过 Redis 查询（直接走 DB），后台 goroutine 定期探测 Redis 是否恢复

### 4. 涉及的关键文件

- `internal/cache/cache.go` — 接口定义
- `internal/cache/redis.go` — Redis 实现
- `internal/cache/memory.go` — 内存实现（测试用）
- `internal/cache/noop.go` — Noop 实现（降级用）
- `internal/cache/metrics.go` — 命中率指标
- `internal/conversations/service.go` — 接入缓存
- `internal/group/service.go` — 设备列表缓存（如 0011 未做）
- `docs/adr/0002-shard-key-selection.md` — 分片键选择文档
