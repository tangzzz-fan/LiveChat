# SPEC 评审意见（第二轮）— LiveChat 全 15 份规格文档

> 评审日期: 2026-07-10 | 评审范围: SPEC-000 ~ SPEC-014（含第一轮修复 + M4 新增 012/013/014）

## 前置说明

第一轮 REIVEW-SPECS.md 的 15 条意见（🔴×4 + 🟡×6 + 🟢×5）已全部处理，修复痕迹可在各 spec 原文中逐条对账。本轮聚焦三件事：

1. **M4 三份新 spec 的内部质量与互锁关系**
2. **M4 与 M1~M3 的跨 milestone 一致性**（spec 拆得越细，跨文件裂缝越多）
3. **第一轮修复之后遗留的次生问题**

问题仍按 🔴/🟡/🟢 三级组织，并在末尾给出"可以进入 M1 实施吗？"的总体判定。


## 🔴 正确性与一致性风险

### C5. 012 的 FSM 范式声称层级但不编码层级（012 内部矛盾）

**问题**。SPEC-012 §挑战 D 定义了统一的 FSM 泛型协议：

```swift
protocol StateMachine {
  associatedtype State: Equatable
  associatedtype Event
  associatedtype Effect
  static func transition(_ s: State, _ e: Event) -> (State, [Effect])
}
```

然后在同一段里说"层级与并行状态：连接机是父机，会话页呈现机是子机（父机
disconnected 时子机整体进 offline 呈现态）——学 statechart 的层级思想但不引
XState 类重型库"。

**冲突**：上面的 `protocol` 是一个**纯扁平状态机**——`State` 是单一
`Equatable` 类型，没有子状态、没有正交区域、没有历史伪态。父机 disconnected
→ 子机进 offline 这个行为在协议里根本编码不了：要么 `State` 膨胀为
`ConnectionState × ConversationState` 的笛卡尔积（状态数爆炸），要么两个机
各自独立跑、靠 Effect 通信（但这不是层级，是 pub-sub）。

**建议**。选择一条路并写死：

- **路线 A（推荐，项目规模够用）**：放弃层级，父机 disconnected 时产生
  `Effect.connectionLost`，子机的 runtime 在收到该 Effect 时把子机状态置为
  offline——Effect 是机与机之间唯一的耦合面。把"学 statechart 的层级思想"
  这句话删掉或改为"学 actor 模型的消息驱动思想"，否则误导读者以为泛型协议
  能编码层级。
- **路线 B**：真的实现层级（引入 compound state、正交区域、历史伪态），但这
  个工作量足够单独成库，对本项目的学习目标 ROI 为负。

无论选哪条，状态图文档（`docs/statecharts/`）应如实反映实际实现，不画协议
不支持的特性。

---

### C6. 014 的 primary path 要求 iOS 26+，与 D6（iOS 17+）冲突

**问题**。SPEC-000 §2 技术决策 D6 写定 `iOS 17+`。但 SPEC-014 §挑战 A 说：

> 主路径：Apple Foundation Models framework（iOS 26+）

Foundation Models framework 是 WWDC 2026 新框架，iOS 17 上根本不存在。
014 给了备选路径（Core ML 自带模型 / MLX），但措辞是"备选"——这意味着
主路径在 iOS 17~25 设备上完全不可用。

**这个矛盾有两种读法**：

- **读法 1**：M4 是真把最低部署目标抬到 iOS 26。这会让 SPEC-004 的 M1 代码
  在 iOS 17 上跑但到了 M4 突然不可用——004 和 014 是不同 milestone，但同一
  个 App target。
- **读法 2**：014 功能只在 iOS 26+ 设备上可用（`@available(iOS 26, *)`），
  低版本退化到备选路径或静默隐藏。这是 014 自己声明的"资源纪律"逻辑。

**建议**。显式声明选择读法 2，并在以下三处对齐措辞：

1. SPEC-000 D6 加注释："M4 端侧 AI 功能的 primary API 要求 iOS 26+，低版本
   走 Core ML 备选路径或功能降级隐藏，App 最低部署目标仍保持 iOS 17"；
