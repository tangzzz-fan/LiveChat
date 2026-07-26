# iOS 高负载 / 弱网客户端方案与坑点

> Issue [0033](../issues/0033-ios-high-load-client-design.md) · 计划 [`高负载IM验证计划.md`](./高负载IM验证计划.md) · Spec 13  
> **本文只做设计与坑点**；不写 Swift 实现。服务端容量打压用 Python `load_test/`，不用 iOS 模拟器集群。

## 定位

| 角色 | 说明 |
|------|------|
| 负载源 | `load_test/` |
| iOS | 高负载下的**被测客户端**：大会话、突发投递、弱网、后台、写风暴时不卡 UI、不丢消息、不爆内存 |
| 正确性票 | `0022`–`0025`；本文横切验收项可并入这些票或后续实现票 |

原则对齐 Spec 13：GRDB 为本地单一可信源；远程事件统一入口；Presentation 只听投影。

---

## 子问题清单

### 1. 突发投递刷新风暴

**触发**：WS 短时间成百条 `MESSAGE_DELIVERY`。

**方案**：事件入队 → 批量事务落库 → UI 用 GRDB `ValueObservation` **去抖**（约 16–33ms 合并窗口），禁止每条 `reloadData`。

**坑点**：每条消息一次 UI diff；observation 无节流导致主线程风暴。

### 2. 大会话滚动

**触发**：会话数万条历史。

**方案**：按 `conversation_seq` 游标分页；`List`/`UITableView` 懒加载；保留 `(conversation_id, conversation_seq)` 索引（骨架已有，勿删）。

**坑点**：`SELECT *` 全表；SwiftUI `ForEach` 全量；无复合索引。

### 3. 发送队列写风暴

**触发**：狂发 / 失败重试堆积。

**方案**：`actor MessageSendExecutor`（Spec 13 §5.2）单串行 + **有界队列** + `client_message_id` 幂等；重试指数退避。

**坑点**：多入口并发写同一行；无界队列吃内存；无退避形成本地风暴。

### 4. GRDB 写并发

**触发**：sync 批量落库 + 发送 + 已读同时写。

**方案**：单一 `DatabaseQueue` + WAL；写操作批量事务；禁止主线程同步重查询。

**坑点**：多 writer；逐条 fsync；主线程卡在 DB。

### 5. 弱网 / 断网

**触发**：地铁、切网、丢包。

**方案**：先写本地 `queued`；`NWPathMonitor`；恢复后续跑队列（Spec 13 §8.3）；`sending` 需超时 → `failed`/`queued`。

**坑点**：路径抖动反复重连；`sending` 卡死；乐观 UI 不收敛。

### 6. 客户端重连风暴

**触发**：回前台 / 服务端踢连 / 网关 kill。

**方案**：指数退避 + jitter（对齐服务端 `reconnect.go` 语义）；**single-flight** 建连；区分不可重连错误。

**坑点**：立即重连打穿 Gateway 429；多 Task 并发建连。

### 7. 后台唤醒预算

**触发**：Silent Push / BGTask。

**方案**：后台只触发增量 sync；心跳降至 120s（Spec 13 §8.2）；限时完成。Push payload **不是**消息真相源。

**坑点**：超时被系统杀；后台狂 sync 耗电。

### 8. sync 追赶洪流

**触发**：离线久、`has_more` 连拉。

**方案**：分批 apply + 让出主线程；cursor **单调**推进；首屏优先可见会话。

**坑点**：递归 sync 无让步；cursor 提前推进丢事件；全量 apply 卡首屏。

### 9. 内存 / 图片

**触发**：缩略图滚动。

**方案**：缩略图优先；磁盘/内存双层缓存；离屏取消加载。

**坑点**：原图直载 OOM；`Data(contentsOf:)` 阻塞；无取消积压。

### 10. 时钟与顺序

**触发**：多端乱序到达。

**方案**：渲染一律按 `conversation_seq`；`server_message_id` / `client_message_id` 去重。

**坑点**：用 `created_at` 排序乱序；缺去重重复气泡。

---

## 本地复现（供实现票）

```bash
# 灌历史：向会话打 1k+ 消息后，iOS 冷启动 sync
cd load_test && python run.py --scenario send_message --concurrency 20 --duration 60

# 热点群 + 前台观察刷新
python run.py --scenario group_fanout --concurrency 30 --duration 30
```

- Instruments：Time Profiler / Allocations / Core Animation  
- Network Link Conditioner：高延迟 / 100% loss → 验证队列与重连  

## 建议的横切验收项（实现时）

- [ ] 首屏约 1k 条历史：主线程无明显长时间卡顿（Instruments 可说明）  
- [ ] 弱网发送：本地 `queued` → 恢复后可达，不丢幂等键  
- [ ] 突发投递：列表不掉帧级卡顿（去抖生效）  
- [ ] 重连：不并发建多条 WS；退避可见  

## 明确不做

- 用 iOS 模拟器集群打服务端容量  
- 本文范围内实现 Swift 业务代码  

## 文档导航

- Spec 13 · [`iOS多端接入评估与实现.md`](./iOS多端接入评估与实现.md) · [`load-practice/`](./load-practice/) · `ios/README.md`  
