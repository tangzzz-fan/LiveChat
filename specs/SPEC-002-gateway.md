# SPEC-002 — 接入网关与长连接（C100K 课题）

> 状态: Draft | Milestone: M1 | 依赖: SPEC-001 | 被依赖: 003, 005, 007

## 1. 背景与动机（Why）

这是整个项目"高并发"二字的主战场。目标：一个 Go 进程在开发机上稳定持有
**5w~10w 条 WebSocket 长连接**，且架构上加机器就能线性扩容。
WhatsApp 的传奇数字是单机 200w 连接（FreeBSD + Erlang, 2012），我们不追数字，
但要把它背后的每个问题亲手踩一遍。

## 2. 核心挑战与典型解法

### 挑战 A：单机 10w 连接，资源花在哪？

| 资源 | 天真做法的坟场 | 解法 |
|------|---------------|------|
| 文件描述符 | 默认 `ulimit -n 256`(macOS)/1024(Linux)，几百连接就挂 | `rlimit` 调到 1,048,576；macOS 另需 `kern.maxfiles` |
| 内存 | 每连接两个 goroutine，初始栈 ~4KB×2 + 读写缓冲 | 预算制：每连接目标 < 20KB；缓冲区用 `sync.Pool` 复用 |
| 调度 | 天真 thread-per-conn 是 C10K 的死法 | Go runtime 已把 epoll/kqueue 藏在 netpoller 里，goroutine-per-conn 就是正解——这是选 Go 的核心理由 |
| TLS | 10w 连接同时握手, CPU 打满 | 握手是一次性成本；重连风暴才是真问题（见挑战 C） |

**架构决定：goroutine-per-connection（每连接 1 读 goroutine + 1 写 goroutine），
不用 evio/gnet 类 event-loop 库。** 理由：10w 级别下 goroutine 模型的简洁性
完胜 event-loop 的复杂度，100w+ 才需要重新评估——把这个权衡本身写进学习笔记。

### 挑战 B：慢客户端与背压（backpressure）

2G 网络上的手机消费很慢，服务端如果无界缓冲它的下行消息 → OOM；
如果同步阻塞写 → 一个慢客户端拖死整个扇出循环。

**解法（写路径三件套）：**
1. 每连接一个**有界** send channel（如 256 帧）；
2. 投递用非阻塞写：`select { case ch <- frame: default: /* 溢出 */ }`；
3. 溢出策略 = **断开连接**。听起来粗暴，其实是 IM 的标准答案：连接断了
   客户端会重连并走 SyncRequest 补拉（SPEC-003），消息在收件箱里一条不丢。
   "连接可以死，数据不能丢"——把可靠性从连接层挪到同步层，这是本项目最重要的
   一个架构观念。
   （M4 升级点：SPEC-013 引入帧分级——StreamChunk 等可丢弃帧在高水位时
   直接丢弃不断连，只有不可丢弃帧溢出才走断连规则。M1 实现时把"帧能否
   丢弃"留作帧属性位，调度器就地可扩展。）

### 挑战 C：心跳与重连风暴

- **心跳**：NAT 网关和运营商会静默掐死空闲 TCP（常见 60~300s）。客户端每 25s
  发 Ping（低于常见 NAT 超时下限 30s）；服务端 75s（3 个周期）没收到任何帧判死。
  **心跳时间加 ±20% 随机抖动**，否则 5w 个同时启动的压测客户端会产生完美同步的
  心跳尖峰（thundering herd 的第一次亲密接触）。
- **重连风暴**：网关重启瞬间 5w 客户端同时重连 = 自己 DDoS 自己。客户端侧
  指数退避 1s→2s→4s→…→60s 封顶，**每级加全幅抖动（full jitter）**；服务端侧
  对新建连接限速（令牌桶，如 5000 conn/s），超出的直接拒绝让客户端退避。

### 挑战 D：网关必须无状态（水平扩展的前提）

连接天然有状态（就在这台机器的内存里），但**业务不能依赖这一点**：

- 连接注册表：本地 `map[userID]*Conn`（分片 256 个 map + 独立锁，避免全局锁争用）
  + Redis 全局路由表 `route:{user_id} → gateway_id`（TTL 续期）。
