# SPEC-008 — APNs 推送：iOS 后台可达性

> 状态: Draft | Milestone: M2 | 依赖: SPEC-003, 004 | 被依赖: 011(加密推送内容)

## 1. 背景与动机（Why）

iOS 切后台约 30 秒后 socket 必死，这是系统行为，任何"保活黑科技"都是
与平台对抗（且会被拒审）。**唯一正道：APNs。** 本 spec 学的是推送的正确
定位——推送是"门铃"不是"快递"：它负责叫醒用户/App，数据本身永远以
收件箱同步为准（SPEC-003 的推拉结合在移动端的延伸）。

## 2. 核心挑战与典型解法

### 挑战 A："在线不推、离线才推"的判定竞态

用户刚锁屏，socket 还没超时断开，服务端路由表显示"在线"，于是走长连接推
——但设备实际已收不到。消息躺在收件箱里没人叫醒用户。

**解法：双保险投递（WhatsApp 模式的简化版）：**
- 判定基准：presence（SPEC-007）+ iOS 客户端**主动上报**生命周期
  （`applicationDidEnterBackground` 时发一帧 `GoingBackground` 再断连
  ——比等 75s 心跳超时精确得多）；
- 竞态兜底：走长连接推送后 **10s 内未收到该设备的 AckDelivered** →
  补发一次 APNs（collapse-id 去重，见挑战 C）。宁可偶尔重复叫门，
  不可漏叫——重复由客户端 sync 幂等消化。

### 挑战 B：Token 生命周期

- 设备注册：App 启动获取 device token → 上报绑定 `(user_id, device_id, token)`；
- token 会变（重装/恢复备份/系统升级）：每次启动都上报，服务端 upsert；
- APNs 反馈 `410 Unregistered` → 立即删 token 停止投递（继续打死 token
  会被 Apple 降级信誉，这是真实生产事故来源）；
- 登出必须删 token——否则前任账号的消息推到现任设备上，经典隐私事故。

### 挑战 C：推送内容与协作唤醒

- **alert push**（有横幅）：标题=发送者名，正文=消息摘要；`mutable-content: 1`
  + Notification Service Extension 留出 E2EE 解密的位置（011 里推送体只有
  msg_id，扩展进程本地解密渲染——WhatsApp/Signal 正是这么做的）；
- `apns-collapse-id = conv_id`：同会话连续 10 条只留最新横幅，防轰炸；
- badge 数：服务端算全局未读（Redis 求和），随每条推送更新；
- **background push**（`content-available: 1`, 静默）：用于唤醒 App 预拉取
  ——但 iOS 对静默推送限流（每小时个位数次），只作锦上添花，不承载正确性；
- 通知点击 → deep link 直达对应会话（`conv_id` 塞 payload）。

### 挑战 D：Push Worker 服务形态

- 独立服务 `pushworker`：消费"需要推送"事件（msgsvc 投递判定产出），
  调 APNs HTTP/2 API（token-based auth, .p8 密钥）；
- HTTP/2 连接复用 + 并发流控（APNs 允许 1 连接多路复用，不要每推一连接）；
- 失败重试队列（网络错误重试，4xx 按语义处理不盲重）；
- 本地开发：APNs sandbox 环境 + 真机（模拟器 iOS 16+ 也支持推送调试，
  但压测走 mock APNs server 记录请求即可）。

## 3. 范围

**In**：pushworker 服务、token 注册/失效链路、投递判定 hook（003 预留处）、
双保险兜底、collapse/badge/deep-link、iOS 端 UNUserNotificationCenter 集成、
mock APNs server（供集成测试与压测）。
**Out**：E2EE 推送解密扩展的实现（011，本 spec 只留 `mutable-content` 结构）、
Android FCM（D8 排除）、营销类推送（永不做）。

## 4. 验收标准（真机实验为主）

1. 锁屏设备收到 1:1 消息 → 横幅出现 < 5s；点击横幅直达对应会话且消息已就位
   （sync 在启动路径完成）。
2. 竞态实验：锁屏后立即（<75s 心跳窗口内）发消息 → 兜底 APNs 生效，
   横幅仍 < 15s 到达（10s 兜底窗 + APNs 延迟）。
3. 同会话快速发 20 条 → 通知中心只见最新 1 条横幅（collapse 生效），
   badge 数与实际未读一致。
4. 登出后向该账号发消息 → 设备无任何推送（token 已删，mock server 证明
   零请求）。
5. 410 处理：mock APNs 返回 410 → token 被清除 → 后续零投递尝试。
6. pushworker 吞吐：mock 模式 ≥ 2,000 push/s（群消息扇出推送的容量地板）。

## 5. 测试计划

投递判定与兜底定时器单测；pushworker 对 APNs 各状态码的处理矩阵测试
（mock server）；真机 runbook 覆盖验收 1~4；sandbox 证书与 .p8 的配置
文档化（一次性摩擦点，写清楚省未来的自己两小时）。