2. SPEC-014 §挑战 A 把主路径措辞从"iOS 26+"改为"iOS 26+（有则用），
   备选路径 Core ML（iOS 17+ 通用降级）"，去掉"备选"二字的次要暗示；
3. SPEC-004 的 M4 扩展点加一条：AI 入口组件用 `@available(iOS 26, *)` 编译
   期守卫，低版本编译时不链接 Foundation Models 符号——这本身就是一次真刀真枪
   的"多 iOS 版本 API 适配"学习点。

---

### C7. 013 的新服务 aisvc 未出现在架构全景图中（000）

**问题**。SPEC-013 §挑战 B 引入新后端服务 `aisvc`（订阅 bot 消息、组装上下文、
调 LLM、流式产出），这是继 gateway / msgsvc / api / pushworker 之后第五个
Go 服务。SPEC-000 §7 仓库结构注了 `aisvc(M4)/`，但 §3 的 ASCII 架构图没有
aisvc 的位置——它应该连到哪些组件、被哪些组件调、监听什么——在新读者看来
是凭空出现的。

**建议**。在 SPEC-000 §3 架构图中补上 aisvc（放在 msgsvc 旁，连 msgsvc 的
消息管道、连 gateway 的 StreamChunk 直推、连外部 LLM），并标注"(M4)"。不是
装饰性问题——架构图决定了新接手的读者对"这个系统有哪些服务"的第一印象。

---

### C8. 012 的渲染引擎刷新机制与 SwiftUI 的 diff 周期可能互搏

**问题**。SPEC-012 §挑战 A 说：

> token 到达只写入缓冲，`CADisplayLink` 每帧（60/120Hz）醒来把缓冲一次性
> append——渲染频率与数据到达频率解耦

`CADisplayLink` 是 UIKit/AppKit 概念。在 SwiftUI 中，`CADisplayLink` 回调里
更新 `@State` / `@StateObject` 会触发 SwiftUI 的 body 重算和 diff——如果
SwiftUI 自己的布局在同一帧（也是由同一个 display link 驱动），就有可能出现
"缓冲 append 了一半、SwiftUI 恰好这帧也在 diff"的竞态。这不是 bug（actor
隔离能保证数据一致性），但可能出现"一帧内触发了两次布局"的性能浪费。

**建议**。在 SPEC-012 §挑战 A 中加一小段：

> CADisplayLink 回调中只做数据更新（写缓冲 → append 到 AttributedString），
> 不直接操作 UIView。TextKit 2 的 `NSTextContentStorage` 在后台队列做
> layout invalidation，下一帧 SwiftUI 的 body 重算时 `UITextViewRepresentable`
> 已将增量排版完成——两段布局不重叠。

这不是正确性问题，但如果 M4 实施者不熟悉 TextKit 2 + SwiftUI 桥接的细节，
可能踩进"手动驱动布局 + SwiftUI 也驱动布局"的双重刷新坑。spec 级别的三句话
可以省掉两天的 debug。

---


## 🟡 模糊与遗漏

### A7. 非流式消息气泡在 M4 是否统一切到 012 引擎？（004 ↔ 012）

**问题**。SPEC-004 M4 扩展点说"气泡文本组件保持可替换（012 的 TextKit 2
流式引擎将替换纯 SwiftUI Text 实现）"。SPEC-012 把渲染引擎定位为 "013/014
的呈现层地基"。两端都没说清楚：M4 做完后，**普通（非流式）消息**的气泡是
留在 SwiftUI `Text` 还是切到 012 引擎？

两套渲染路径的风险：emoji 行高不一致、RTL 文本方向处理不同、`AttributedString`
与 SwiftUI `Text` 的 markdown 解析行为差异——用户会在同一个会话页看到两条
"看起来字体一样但排版微妙不同"的气泡相邻排列。这不是理论问题，iMessage
历史上至少出过两次此类 bug。

**建议**。SPEC-012 开头加一句：

