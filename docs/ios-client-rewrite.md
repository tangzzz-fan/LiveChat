# iOS 客户端从零重写 — 决策与边界

对齐 [Spec 13](../Specs/13-iOS客户端架构设计.md)、[ios-high-load-client.md](./ios-high-load-client.md)、父票 [0035](../issues/0035-ios-client-rewrite.md)。  
旧 `ios/` 骨架与实现票 **0022–0025 / 0027–0028** 作废；服务端 [0026](../issues/0026-server-direct-conversation-api.md) 保留。

## 1. 拍板摘要

| 项 | 决策 |
|----|------|
| 工程形态 | 纯 SPM 多 package + 薄 App target |
| 本地 DB | GRDB（单 `DatabaseQueue` + WAL） |
| 协议 | swift-protobuf（与网关帧对齐） |
| Presentation 状态 | TGReduxKit（有边界，见 §3） |
| 旧代码 | 清空 `ios/` 后重写 |
| 长连接实现 | **默认** `URLSessionWebSocketTask`，藏在 `WebSocketTransport` 后 |
| 后台模型 | **不硬撑 WS**；进后台断开，靠 APNs 唤醒 + 增量 sync |

## 2. 依赖（本地 path，已下载）

路径根：`/Users/bigapple/Developments/TG Libraries/`

| 包 | 本地版本 | 用途 |
|----|----------|------|
| GRDB.swift | v7.7.1 | 本地 SQLite |
| swift-protobuf | 1.31.0 | WS/HTTP protobuf |
| TGReduxKit | 本地 2.0.0 系 | Store / ScopedStore / Middleware |

Package 引用示例（相对路径按仓库实际布局调整）：

```swift
.package(path: "../../TG Libraries/GRDB.swift"),
.package(path: "../../TG Libraries/swift-protobuf"),
.package(path: "../../TG Libraries/TGReduxKit"),
```

**默认不引入**：Alamofire、Starscream、gRPC-Swift、KeychainAccess（Keychain 自写薄封装）。  
**非 SPM 工具**：`protoc` + `protoc-gen-swift`（`brew install protobuf swift-protobuf`）；生成的 `.pb.swift` 建议入库，日常可离线编。

Starscream / NWConnection：**仅当实测撞到原生 WS 硬限时**再加实现类，不改上层。

## 3. Redux 边界（必须守住）

```text
GRDB = 数据真相          Store = 视图真相
全量消息 / 游标          当前可见页投影、草稿、横幅、导航、登录流
批量写事务               纯 Reducer
Infra 后台队列           Middleware → UseCase → 写 DB，再观察投影
```

桥接：`ValueObservation`（去抖 16–33ms）→ 投影变更 →（可选）`dispatch` 进 Store。  
**反模式**：根 State 塞整会话；WS 每帧 dispatch；Reducer 内 DB/网络。

## 4. WebSocket：对比结论与落点

| | URLSessionWebSocketTask | Starscream | NWConnection+WS |
|--|-------------------------|------------|-----------------|
| 依赖 | 零 | 第三方 | 零 |
| 断线检测 | 弱，需应用层心跳 | 较好 | 最强 |
| 压缩 | 不可控 | 有 | 自配 |
| 本仓选择 | **默认实现** | 逃生口 | 后期弱网压榨 |

抽象：

```swift
protocol WebSocketTransport {
    var events: AsyncStream<TransportEvent> { get }
    func connect() async throws
    func send(_ frame: Data) async throws
    func close()
}
```

应用层心跳（协议 opcode）+ `NWPathMonitor` 兜底原生断线检测弱点。

## 5. 前台长连 / 后台唤醒

与真实 IM 一致，也与 Spec 13 §8.2 修订一致：

1. **前台**：WS 实时投递 + 应用层心跳。  
2. **进后台**：断开或允许系统掐断；不延长心跳硬撑。  
3. **唤醒**：Silent Push / 点击可见推送 → 只跑增量 sync（预算内）。  
4. **回前台**：重连（退避 + jitter + single-flight）+ 立刻 sync。

## 6. 与高负载文档的横切验收（实现票必须带）

见 [ios-high-load-client.md](./ios-high-load-client.md)：首屏约 1k 历史主线程不长时间卡顿；弱网发送不丢（本地 queued + 幂等）；突发投递批量落库 + observation 去抖不掉帧；重连不形成本地风暴。

## 7. HITL：Xcode 工程

Agent **不能**可靠代建完整 Xcode App 工程与签名。到 [0037](../issues/0037-ios-spm-scaffold.md) 时会明确请你：

1. 新建 App 工程（iOS 17+，SwiftUI）  
2. 把薄 App target 挂上各 SPM package  
3. 配置 ATS / 明文 HTTP 开发例外、Capabilities（Push 等按阶段开）

Agent 侧负责：清空旧 `ios/`、写好各 package 的 `Package.swift` 与目录骨架、本地 path 依赖、proto 生成脚本说明。

## 8. 票序

| ID | 内容 |
|----|------|
| [0035](../issues/0035-ios-client-rewrite.md) | 父票 |
| [0036](../issues/0036-ios-rewrite-design.md) | 本决策落入 Spec + 本文 |
| [0037](../issues/0037-ios-spm-scaffold.md) | 脚手架（含你建 Xcode 工程） |

功能垂直切片（登录 / 发送 / sync / WS / 推送）在 0037 完成后按 Spec 13 重新拆票，不再沿用 0022–0028。
