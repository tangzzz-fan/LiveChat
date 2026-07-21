# 缓存三大坑：穿透、击穿、雪崩的场景与对策

标签: `scale`, `observability`

## 问题是什么

缓存是提升读性能的利器，但三个经典问题会让缓存不仅没用，反而雪上加霜：

1. **缓存穿透**：请求不存在的 key → 每次都打到 DB
2. **缓存击穿**：热点 key 在过期瞬间 → 大量并发请求同时打到 DB
3. **缓存雪崩**：大量 key 同时过期 → DB 瞬间压力过载

这三种场景在聊天系统中特别危险——大量用户同时刷新会话列表、同一个热门群的 summary 被频繁查询。

## 典型场景

### 穿透

```
攻击者构造大量不存在的 conversation_id → 每次查缓存 miss → 每次查 DB
→ SELECT * FROM conversation_summaries WHERE conversation_id = 'fake-id'
→ 返回 0 行 → 缓存不存储 → 下次请求继续 miss → DB 被打满
```

### 击穿

```
热点群 "校友群" summary 被 100 个用户同时查询
→ Redis key "conv_summary:1001:conv_grp_123" 刚好过期
→ 100 个请求同时发现 cache miss
→ 100 个请求同时去 DB 查
→ DB 连接池耗尽
```

### 雪崩

```
所有 conversation_summary 的 TTL 都设为 5 分钟
→ 第 5 分钟时全部 key 同时过期
→ 接下来 1 秒内所有请求都 miss → 全部打到 DB
→ 如果 DB 刚好因为击穿在处理热点 key → 叠加 → 雪崩
```

## 通用分析思路

1. **穿透**：问题的根因是"不存在的 key 不会被缓存"。解决：缓存空值（NULL object），短 TTL。
2. **击穿**：问题的根因是"并发 miss 没有协调"。解决：互斥锁（SET NX），只允许一个请求回源。
3. **雪崩**：问题的根因是"大量 key 同时过期"。解决：TTL 加随机抖动。

**统一原则：先写 DB，再（删）缓存**。缓存不可作为写入的权威源。

## 当前项目方案

LiveChat 在 0017（P1）中建立了通用缓存层，内置三种防护：

### 穿透防护：NULL Object Cache

```go
// cache.GetOrLoad 对于 DB 中不存在的 key，缓存空值标记
key := "conv_summary:1:nonexistent-conv"
val := cache.Get(ctx, key)
if val == "__NULL__" {
    return nil, ErrNotFound  // 缓存命中 NULL → 直接返回，不查 DB
}

loader := func() (string, error) {
    result, err := db.Query("SELECT ... WHERE conversation_id = $1")
    if err == sql.ErrNoRows {
        cache.Set(ctx, key, "__NULL__", 30*time.Second)  // NULL 对象 TTL 短
        return "", ErrNotFound
    }
    return result, nil
}
```

### 击穿防护：互斥锁

```go
func (s *Store) GetOrLoad(ctx, key, ttl, loader) ([]byte, error) {
    val, err := s.Get(ctx, key)
    if err == nil { return val, nil }

    // 尝试获取锁
    lockKey := "lock:" + key
    ok := s.client.SetNX(ctx, lockKey, "1", 5*time.Second).Val()
    if !ok {
        // 其他请求正在回源 → 等待并重试
        time.Sleep(100 * time.Millisecond)
        return s.GetOrLoad(ctx, key, ttl, loader)
    }
    defer s.client.Del(ctx, lockKey)

    // Double-check：拿到锁后再查一次
    val, err = s.Get(ctx, key)
    if err == nil { return val, nil }

    // 回源
    val, err = loader()
    s.Set(ctx, key, val, ttl)
    return val, err
}
```

### 雪崩防护：TTL 随机抖动

```go
func jitteredTTL(baseTTL time.Duration) time.Duration {
    // ±20% 随机抖动
    jitter := time.Duration(float64(baseTTL) * 0.2 * (rand.Float64()*2 - 1))
    return baseTTL + jitter
}

// 原来：所有 key 的 TTL 都精确为 5 分钟
// 现在：实际 TTL 在 4-6 分钟之间随机分布
// → 同一时间过期的 key 数量大幅减少
```

### 缓存指标

```
cache_hit_total  — 缓存命中次数
cache_miss_total — 缓存未命中次数
命中率 = hit / (hit + miss)  → 目标 > 80%
```

## 替代方案及取舍

| 防护 | 方案 A | 方案 B | LiveChat 选择 |
|------|--------|--------|--------------|
| 穿透 | NULL Object (30s TTL) | Bloom Filter 预判断 | NULL Object（简单，够用） |
| 击穿 | SET NX 互斥锁 | 永不过期 + 异步刷新 | SET NX（标准方案） |
| 雪崩 | TTL 随机抖动 | 多级缓存 (L1 local + L2 Redis) | TTL 抖动（P0 简捷） |

## 踩坑记录

1. **NULL Object 的 TTL 必须比正常缓存短**：正常 key 可能 5min TTL，NULL 值只能 30s——否则如果后来真的创建了这个 key，需要等 5min 才能被缓存。
2. **SET NX 锁要设过期时间**：如果持有锁的请求因 OOM/panic 挂掉而没删锁，其他请求永远获取不到锁。`SET NX lock:{key} EX 5` 确保锁自愈。
3. **Double-check 很重要**：拿到锁后再查一次缓存——可能在你等锁的时候，前一个请求已经回源并写入了缓存。

### 代码位置

- `internal/cache/cache.go` → Store 接口
- `internal/cache/redis.go` → RedisStore（P1，含 bloom/nx/jitter 预留接口）
- `internal/cache/memory.go` → MemoryStore（测试用）
- `internal/cache/noop.go` → NoopStore（降级用）
- 中群 fanout 不更新 summary → 实际上是一种"架构级缓存优化"
