# SPEC 评审意见 — LiveChat 全 12 份规格文档

> 评审日期: 2026-07-09 | 评审范围: SPEC-000 ~ SPEC-011

## 总评

这是一份**学习目的驱动的 IM 架构文档**，整体质量显著高于大多数 side-project spec。12 份 spec 构成了一条完整的依赖链（001→002→003→004/005/006...），每份 spec 都有明确的"核心挑战 + 典型解法"结构，而且验收标准全部可量化、可实验。底层思想（推拉结合、收件箱模型、local-first、连接与业务分离）是业界验证过的正确答案，不是拍脑袋出来的。

但正因为质量高，更值得深挖。以下问题按严重程度组织：🔴 = 逻辑矛盾或正确性风险，🟡 = 模糊或遗漏，🟢 = 增强建议。


## 🔴 正确性与一致性风险

### C1. 收件箱游标：per-user 序列号 vs per-device 消费的矛盾（003 ↔ 010）

**问题**。SPEC-003 定义 `inbox_seq` 是 per-user 全局单调递增序列号，收件箱写入以 user 为单位。SPEC-010 把这个游标升级为 per-device——每台设备独立 ACK。但两处都没有回答：**iPhone ACK 到 100、iPad 才到 50 时，服务端如何处理 inbox 条目的生命周期？**

- 不能删——iPad 还没消费。
- 不能无限保留——收件箱无限膨胀。

WhatsApp 的做法是 inbox 只保留 30 天，过期物理删除，客户端降级为按 `conv_seq` 范围全量拉取。Signal 则根本不用服务端收件箱模型（端到端加密下服务端不可信）。

**建议**。在 SPEC-003 和 SPEC-010 中分别加入一致的收件箱保留策略，例如：

> inbox 条目生命周期 = max(30d, max_device_last_active + 7d)。超过后物理删除。客户端收到 `gap_detected=true` 时降级为 per-conversation `conv_seq` 范围拉取。

---

### C2. 群聊异步扇出的幂等保证不牢靠（006）

**问题**。SPEC-006 说扇出 worker 崩溃后重做时用 `ON CONFLICT DO NOTHING` 保证幂等。但 inbox 主键是 `(user_id, inbox_seq)`——而 `inbox_seq` 是全局自增的：

> 重做 → 分配到新 `inbox_seq` → 新 PK → `ON CONFLICT` 不命中 → 同一个 `(conv_id, conv_seq)` 被插两次。

正确的幂等键应该是 `(user_id, conv_id, conv_seq)` 而不是 `(user_id, inbox_seq)`。SPEC-006 原文中括号里的注释其实在说这个：

> `ON CONFLICT DO NOTHING`，主键 `(user_id, inbox_seq)` 换成 `(user_id, conv_id, conv_seq)` 派生即可幂等

但这在 SQL 设计上没有展开——"派生"怎么实现？是加一个唯一约束、还是换主键？如果换主键，那 SPEC-003 的 `PRIMARY KEY (user_id, inbox_seq)` 就要变。这是两个 spec 之间的未解决依赖。

**建议**。在 SPEC-006 中给出具体的 DDL 变更：

```sql
ALTER TABLE inbox ADD CONSTRAINT uq_inbox_msg 
  UNIQUE (user_id, conv_id, conv_seq);
```

同时把 SPEC-003 的 inbox DDL 里也加上这条约束（不在 M1 改主键，只加唯一约束，成本最低）。

---

### C3. Presence TTL 与心跳超时的数值关系未显式论证（007 ↔ 002）

**问题**。SPEC-002 定义心跳超时 75s（3 个 25s 周期）。SPEC-007 定义 presence TTL 90s。方向是对的（TTL > 心跳超时，防止误判离线），但这个 15s 的 margin 是否足够应对 GC 停顿、网络抖动、Redis 写入延迟——没有论证。

边界情况：如果 TTL 设得太短，一次 GC pause（Go 1s+ 不是不可能）+ Redis 写入排队 → presence 提前过期 → 在线用户被误判离线 → 消息走推送路径白费一轮。如果太长，离线检测延迟增大。

**建议**。在 SPEC-007 中加一段明确的数值论证，TTL 最好 = 心跳超时 + 2×(心跳间隔 + Redis 写 p99)。如果心跳 25s、超时 75s、Redis 写 p99 < 100ms，则 TTL ≈ 75 + 2×25.1 ≈ 125s 更安全。90s 的当前值偏激进，建议改为 120s 并在验收标准中增加"连续运行 30 分钟零误判离线"的指标。

---

### C4. SPEC-008 双保险的时间窗口假设未在 003 中定义

**问题**。SPEC-008 说：

> 走长连接推送后 10s 内未收到该设备的 AckDelivered → 补发一次 APNs