> M4 完成后，**所有**消息气泡（流式与非流式）统一切到 012 引擎渲染——非
> 流式消息就是"只 append 一次、无逐字动画"的退化路径。004 侧的 SwiftUI
> `Text` 实现视为 M1 原型，M4 删除。

如果决定两套并存（降低 M4 风险），则必须在验收标准中加一条"双引擎排版对齐
测试（emoji/RTL/混合中英文的 10 条 case 逐像素 diff）"。

---

### A8. SPEC-005 缺少 M4 压测场景

**问题**。SPEC-005 的交付物列出了 M2 追加载荷场景、M3 追加 `multi_device.yaml`，
但没有 M4 的对应场景。SPEC-013 在测试计划里提到 `ai_streaming.yaml`（50
并发生成 × 慢客户端混布），但这个文件的存在只在 013 里提到，005 作为"压测
场景的 canonical 索引"没有收录。

**建议**。在 SPEC-005 §3 交付物列表中加入 M4 场景：

```
# M4 追加: ai_streaming.yaml (013: 50 并发生成 × 慢客户端混布,
#          断言帧分级调度正确) / ai_on_device.yaml (014: 端侧推理
#          不阻塞主线程, 热压力降级有效)
```

---

### A9. 013 上下文窗口截断策略缺乏定义

**问题**。SPEC-013 §挑战 B 说：

> 组装上下文（按 `conv_seq` 窗口取最近 N 条历史，token 预算截断）

"token 预算截断"有至少三种实现，结果完全不同：

1. **truncate-head**：从最早的消息开始丢（丢上下文开头，适合"最近消息最相关"）；
2. **truncate-tail**：从最新的开始丢（保留开头系统提示，适合指令场景）；
3. **smart-truncate**：系统消息优先保留、图片用 caption 代替、较短的文本优先保留。

且"最近 N 条"按 `conv_seq` 取——但 bot 消息本身（partial/终版）也占
`conv_seq`，bot 的上下文是否包含自己的历史回复？包含 = 自指循环风险，
不包含 = 不能做多轮对话。

**建议**。在 SPEC-013 §挑战 B 中明确：

> 截断策略：`conv_seq` 逆序取最近 50 条（先取最新），按 token 预算从最早
> 的开始丢弃（truncate-head）。bot 自己的消息纳入上下文（支持多轮），系统
> 提示不纳入截断计数。token 计算使用 LLM 后端的 tokenizer（本地 Ollama 时
> tiktoken 估算是可接受的近似），不做按字符粗估。

---

### A10. 014 的"自动化流"边界里遗漏了静默推送触发

**问题**。SPEC-014 §挑战 E 定义了规则式自动回复：驾驶专注模式 → 白名单消息
→ 自动回复模板。但触发条件是"收到消息"——在 iOS 后台 socket 已死的情况下
（SPEC-008），收到消息靠的是 APNs。如果用户开启了专注模式，设备收到了推送，
但 App 在后台——谁来触发自动回复？

可能的路径：Notification Service Extension 可以修改推送内容但不能发消息；
后台 push（`content-available:1`）可以唤醒 App 但 iOS 限流；`BGAppRefreshTask`
不保证及时性。这三条路都不可靠。

**建议**。在 SPEC-014 §挑战 E 中坦率声明：

> 自动回复的触发窗口限定在 App 前台或 `BGAppRefreshTask` 唤醒期间（约每
> 15 分钟一次，不保证及时）。APNs 唤醒（`content-available:1`）因限流不可
> 依赖。这是 iOS 平台的硬约束——Telegram/WhatsApp 的自动回复同样受此限制。
> 文档写明"自动回复可能在离线期间延迟"，UI 上不带"即时"承诺。

并且：自动回复消息的 `server_ts_ms` 用实际发送时间而非原始消息时间，避免
"3 小时前的消息突然出现自动回复"的乱序体验。

---

### A11. 012 滚动锚定与 004 的 ScrollView+LazyVStack 的兼容性