- Message Service 要推消息给某用户：查 Redis 路由 → gRPC 调对应网关 → 网关写本地连接。
  路由不存在 = 离线，走收件箱 + 推送（SPEC-008）。
- **优雅下线**：收到 SIGTERM → 摘掉 LB 流量 → 向所有连接发 GoAway 帧（带
  "请立即重连"语义）→ 分批（如每 100ms 关 1%）关闭 → 兜底超时硬关。
  分批就是为了不制造自己的重连风暴。

### 挑战 E：鉴权

WebSocket 升级请求带 JWT（HTTP header）。**连接建立 ≠ 鉴权通过**：升级后
5s 内必须收到合法 AuthRequest，否则掐掉——防止匿名连接耗尽 FD（最便宜的 DoS）。

## 3. 详细设计要点

```
Listener (TLS) ──► upgrade ──► Conn{
    readLoop  goroutine: 解帧 → 鉴权态检查 → 分发(ping→pong / send_msg→gRPC msgsvc / ...)
    writeLoop goroutine: for frame := range sendCh(cap 256) → ws.Write
    state: userID, deviceID, lastSeenAt (atomic)
}
ConnRegistry: [256]shard{ mu sync.RWMutex; m map[string]*Conn }
RouteKeeper:  每 30s 续期 Redis route:{uid}, TTL 90s
gRPC server:  PushToUser(uid, frames) — 供 msgsvc 反向推送
```

Linux 容器内核参数（deploy/ 提供）：`fs.file-max`、`net.core.somaxconn=4096`、
`net.ipv4.tcp_max_syn_backlog`；压测机侧 `ip_local_port_range` + 多源 IP
（详见 SPEC-005，单源 IP 只有 ~28k 临时端口——每个压测者必踩的坑）。

**服务配置与发现（评审 A6，适用全部 Go 服务）**：本项目规模不引入注册中心，
全部走 **docker-compose DNS（服务名即主机名）+ 环境变量**，十二要素风格：

| 变量 | 示例 | 使用方 |
|------|------|--------|
| `MSGSVC_ADDR` | `msgsvc:9090` | gateway, api |
| `GATEWAY_GRPC_ADDR` | `0.0.0.0:9091`（自身监听，注册进 Redis 路由表供反向推送） | gateway |
| `REDIS_ADDR` / `DB_DSN` | `redis:6379` / `postgres://...` | 全部 |
| `APNS_*`（key 路径/team/bundle） | — | pushworker |
| `LISTEN_WS` / `LISTEN_HTTP` | `0.0.0.0:8080` | gateway / api |

约定：所有服务启动时打印生效配置（密钥脱敏）；缺必填变量 = 启动失败快速
退出，不给默认值（fail-fast 优于半死状态）。msgsvc → gateway 的反向推送
不走 DNS：路由表里存的 `gateway_id` 即该实例的 gRPC 地址（自注册）。

## 4. 范围

**In**：上述全部 + Prometheus 指标埋点（连接数、goroutine 数、sendCh 溢出
计数、心跳超时计数）。
**Out**：消息语义（003）、状态广播（007）、压测客户端本身（005）。

## 5. 验收标准（全部量化）

1. 开发机（或 Linux 容器）单网关进程持有 **50,000** 条活跃连接（含心跳）
   稳定 30 分钟：RSS < 2GB，goroutine 数 ≈ 2×连接数 + O(1)，无泄漏趋势。
2. 慢客户端实验：1 个消费为 0 的客户端 + 对它持续注入下行 → sendCh 溢出 →
   该连接被断开，**其余连接的 p99 延迟无劣化**（对照数据证明）。
3. 重连风暴实验：`kill -9` 网关，5w 客户端按退避重连，恢复期间新网关 CPU
   不打满、连接限速器丢弃计数 > 0（证明限速真的在工作）、全部客户端 90s 内重连成功。
4. 优雅下线实验：SIGTERM 后所有客户端收到 GoAway 并迁移到另一网关，重连尖峰
   被摊平（连接建立速率曲线为证）。
5. docker-compose 起 2 个网关 + LB，客户端随机落到两台，路由表正确反映归属。

## 6. 测试计划

单测（分片注册表并发正确性、鉴权超时）；集成测试用 loadtest 工具的小规模
模式（1k 连接）跑在 CI；上述验收实验为手动 runbook，结果记录进
`docs/experiments/`。
