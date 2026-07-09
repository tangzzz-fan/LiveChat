# SPEC-012 — iOS 实时交互 UX 与复杂状态机

> 状态: Draft | Milestone: M4 | 依赖: SPEC-004 | 被依赖: 013（流式渲染）, 014（AI 输出呈现）

## 1. 背景与动机（Why）

M1 的 004 解决的是"数据正确地流动"；本 spec 解决的是"变化正确地呈现"。
两个学习主线：**流式/高频更新下的渲染工程**（打字机效果是它的试金石），
和**把 App 里所有异步面显式建模为状态机**（消灭"散落的 Bool 标志位"这个
iOS 复杂界面的万病之源）。产出同时是 013/014 的呈现层地基——AI 的流式输出
没有这一层就只能整段闪现。

## 2. 核心挑战与典型解法

### 挑战 A：打字机效果 —— 流式文本渲染的工程学

天真做法：每来一个 token 就替换整个 `AttributedString` 触发一次布局。
后果：布局成本随文本长度线性增长 × token 数 = 二次方总成本，长回复必掉帧；
且每次全量重排会抖动行高，滚动位置跳跃。

**解法（三层）：**
1. **帧级合批**：token 到达只写入缓冲，`CADisplayLink` 每帧（60/120Hz）
   醒来把缓冲一次性 append——渲染频率与数据到达频率解耦；
2. **增量排版**：TextKit 2 只对 append 的 range 做 layout invalidation，
   不动已排版部分（这是 TextKit 2 相对 SwiftUI `Text` 全量 diff 的核心优势，
   聊天气泡内嵌 `UITextView` 桥接是值得学的取舍）；
3. **背压降级**：渲染是 UX 糖，不许拖数据。缓冲积压超过阈值（如 2 帧
   预算内排不完）→ 放弃逐字动画，整段跳跃追平。**打字机速度上限恒定，
   落后时跳跃，永不排队**——否则 200 tok/s 的模型输出会让动画播到下个世纪。

**与 SwiftUI 刷新周期的分工（评审 C8，实施前必读）**：流式气泡的每帧更新
**完全绕过 SwiftUI 状态系统**——display link 回调里只做"缓冲 → 直接写
`UITextViewRepresentable` 持有的 text storage"，不碰任何 `@State`/
`@Observable`（否则每帧触发一次 body 重算 + diff，和 UIKit 布局在同一帧
双重刷新）。SwiftUI 只负责气泡外壳（背景、圆角、布局槽位），气泡高度变化
通过 `sizeThatFits` 回报，按帧合批后驱动一次外层更新。原则一句话：
**高频路径走 UIKit 直通，SwiftUI 只管低频结构**。

**渲染路径唯一（评审 A7 定案）**：M4 完成后**所有**消息气泡（流式与
非流式）统一走本引擎——非流式消息就是"append 一次、无逐字动画"的退化
路径。004 的纯 SwiftUI `Text` 实现视为 M1 原型，M4 删除。两套排版引擎
并存必然产生 emoji 行高/RTL/markdown 解析的微妙差异（iMessage 出过此类
事故），不留双路径。

配套细节：光标闪烁 caret、markdown 渐进解析（代码块/列表边流边成型，
半开语法的容错解析）、流式中禁用文本选择（排版未稳定）。

### 挑战 B：滚动锚定（scroll anchoring）—— 聊天 UI 的隐形难题

流式增长的气泡在列表**底部**：内容每帧变高，滚动位置怎么办？

- **贴底规则**：用户在底部（距底 < 1 屏）→ 内容增长时自动贴底跟随；
- **阅读保护**：用户主动上滚 → 立即停止跟随（哪怕正在流式），新内容折叠为
  "↓ 新消息" pill，点击回底；**任何时候用户手势优先级最高**，动画抢滚动
  条是聊天 UI 最招人恨的 bug；
- 倒置列表（inverted list）技巧的替代：iOS 17+ 用 `scrollPosition` +
  `defaultScrollAnchor(.bottom)`，避免倒置带来的手势/上下文菜单镜像坑；
- 键盘升降、气泡高度变化（图片加载完成）时的锚点补偿——统一收敛到一个
  `ScrollAnchorController`，禁止散落各处各自 `scrollTo`。

### 挑战 C：实时交互模式（micro-interactions 全集）