**问题**。SPEC-004 §挑战 C 使用 `ScrollView` + `LazyVStack` 反转技巧。
SPEC-012 §挑战 B 使用 iOS 17+ 的 `scrollPosition` + `defaultScrollAnchor(.bottom)`。
两种方案在 M4 切换时，004 的"反转列表"技巧要改成 012 的原生锚定——
这会改变整个会话页的 ScrollView 层级结构。004 的 M4 扩展点只提了"气泡组件
可替换"，没提"滚动容器可替换"——但后者对 UI 的影响更大。

**建议**。SPEC-004 M4 扩展点加一句：

> 会话页滚动容器在 M4 从 LazyVStack 反转技巧切换为原生
> `defaultScrollAnchor(.bottom)` + `ScrollAnchorController`（012 定义）。
> 列表数据源（GRDB ValueObservation）保持不变，切换只碰容器层。

---

### A12. 011 E2EE 盲测验收未考虑 bot 会话豁免的影响

**问题**。SPEC-013 §挑战 D 声明"bot 会话显式不做 E2EE"。SPEC-011 §验收 1
（服务器盲测）说 dump 所有 messages.content 全为密文。但 bot 会话的消息
是明文落库的——这两个断言不矛盾（bot 会话不在 scope 内），但验收脚本如果
直接 `SELECT content FROM messages` 不加 `WHERE conv_id NOT IN (SELECT
conv_id FROM conversations WHERE has_bot=true)` 就会假失败。

**建议**。SPEC-011 §验收 1 加入豁免过滤声明：

> 盲测脚本显式排除 bot 会话（`has_bot=true`），豁免范围在测试用例中明文列
> 出，禁止静默跳过。

并增加一条反向验证："bot 会话的消息 content 为明文"——既证明豁免是正确的，
也防止哪天 bot 会话被无意加密后无人察觉。

---


## 🟢 增强建议

### E6. 002 的帧分级调度应在 SPEC-002 正文中留升级点注记

**现状**：SPEC-013 §挑战 C 要求"帧分级调度"（StreamChunk 可丢弃、普通消息
帧仍走断连规则）。这要求 002 的 sendCh 溢出策略从"统一断连"升级到"按帧优先
级决策"。013 说这是"002 的调度器升级点"，但 002 正文里没有给这个升级留注记。

**建议**：在 SPEC-002 §挑战 B 的"溢出策略 = 断开连接"处加一句"(M4 升级：
帧分级后只有不可丢弃帧溢出才断连，可丢弃帧直接丢弃，见 SPEC-013)"。
类似 004 的 M4 扩展点风格。

---

### E7. 012 的状态图文档格式未定义

**现状**：SPEC-012 §挑战 D 说"每台机器附一张状态图进 `docs/statecharts/`"，
但没说用什么格式——手绘 PNG？Mermaid？ASCII art？Graphviz dot？

**建议**：指定 Mermaid（`stateDiagram-v2`），因为：
- 纯文本，进 git 可 diff；
- GitHub/GitLab 原生渲染，CI 友好；
- 与 SPEC-000 的 Mermaid 依赖图一致。

且 Mermaid 语法不支持的就说明"此处 Mermaid 无法表达，用 PlantUML 或文字说明"。

---

### E8. 014 的语义搜索评估缺少基准数据集

**现状**：SPEC-014 §验收 4 说"30 条造数据用例上 Recall@10 ≥ 80%"。30 条偏少，
且"造数据"意味着和实际聊天数据可能有分布偏移。

**建议**：不扩大评测集（30 条够用，本项目的学习目标不需要 BEIR 级 benchmark），
但加一条约束：造数据用例覆盖 3 种查询类型各 10 条——"精确匹配"（"上周五的
那张图"）、"语义关联"（"那家餐厅"）、"时间锚定"（"上个月 Alice 说的..."），
分别报告 Recall，暴露各类型的弱点而非被平均数掩盖。这本身就是一次"搜索质量
评估怎么设计"的工程练习。

---

### E9. 013 的 StreamChunk 与 007 typing indicator 的数据分级对称性值得点明

