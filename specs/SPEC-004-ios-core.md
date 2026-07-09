# SPEC-004 — iOS 客户端核心：local-first 架构与同步引擎

> 状态: Draft | Milestone: M1 | 依赖: SPEC-001, 003 | 被依赖: 008, 009, 010, 011
> 栈: SwiftUI + GRDB + Swift Concurrency, iOS 17+

## 1. 背景与动机（Why）

聊天 App 的体验差距不在 UI，在**架构**：冷启动是否秒开（不等网络）、
弱网发消息是否即时上屏、切后台回来消息是否已就位。这些全部来自同一个原则：
**UI 只读本地数据库，网络只负责让本地库趋近服务端状态。**（local-first）

## 2. 核心挑战与典型解法

### 挑战 A：数据流向必须单向

天真做法：收到网络消息 → 直接更新 ViewModel → 顺便写库。结果：UI 状态和
库不一致、消息闪烁、重复渲染——所有自研 IM 客户端的经典坟场。

**解法：严格单向环**

```
WebSocket/Sync ──► SyncEngine(actor) ──写──► GRDB ──ValueObservation──► SwiftUI
     ▲                                                                    │
     └────────────── OutboxWorker ◄──写 outbox 表────── 用户点击发送 ◄──────┘
```

- UI 发消息 = 往本地库写一行（状态 pending）+ 立即上屏（乐观 UI），仅此而已；
- OutboxWorker 观察 outbox 表，负责真正发出去、处理 ACK、更新状态；
- 网络收到的一切（新消息、回执、sync 结果）都只写库，UI 靠 GRDB 的
  `ValueObservation` 自动刷新。UI 层没有任何网络代码。

### 挑战 B：连接状态机（移动网络是常态性断连的）

**解法：一个 `ConnectionManager` actor 持有显式状态机：**

```
disconnected ─connect─► connecting ─ws open─► authenticating ─auth ok─► ready
     ▲                      │每级退避+抖动          │                     │
     └──────失败/掐断────────┴────────────────────┴──── 心跳超时/写失败 ──┘
```

- 触发重连的信号：`NWPathMonitor` 网络恢复、App 进前台、心跳超时；
- 进入 `ready` 后第一件事永远是 `SyncRequest{since: last_synced_inbox_seq}`
  （SPEC-003 挑战 C 的客户端半边）；
- 后台/前台：iOS 切后台 ~30s 后 socket 必死（系统行为，不可对抗）。**不对抗**：
  后台直接主动断连省电，回前台重连 + sync。消息不丢由收件箱模型保证，
  后台期间的感知靠 APNs（SPEC-008）。

### 挑战 C：本地库 Schema 与 UI 性能

- GRDB 表：`conversations`（含冗余的 lastMessage 摘要与 unreadCount——列表页
  一次查询渲染，不做 N+1）、`messages`（主键 `(convId, convSeq)`，pending 消息
  convSeq 为 null 用 clientMsgId 排尾）、`outbox`、`syncState`（游标）。
- 会话页：倒序分页 50 条/页，`ScrollView` + `LazyVStack` 反转技巧；
  聊天气泡视图必须是纯函数 of 数据行（Equatable），滚动才不掉帧。
- 写合并：sync 批量落库在**单个 GRDB 事务**里完成，ValueObservation 只触发
  一次 UI 刷新（1000 条离线消息不能刷 1000 次）。

### 挑战 D：乐观发送的状态呈现

pending（转圈/单灰勾）→ sent（单勾, 收到 ACK 填入 convSeq）→ delivered（双勾）
→ failed（红色感叹号, 点击重发, 复用原 clientMsgId——幂等键在 UI 层的意义）。
重试预算：自动重试至连接恢复后 3 次，之后转 failed 等手动。

## 3. 模块划分

```
ios/LiveChat/
├── Core/Database/        # GRDB schema, migrations, DAO
├── Core/Network/         # WebSocketClient, ConnectionManager(actor), 编解码(SPEC-001 生成物)
├── Core/Sync/            # SyncEngine(actor), OutboxWorker
├── Features/ChatList/    # 会话列表 (纯读库)
├── Features/Conversation/# 会话页 (纯读库 + 写 outbox)
└── Features/Auth/        # 登录 (用户名密码, D8 决策)
```

## 4. 范围

**In**：上述全部 + 登录/token 管理 + 基础会话/消息 UI（不追求视觉打磨）。
**Out**：推送（008）、媒体（009）、多设备（010）、E2EE（011）、
消息全文搜索（GRDB FTS5 预留 virtual table，实现延后）。

## 5. 验收标准（真机实验）

1. **飞行模式实验**：飞行模式下连发 5 条 → 全部即时上屏为 pending → 关飞行
   模式 → 10s 内全部转 sent 且对端收到，顺序正确，无重复。
2. **冷启动**：杀进程重开，会话列表首帧渲染 < 500ms（本地库直出，无网络等待，
   Instruments 数据为证）。
3. **离线补齐**：对端在本机离线时发 1,000 条 → 本机启动 → 会话页就绪且
   无 UI 卡顿（掉帧 < 5%，Instruments）；ValueObservation 刷新次数 = O(页数)
   而非 O(消息数)。
4. **杀 App 不丢 pending**：发送瞬间杀进程 → 重开 → outbox 恢复重发成功。
5. 单测：状态机全部边覆盖；DAO 层迁移测试；SyncEngine 对乱序/重复推送的
   幂等落库测试。

## 6. 测试计划

Swift Testing 单测（状态机、DAO、SyncEngine 注入 mock transport）；
UI 冒烟用 XCUITest 跑登录→发消息→收消息 happy path；
验收 1~4 为手动 runbook 记录在 `docs/experiments/`。