但 SPEC-003 的推送模型只说"异步推送 best-effort"，没有定义设备 AckDelivered 的预期延迟。10s 是合理的 engineering judgment，但缺乏 spec 层面的依据。如果消息处理链路上某个环节拥塞 15s（合理——SPEC-003 吞吐基准是 1,000 msg/s 但不是延迟保证），那几乎每条在线推送都会触发一次 APNs 兜底，推送量翻倍。

**建议**。在 SPEC-003 中定义在线推送的 AckDelivered 预期延迟（如 p99 < 2s），SPEC-008 的兜底窗口基于此值设定（如 p99.9 + 3s margin），并给出推导过程。

---

## 🟡 模糊与遗漏

### A1. Snowflake 的机器 ID 分配未涉及容器化场景

SPEC-001 说用 Snowflake 发号器，单测包含时钟回拨保护。但在 Docker 多实例（SPEC-000 的部署就是 2×gateway + msgsvc 多实例）下，Snowflake 的 `worker_id` 怎么分配？手动配置（docker-compose 里写死 `WORKER_ID=1/2/3`）是可行的但这个做法本身应该被文档化，因为它牵涉到"worker ID 碰撞 = server_msg_id 重复"这个严重正确性 bug。

**建议**。SPEC-001 验收标准补充一条：多实例并发发号 3 实例 × 10w 次无跨实例重复（docker-compose 集成测试）。

---

### A2. Gateway 路由表从单值升级到集合的迁移策略缺失（010）

SPEC-010 把 `route:{uid}` 从单 `gateway_id` 升级为 `{device_id → gateway_id}` 集合。但 Redis key 的 value 格式变了——旧数据的读取和迁移策略是什么？

**建议**。在 SPEC-010 中加一个迁移段落：使用新的 Redis key pattern（如 `route:{uid}:{device_id}`），读时先读新 key、fallback 读旧 key（向后兼容），写时只写新 key。旧 key 自然过期（TTL 90s）后迁移完成，无需离线维护窗口。

---

### A3. 错误帧格式与错误处理策略空白

SPEC-001 的 Envelope 里有 `ErrorFrame`，Proto 枚举了它，但没有任何 spec 定义错误码体系。哪些错误是可重试的（nack → 客户端退避重连）？哪些是致命的（鉴权失败 → 立即拒绝且不重试）？错误帧里带什么信息？这是整个协议的空白页。

**建议**。在 SPEC-001 中加一个 ErrorCode 枚举表（至少 10 个常见码），定义两类：可恢复（RETRYABLE：如 SERVER_UNAVAILABLE、RATE_LIMITED）和致命（FATAL：如 AUTH_FAILED、DEVICE_REVOKED），以及客户端对每类的标准响应。

---

### A4. "first message" 冷启动场景的 prekey 耗尽路径不完整（011）

SPEC-011 描述了 X3DH 预铸钥匙：每个设备注册时上传 IK + SPK + 100 个 OPK。Alice 首条消息找服务器取 Bob 的 prekey bundle。但 OPK 用完了怎么办？SPEC-011 只提了"OPK 耗尽的降级路径（只用 SPK，牺牲一点前向性）+ 客户端低水位自动补铸"——这只描述了对策，没说具体的实现判定和客户端的连动。

**建议**。加一段：Key Server 在 OPK 数量降为 0 时返回 `opk_exhausted=true` 标志；客户端收到时应立即补铸（不等待 100 个用完）。同时 Bob 侧收到首条消息后检查本机 OPK 池 ≤ 20 个时触发补铸。这个逻辑应当在 SPEC-011 的 acceptance criteria 中有一条："OPK 耗尽场景下 1:1 首条消息延迟 < 2×正常情况"。

---

### A5. `client_msg_id` 的跨会话语义未定义

SPEC-001 说 `client_msg_id`（UUIDv7）用于幂等去重，唯一约束在 `(sender_id, client_msg_id)`。如果同一条消息被转发到多个会话（或"群发"功能），客户端用同一个 `client_msg_id` 还是新 ID？用同一个 → 唯一约束不冲突（`conv_id` 不在唯一索引里），但语义上变成"不同会话的两条独立消息共享幂等键"——对吗？

**建议**。明确：`client_msg_id` 是 **per-send-action** 的幂等键，不是 per-message-content。转发 = 新的 send action = 新的 `client_msg_id`。在 SPEC-001 的消息状态机文档中写明。

---

### A6. 配置管理与服务发现缺失

SPEC-000 的架构图有 Gateway、Message Service、API Service、Push Worker——5 类服务，docker-compose 拉起。Gateway 调 msgsvc 用 gRPC，msgsvc 调 gateway 也用 gRPC（push RPC）。但没有任何 spec 定义这些服务间怎么发现彼此（DNS? hardcoded ports?）、配置怎么管理（环境变量? config file?）。