**现状**：013 把 StreamChunk 归入 007 定义的数据分级光谱（"可丢弃"端），这
很好。但 013 没有提到 typing indicator 和 StreamChunk 在"不落库、不重试、
纯转发"上的对称性——它们分别在"可丢弃"光谱的两端各有一次独立实现，这正是
SPEC-007 把"数据分级"作为核心学习点想要的效果。

**建议**：在 SPEC-013 §挑战 A 结尾加一句："StreamChunk 与 typing (007)
对称为'纯转发、不落库、不重试'的可丢弃数据——它们独立验证了 007 设计光谱
的正确性：不是 typing 特殊，而是所有可丢弃数据都该这么做。"

---

### E10. 014 索引管道与 GRDB 的写入并发

**现状**：SPEC-014 §挑战 D 说"新消息落库后异步嵌入（批量、低 QoS 队列）"。
GRDB 是 SQLite wrapper，SQLite 多线程写需要 WAL 模式（GRDB 默认开启），但
批量嵌入写入向量索引时如果持有长写事务，会阻塞 ValueObservation（004 的
UI 刷新路径）。这不是一定会发生的 bug，但设计上值得提一句。

**建议**：在 SPEC-014 §挑战 D 中加一句："嵌入写入采用短事务（每 10 条一批
commit），不与 GRDB 的 ValueObservation 刷新周期争锁。"

---

## 问题汇总表

| # | 类型 | 涉及 Spec | 严重度 | 摘要 |
|---|------|-----------|--------|------|
| C5 | 内部矛盾 | 012 | 🔴 | FSM 泛型协议不支持层级，但文字承诺层级/并行状态 |
| C6 | 一致性风险 | 014, 000(D6) | 🔴 | Foundation Models 要求 iOS 26+，与 D6 iOS 17+ 冲突 |
| C7 | 遗漏 | 013, 000 | 🔴 | aisvc 服务未出现在架构全景图中 |
| C8 | 正确性风险 | 012 | 🔴 | CADisplayLink 驱动更新与 SwiftUI diff 周期可能互搏 |
| A7 | 模糊 | 004, 012 | 🟡 | 非流式消息气泡在 M4 是否统一切换未明确 |
| A8 | 遗漏 | 005, 013, 014 | 🟡 | M4 压测场景未收入 SPEC-005 canonical 索引 |
| A9 | 遗漏 | 013 | 🟡 | 上下文窗口截断策略与自指循环未定义 |
| A10 | 遗漏 | 014, 008 | 🟡 | 自动回复的 iOS 后台触发路径不可靠 |
| A11 | 遗漏 | 004, 012 | 🟡 | 滚动容器的切换策略未列入 004 M4 扩展点 |
| A12 | 遗漏 | 011, 013 | 🟡 | E2EE 盲测脚本需显式排除 bot 会话 |
| E6 | 增强 | 002, 013 | 🟢 | 002 正文缺帧分级调度升级点注记 |
| E7 | 增强 | 012 | 🟢 | 状态图文档格式未定义 |
| E8 | 增强 | 014 | 🟢 | 语义搜索缺少分查询类型的评估（学习点） |
| E9 | 增强 | 013, 007 | 🟢 | StreamChunk 与 typing 的数据分级对称性未点明 |
| E10 | 增强 | 014 | 🟢 | GRDB 写入并发与向量索引事务策略未说明 |


## M4 依赖互锁摘要（与 SPEC-000 对照）

SPEC-000 的 M4 依赖声明和 spec 内的交叉引用一致，未发现矛盾：

| 声明（000） | 实际（spec 内） | 一致？ |
|-------------|----------------|--------|
| 012 ← 004（●） | 012 的渲染引擎替换 004 的气泡实现 | ✅ |
| 013 ← 001,003,004,012（●） | 013 扩展 001 协议、复用 003 管道、消费 004 outbox/012 渲染 | ✅ |
| 013 ← 002（◐ 帧分级） | 013 §挑战 C 要求 002 帧分级调度，002 未留注记 | ⚠️ 缺注记（见 E6） |
| 013 ← 011（◐ bot 豁免） | 013 §挑战 D 声明 bot 会话不加密，011 盲测未列豁免 | ⚠️ 需对齐（见 A12） |
| 014 ← 004,012（●） | 014 消费 012 渲染 + 004 GRDB/outbox | ✅ |
| 014 ← 011（◐ 架构论证） | 014 §挑战 A 统一心智模型（加密不转云） | ✅ |
| 012 先行，013/014 可并行 | 013 依赖 012 渲染，014 依赖 012 渲染 + 004，013 与 014 互相无依赖 | ✅ |