- **乐观交互**：reactions（长按表情，本地立即上屏走 outbox 同步——复用
  004 的模式，学习点是把 outbox pattern 泛化到"消息之外的用户动作"）；
  消息撤回/编辑的中间态呈现（"撤回中…"）；
- 手势系统：swipe-to-reply、长按菜单、拖拽多选——手势之间的冲突仲裁表
  （谁吃掉谁）显式写成文档，不靠试出来；
- Haptics 纪律：发送成功轻击、撤回 warning、连接恢复 success——集中一个
  `HapticsPolicy`，禁止组件内随手 `impactOccurred`（体验一致性）；
- 可中断性：一切动画可被下一个状态打断（`withAnimation` 的可组合性），
  流式可取消（为 013 的停止生成按钮预留交互位）。

### 挑战 D：复杂状态机设计 —— 从"隐式布尔汤"到显式 FSM

App 里每个异步面都是状态机，但通常写成 `isLoading` / `isRetrying` /
`hasError` 三个 Bool 的 2³ 组合，其中一半是非法状态。

**解法：统一的轻量 FSM 范式（不引外部框架，自己写 ~100 行泛型）：**

```swift
protocol StateMachine {
  associatedtype State: Equatable
  associatedtype Event
  associatedtype Effect
  static func transition(_ s: State, _ e: Event) -> (State, [Effect])
}
```

- **纯函数转移表**：`(State, Event) → (State, [Effect])`，副作用以 Effect
  值返回由 runtime 执行——转移表因此可穷举单测（给定全部 State×Event 组合，
  断言输出），这是本 spec 最重要的工程收益；
- 全量改造清单：连接状态机（004 已有，迁入此范式）、消息生命周期
  （新增 streaming 分支：`streaming(partial) → sent`，013 使用）、
  上传状态机（009 的分片续传）、音频播放（全局单实例）、同步引擎阶段机；
- **机与机的组合：消息驱动，不是层级（评审 C5 定案）**。上面的泛型协议是
  纯扁平机——`State` 是单一 Equatable 类型，编码不了 statechart 的复合状态/
  正交区域/历史伪态。本项目**不做层级**（自研层级机的工作量够单独成库，
  ROI 为负）：连接机 disconnected 时产出 `Effect.connectionLost`，
  会话页呈现机的 runtime 收到后向自己注入 `.wentOffline` 事件——
  **Effect 是机与机之间唯一的耦合面**（actor 模型的消息驱动思想），
  任何机不得直接读另一台机的 State。状态图文档只画实现了的东西；
- 非法转移策略：debug 断言崩溃（开发期暴露），release 记录遥测并保持原态
  （线上不崩）。每台机器附一张状态图进 `docs/statecharts/`，格式统一
  **Mermaid `stateDiagram-v2`**（纯文本可 diff、GitHub 原生渲染）；
  Mermaid 表达不了的（如 Effect 清单）用图旁的表格补充，不硬画。

## 3. 范围

**In**：流式文本渲染引擎（TextKit 2 桥接组件）、ScrollAnchorController、
reactions/撤回/编辑的乐观交互（含服务端配套：编辑/撤回作为特殊消息类型走
003 管道）、手势仲裁表、HapticsPolicy、FSM 泛型 + 五台机器改造 + 状态图文档。
**Out**：AI 相关（013/014 只消费本 spec 的渲染引擎）、主题/深色模式打磨
（backlog）、iPad 分栏适配（backlog）。

## 4. 验收标准

1. 打字机压力实验：注入 200 tok/s × 2,000 token 长文本，全程掉帧 < 5%
   （Instruments），降级策略触发计数 > 0（证明背压真在工作而不是碰巧够快）。
2. 滚动锚定实验矩阵：流式中贴底跟随 / 上滚后不跟随且 pill 出现 /
   流式中图片加载完成不跳位——三项真机录屏留档。
3. FSM 转移表单测覆盖 100%（穷举 State×Event）；随机事件序列 fuzz 1w 步
   无非法状态、无 Effect 泄漏（未执行即丢弃）。
4. reactions 飞行模式实验：离线点表情 → 本地立即显示 → 恢复网络后对端
   一致（outbox 泛化的证明）。
5. 手势仲裁：文档表中每一对冲突手势有对应 XCUITest 断言。

## 5. 测试计划

FSM 全部机器的转移表单测 + fuzz；渲染引擎的性能基准进 CI（XCTest
metrics，回归 > 10% 报警）；交互实验为真机 runbook 录屏归档。