**建议**。不作为独立 spec（项目规模不需要），在 SPEC-002 或 SPEC-000 中加一个"服务配置"小节，列出环境变量表（`MSGSVC_ADDR`、`REDIS_ADDR`、`DB_DSN` 等），docker-compose 里自解释。

---

## 🟢 增强建议

### E1. SPEC-000 的依赖图可以更精确

当前依赖图用 ASCII 箭头，但缺少一些间接关系——比如 004 在 M1 但 iOS 的 APNs 部分依赖 008（M2）；009 依赖 MinIO 部署（不在任何 spec 的范围内）。建议在 SPEC-000 中加一个 n×n 依赖矩阵表格，清晰展示"哪些 spec 可以在另一个没完成之前开始但必须之后完成"。

---

### E2. SPEC-005 缺少多设备场景的压测场景

SPEC-010 引入了 per-device 游标、回环投递、路由集合化——这些都是新的性能变量。SPEC-005 的 loadtest 场景目前只覆盖 M1 的 5w 连接 + 1,000 msg/s + kill -9。建议在 M3 的 SPEC-005 交付物中加入 `multi_device.yaml`（每用户 2~3 台虚拟设备），验证 inbox 写入量不随设备数线性放大。

---

### E3. SPEC-007 typing indicator 的节流使用了具体的数值（每 4s 一次），但没有说明为什么是 4s

WhatsApp/Signal 用的大约是 2~3s。如果客户端已经在本地节流（SwiftUI 侧做防抖），服务端又做一层 consolidation——两层的交互会不会导致"用户敲了半天对方什么也没看到"？建议把这个数值标记为"可调"，在验收标准中做一个体验实验（两边各真机，肉眼确认 typing 感知延迟可接受）。

---

### E4. SPEC-009 blurhash 编码大小写的是 ~30 字节，但 blurhash 对 4×3 分量（默认）base83 编码约 30 字符，按 UTF-8 确实 ~30 字节

这是对的，但可以加一个引用——"blurhash 的默认分量 4×3，base83 编码后 ~30 字节"——以便读者不需要自己算。

---

### E5. SPEC-011 的"前向保密实验"验收标准设计精妙但可复现性需要加固

> 导出接收方当前 ratchet 状态 → 尝试解密该会话 100 条前的历史密文 → 失败

"失败"是指返回错误还是返回垃圾明文？libsignal 对"用错密钥"的 API 行为是什么——抛异常还是静默返回 garbage？验收标准要把行为具体化，否则脚本判断不了。

---

## 问题汇总表

| # | 类型 | 涉及 Spec | 严重度 | 摘要 |
|---|------|-----------|--------|------|
| C1 | 逻辑矛盾 | 003, 010 | 🔴 | per-user inbox_seq 与 per-device 游标的生命周期冲突，缺少保留策略 |
| C2 | 正确性风险 | 006, 003 | 🔴 | 扇出重试的幂等键与主键不一致，需要额外唯一约束 |
| C3 | 正确性风险 | 007, 002 | 🔴 | Presence TTL 90s vs 心跳超时 75s，margin 15s 可能不够 |
| C4 | 逻辑矛盾 | 008, 003 | 🔴 | APNs 兜底 10s 窗口缺少 AckDelivered 预期延迟定义 |
| A1 | 遗漏 | 001 | 🟡 | Snowflake worker_id 的容器化分配方案缺失 |
| A2 | 遗漏 | 010 | 🟡 | Redis 路由表 value 格式迁移策略缺失 |
| A3 | 遗漏 | 001 | 🟡 | 错误码体系空白，ErrorFrame 只有类型没有语义 |
| A4 | 遗漏 | 011 | 🟡 | OPK 耗尽路径的客户端补铸连动未定义 |
| A5 | 模糊 | 001 | 🟡 | client_msg_id 的跨会话/转发语义未定义 |
| A6 | 遗漏 | 002, 000 | 🟡 | 服务发现与配置管理未文档化 |
| E1 | 增强 | 000 | 🟢 | 依赖图可加矩阵表示 |
| E2 | 增强 | 005, 010 | 🟢 | 缺少多设备压测场景 |
| E3 | 增强 | 007 | 🟢 | typing 节流 4s 缺乏推导依据 |
| E4 | 增强 | 009 | 🟢 | blurhash 大小可加推导注释 |
| E5 | 增强 | 011 | 🟢 | 前向保密验收需定义行为语义 |

---

## 总体判定

这 12 份 spec 在**架构思想、分拆粒度、验收标准设计**三个维度上均达到 professional-grade 水平。4 个 🔴 问题集中在跨 spec 的边界条件和生命周期管理上——这恰恰是"把一个系统拆成 12 份 spec"时最容易被忽略的：每份 spec 内部自洽，碰在一起才暴露裂缝。这些问题修复代价都很低（主要是加几个段落/约束），不需要任何架构返工。