---

## 总体判定

**M1~M3 的 12 份 spec 已达到实施就绪状态**——第一轮 🔴 问题全部修复且修复
质量高（多数补了配套验收条目），无需第二轮修回。

**M4 的 3 份 spec 整体质量与 M1~M3 持平**，"挑战 → 典型解法 → 验收" 结构
完整，012 → (013, 014) 的依赖顺序正确且论证充分。两个架构决策——013 的 bot
豁免 E2EE 与 014 的端侧不转云——是诚实的取舍，且已被 013/014 各自写明了
理由。

4 个 🔴 问题中：
- **C5**（FSM 泛型不支持层级）和 **C6**（iOS 版本冲突）是需要在代码开工前
  必须决策的——前者的写法决定了五台机器的迁移方式，后者决定了 App target
  的 `IPHONEOS_DEPLOYMENT_TARGET` 是否需要分 phase 调整。这两个不建议拖到
  实施中。
- **C7**（架构图缺 aisvc）和 **C8**（CADisplayLink 与 SwiftUI 互搏）是
  文档完善级别，不影响正确性但影响实施效率——特别是 C8，如果实施者对
  TextKit 2 桥接不熟，spec 里先写好就不会踩坑。

**建议**：4 个 🔴 修复后（保守估计 2 小时工作量，全是改文档/画图），M4 的
3 份 spec 即可与 M1~M3 一起进入实施。6 个 🟡 和 5 个 🟢 可在对应 milestone
实施过程中边做边补。

---

## 处理结果（2026-07-10，辩证采纳）

15 条全部处理完毕：11 条按评审建议采纳，4 条（C8/A10/A11/E7）经辩证分析后
**修正或升级评审的解法**再落地。分歧点均注明理由。

### 按评审建议采纳（11 条）

| # | 处理 | 落点 |
|---|------|------|
| C5 | ✅ 路线 A | SPEC-012 挑战 D 重写：明确不做层级，`Effect.connectionLost` → 对端 runtime 注入事件，"Effect 是机与机之间唯一耦合面"（actor 消息驱动，非 statechart 层级）；删除层级承诺；状态图只画实现了的东西 |
| C6 | ✅ 读法 2 | 三处对齐：SPEC-000 D6 加注（部署目标恒为 iOS 17，AI 入口 `@available(iOS 26,*)` 守卫）；SPEC-014 挑战 A 改为"iOS 26+ 有则用 / 低版本降级隐藏 / Core ML 仅文档化不默认实现"；SPEC-004 M4 扩展点加编译期守卫条目。辩证补充：降级隐藏与 014 既有"资源纪律"（不可用即隐藏、绝不转云）是同一心智模型，非新增规则 |
| C7 | ✅ 已补 | SPEC-000 §3 架构图新增 AI Service (M4) 框：订阅 bot 消息（经 msgsvc）→ LLM → StreamChunk 经 gateway 直推（可丢弃帧）→ 终版消息回写 msgsvc |
| A7 | ✅ 统一引擎 | SPEC-012 挑战 A 明确：M4 后全部气泡（流式+非流式）统一走 012 引擎，非流式 = "append 一次"退化路径，004 的 SwiftUI `Text` 是 M1 原型 M4 删除；SPEC-004 扩展点同步措辞。未选双引擎并存备选（逐像素 diff 测试的维护成本 > 统一的迁移成本） |
| A8 | ✅ 已补 | SPEC-005 场景索引追加 M4：`ai_streaming.yaml`（断言帧分级：chunk 可丢、消息帧不丢）。评审建议的 `ai_on_device.yaml` 未收——端侧推理是纯客户端行为，不属于服务端 loadtest 工具的管辖，对应验收已在 014 §验收 5（热压力/主线程）用 Instruments 覆盖 |
| A9 | ✅ 定案 | SPEC-013 挑战 B：逆序取 50 条 + truncate-head + bot 历史纳入（多轮前提）+ 系统提示不参与截断 + 后端 tokenizer 计数。辩证补充：自指循环风险由"上下文只含终版消息、绝不含 partial"消解，比"排除 bot 消息"损失更小 |
| A12 | ✅ 已补 | SPEC-011 验收 1：盲测脚本显式过滤 bot 会话、豁免清单明文列出禁止静默跳过 + 反向断言（bot 会话必须明文，防误加密后 aisvc 静默失效） |
| E6 | ✅ 已补 | SPEC-002 挑战 B 溢出策略处加 M4 升级点注记，并给实施提示："帧能否丢弃"留作帧属性位，M1 调度器就地可扩展 |
| E8 | ✅ 已补 | SPEC-014 验收 4 改为三类查询（精确/语义/时间锚定）各 10 条分别报告 Recall@10 |
| E9 | ✅ 已补 | SPEC-013 挑战 A 结尾点明 StreamChunk 与 typing 的对称性："不是 typing 特殊，而是所有可丢弃数据都该这么做" |
| E10 | ✅ 已补 | SPEC-014 挑战 D：嵌入写入短事务（每 10 条一批 commit），不与 ValueObservation 争锁 |

