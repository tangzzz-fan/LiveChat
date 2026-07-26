# LiveChat iOS

按 Spec 13 分层的客户端。**当前进入从零重写**（见 [0035](../issues/0035-ios-client-rewrite.md)）：旧 `AppCore` / `Packages` 骨架将被清空替换。

## 决策与文档

| 文档 | 用途 |
|------|------|
| [Spec 13](../Specs/13-iOS客户端架构设计.md) | 模块、状态机、生命周期 |
| [ios-client-rewrite.md](../docs/ios-client-rewrite.md) | 拍板：SPM、GRDB、TGReduxKit、原生 WS、后台模型 |
| [ios-high-load-client.md](../docs/ios-high-load-client.md) | 高负载/弱网坑点与横切验收 |
| [iOS多端接入评估](../docs/iOS多端接入评估与实现.md) | 多端联调与服务端能力 |
| [API参考](../docs/API参考.md) | HTTP / 协议面 |

## 工程形态（目标）

```
薄 App target (SwiftUI 入口)
  └── ChatPresentation (TGReduxKit Store)
        └── ChatApplication (Use Cases)
              ├── ChatDomain
              └── ChatInfrastructure (GRDB, URLSession, WebSocketTransport, Keychain)
```

本地 SPM path（已下载）：

- `TG Libraries/GRDB.swift` @ v7.7.1  
- `TG Libraries/swift-protobuf` @ 1.31.0  
- `TG Libraries/TGReduxKit`

## 状态

- 设计票 [0036](../issues/0036-ios-rewrite-design.md) ✅  
- 下一票 [0037](../issues/0037-ios-spm-scaffold.md)：脚手架；**创建 Xcode 工程需你本地完成（HITL）**  
- 旧功能票 0022–0025 / 0027–0028 已 `superseded`

## 编译（脚手架就绪后）

```bash
# 具体 scheme / 工程路径以 0037 落地后的 ios/README 为准
xcodebuild -project <LiveChat.xcodeproj> -scheme LiveChat -destination 'platform=iOS Simulator,name=iPhone 16' build
```