**建议**：修复 4 个 🔴 后即可进入 M1 实施；6 个 🟡 可以在实施中边做边补（它们不影响正确性，但会影响调试效率和可维护性）；5 个 🟢 可在 M2/M3 实施前自行评估是否采纳。

---

## 处理结果（2026-07-10）

全部 15 条意见已处理：🔴×4 与 🟡×6 全部修复，🟢×5 全部采纳。

| # | 处理 | 落点 |
|---|------|------|
| C1 | ✅ 已修复 | SPEC-003 新增收件箱保留策略（`max(30d, 全设备 max(last_active_at)+7d)`，出窗返回 `gap_detected=true` 降级 per-conversation 拉取）；SPEC-010 挑战 A 引用同一策略，明确"最慢设备不拖累回收" |
| C2 | ✅ 已修复 | SPEC-003 inbox DDL 直接加入 `CONSTRAINT uq_inbox_msg UNIQUE (user_id, conv_id, conv_seq)`（M1 起就有，不改主键）；SPEC-006 展开幂等机制：扇出 SQL 用 `ON CONFLICT ON CONSTRAINT uq_inbox_msg DO NOTHING`，并解释为何主键挡不住重做 |
| C3 | ✅ 已修复 | SPEC-007 加入 TTL 推导公式（含 GC 停顿余量 2s），TTL 从 90s 调整为 **150s**（比评审建议的 120s 更保守，因为计入了续期间隔 30s 而非心跳间隔 25s）；验收新增 2b"30 分钟零误判离线"。注：SPEC-002 路由表的 TTL 90s 是另一个 key（30s 续期、3 倍余量），不受 C3 影响，未改 |
| C4 | ✅ 已修复 | SPEC-003 定义 AckDelivered 延迟预算 p99 < 2s / p99.9 < 7s 并进 005 面板；SPEC-008 写明 10s = p99.9(7s) + 3s margin 的推导，及预算漂移时"先修拥塞后放宽窗口"的纪律 |
| A1 | ✅ 已修复 | SPEC-001：worker_id 改为启动时 Redis 租约申请（SET NX EX + 续期，失败 fail-fast），不用手写环境变量；验收新增 2b 三实例并发发号 + 租约回收测试 |
| A2 | ✅ 已修复 | SPEC-010：新 key pattern `route:{uid}:{device_id}`，双读（新优先旧 fallback）单写新 key，旧 key TTL 自然过期，双读代码一个版本周期后删除 |
| A3 | ✅ 已修复 | SPEC-001 新增 3.2 错误码体系：10 个码分 RETRYABLE/FATAL 两类，每码带客户端标准响应，`retry_after_ms` 字段，判据"重试能否改变结果"，码号 <100 保留给协议层 |
| A4 | ✅ 已修复 | SPEC-011：三方连动（Key Server 原子 pop + `opk_exhausted` 标志 + 限速防抽干；Bob 存量 ≤20 收 `opk_low` 信令即补铸；Alice SPK-only 降级不报错）；验收新增 2b OPK 耗尽实验 |
| A5 | ✅ 已修复 | SPEC-001：明确 `client_msg_id` 是 per-send-action 幂等键，转发 = 新 ID，只有网络重试复用；生成规则纳入 004 OutboxWorker 契约 |
| A6 | ✅ 已修复 | SPEC-002 新增"服务配置与发现"小节：docker-compose DNS + 环境变量表，缺必填 fail-fast，msgsvc→gateway 反向推送走路由表自注册地址 |
| E1 | ✅ 已采纳 | SPEC-000 新增 n×n 依赖矩阵（●硬依赖/◐可并行但不可先收尾），含 6 条间接依赖脚注与基础设施归属注 |
| E2 | ✅ 已采纳 | SPEC-005 场景清单按 milestone 扩展：M2 加 group_500/presence_mixed，M3 加 multi_device.yaml（断言 inbox 写入量不随设备数放大） |
| E3 | ✅ 已采纳 | SPEC-007：typing 4s/6s 标记为可调初始值，写明约束关系（清除超时 > 节流间隔 + 传输 p99）与"只在客户端节流、服务端不叠加防抖"；验收新增 4b 双真机体验实验 |
| E4 | ✅ 已采纳 | SPEC-009：blurhash 注明"默认 4×3 分量、base83 约 30 字符"的推导 |
| E5 | ✅ 已采纳 | SPEC-011 验收 2：明确 libsignal 抛解密异常（AEAD 认证标签保证不会静默返回垃圾明文），脚本断言捕获异常，实施前 spike 确认绑定层异常类型 |