### 辩证修正后采纳（4 条）

| # | 评审的解法 | 实际落地 | 分歧理由 |
|---|-----------|---------|---------|
| C8 | CADisplayLink 回调只做数据更新 + "TextKit 2 在后台队列做 layout invalidation" | SPEC-012 挑战 A 新增"与 SwiftUI 刷新周期的分工"：每帧更新**完全绕过 SwiftUI 状态系统**（不碰 `@State`，直写 text storage），SwiftUI 只管气泡外壳等低频结构，高度经 `sizeThatFits` 合批回报 | 评审方案治标：只要每帧更新还经过 `@State`，body 重算 + diff 就躲不掉；"后台队列排版"的线程约束（TextKit 2 与 UIView 主线程更新）本身是坑。根治 = 高频路径走 UIKit 直通，把 SwiftUI 从每帧循环里整个摘出去 |
| A10 | 在 014 声明触发窗口限制 + "`server_ts_ms` 用实际发送时间而非原始消息时间" | 触发窗口声明照加（前台 / BGAppRefreshTask，APNs 静默推送不可依赖，UI 不承诺即时）；时间戳一条注明**001 已天然保证**——`server_ts_ms` 由服务端落库时分配，客户端无能力指定，乱序场景架构上不存在 | 前半条完全成立且重要（平台诚实）；后半条是把既有不变量当成了待修问题——修复方式是引用 001 而非新增规则，避免读者以为时间戳可由客户端选择 |
| A11 | 在 004 的 M4 扩展点注记"滚动容器届时从反转技巧切换为原生锚定" | **消灭这次迁移**：SPEC-004 挑战 C 直接改为 M1 起就用 `scrollPosition` + `defaultScrollAnchor(.bottom)`（iOS 17 API，部署目标内可用），012 只在其上叠加 `ScrollAnchorController` 策略层 | 评审默认了"M1 用反转、M4 迁移"的前提，但原生锚定在 iOS 17 就可用——既然 M4 终点已知且起点无版本障碍，正确做法是别走弯路，而不是给弯路立路标。反转技巧的手势/菜单镜像坑也一并规避 |
| E7 | Mermaid，理由含"与 SPEC-000 的 Mermaid 依赖图一致" | SPEC-012 指定 Mermaid `stateDiagram-v2`（可 diff、GitHub 原生渲染），表达不了的用图旁表格补充 | 结论采纳，但论据有事实错误：SPEC-000 的依赖图是 ASCII 不是 Mermaid。Mermaid 的成立理由是前两条，不是一致性 |

### 结论

M1~M4 全部 15 份 spec 处于评审闭环状态，可进入 M1 实施。
