# 原生 WebSocketTask 生命周期与 AsyncStream 单消费者陷阱

标签: `connection`

## 问题是什么

`URLSessionWebSocketTask` 与包装它的 `AsyncStream` **不能当成可任意重入的单例管道**：Task 实例、事件流订阅、读循环若只创建一次却期望多次重连，会出现「连上了却收不到帧 / 第二次 subscribe 永远静默 / 握手响应被先消费掉」。

## 典型场景

1. **Transport 单例 + `close()` 后 `continuation.finish()`**  
   流已结束，上层还对同一 `events` 做 `for await` → 收不到重连后的帧。

2. **握手循环与业务读循环争抢同一 `AsyncStream`**  
   握手 `for await` 读走了 `HANDSHAKE_RESP`，再开第二个 reader → 后续投递丢失或第二个消费者根本接不上（`AsyncStream` 通常**单消费者**）。

3. **Middleware 用固定 `CancellationID` 反复 `runTask` 订阅**  
   TGReduxKit 同 id 会 **cancel 旧 Task**；旧循环取消后流已被部分消费，新循环订阅「半截流」→ 事件静默。

4. **原生断线检测偏弱**  
   仅依赖 Task 错误回调不够；需要应用层心跳 + 路径监控。

## 通用分析思路

对「长连接包装」逐项问：

1. **连接对象**能否复用？还是每次 dial 新建？  
2. **事件流**是热广播还是冷单播？单播则订阅只能一次。  
3. **谁读第一帧**？握手与业务必须同一 reader，或用 continuation 把握手从同一循环里分出。  
4. **取消语义**：UI/中间件 cancel 会不会误杀唯一订阅者？

## 当前项目方案

代码：`URLSessionWebSocketTransport`、`RealtimeSession`。

| 规则 | 实现 |
|------|------|
| 每次重连 **new** transport | `makeTransport(gatewayURL)`，不复用已 `finish` 的 stream |
| **单一读循环** | 先挂 `readerTask` 再 `connect` + 发握手；握手用 `CheckedContinuation` 从同一循环取出 |
| Realtime 事件订阅只绑一次 | `RealtimeListenGate.beginIfNeeded()`；`runTask` **不带** CancellationID |
| 启停与 id | `chat.realtime.start` / `stop` 用 CancellationID；与 listen 分离 |
| 后台 | `stop` 断开 WS；前台 `start` + sync |
| 断线兜底 | 协议心跳 + `NWPathMonitor` |
| 重连 | 指数退避 + jitter（对齐 `reconnect.go`）+ connect single-flight |

## 替代方案及取舍

| 方案 | 优点 | 代价 |
|------|------|------|
| 当前：单消费者 + Gate + 每连新 transport | 模型清晰，贴合 AsyncStream | 要纪律：禁止第二次 `for await events` |
| `AsyncChannel` / 多播 Subject | 多订阅者 | 多依赖或自管缓冲；仍要处理反压 |
| Starscream / NWConnection | 断线感知可能更好 | 额外依赖；上层仍要同一套会话状态机 |

## 踩坑记录

- 0041 初版：握手 `for await transport.events` 成功返回后再 `startReader` → **竞态丢帧**；改为「先 reader，后 connect/handshake」。  
- 同 id `chat.realtime.listen` 在 `syncTapped` / `realtimeEnsureStarted` 重复进入会取消唯一订阅 → 改为 Gate + 无 id 的长寿命 listen Task。  
- `NSLock` 不能在 `async` 函数里用（Swift 6）；transport 状态改到 `DispatchQueue.sync`。  
- Scaffold 时代 transport `connect` 只 `yield(.connected)`、不跑 receive loop —— 功能票必须补齐，否则「已连接」是假象。
