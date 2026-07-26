# LiveChat iOS（重写脚手架 · 0037）

相关：[`API参考.md`](../docs/API参考.md) · [`ios-client-rewrite.md`](../docs/ios-client-rewrite.md) · **手工联调**：[ios-app-testing.md](../docs/ios-app-testing.md)

## 目录

```text
ios/
├── Packages/
│   ├── ChatDomain/           # 实体、状态机、Repository 协议
│   ├── ChatInfrastructure/   # GRDB、WebSocketTransport、SwiftProtobuf
│   ├── ChatApplication/      # UseCase 组装（脚手架）
│   └── ChatPresentation/     # TGReduxKit AppState / Store
├── App/Sources/LiveChatApp.swift   # 薄入口样板（加入 Xcode App target）
├── Generated/                # protoc 生成物（脚本产出后入库）
├── scripts/gen_proto.sh
└── README.md
```

## 本地依赖（已下载）

相对 `ios/Packages/<Pkg>/Package.swift`：

| 包 | path |
|----|------|
| GRDB | `../../../../TG Libraries/GRDB.swift` @ v7.7.1 |
| swift-protobuf | `../../../../TG Libraries/swift-protobuf` @ 1.31.0 |
| TGReduxKit | `../../../../TG Libraries/TGReduxKit` |

## 命令行验证（无需 Xcode App）

```bash
cd ios/Packages/ChatDomain && swift test
cd ../ChatInfrastructure && swift test
cd ../ChatApplication && swift test
cd ../ChatPresentation && swift test
```

## Protobuf 生成

```bash
proxy_on
brew install protobuf swift-protobuf   # 本机若尚未安装
./ios/scripts/gen_proto.sh
```

生成物在 `ios/Generated/`；后续功能票再挂进 `ChatInfrastructure` target。

## Xcode 工程（已创建，0037 已验收）

- 工程：`ios/LiveChat/LiveChat.xcodeproj`，scheme `LiveChat`
- Bundle ID：`com.tango.LiveChat`
- App target 链接的包产物：`TGReduxKit`、`ChatPresentation`、`ChatApplication`、`ChatInfrastructure`
  （**不要**再挂 `GRDB-dynamic` / `SwiftProtobufPluginLibrary` / `protoc` / `protoc-gen-swift`，会导致 iOS 构建失败）
- ATS：`LiveChat/Info.plist` 已含 `NSAllowsLocalNetworking = YES`

联编 / 双模拟器（iOS 26.5）：

```bash
xcodebuild -project ios/LiveChat/LiveChat.xcodeproj -scheme LiveChat \
  -destination 'platform=iOS Simulator,name=iPhone 17 Pro,OS=26.5' build
xcodebuild -project ios/LiveChat/LiveChat.xcodeproj -scheme LiveChat \
  -destination 'platform=iOS Simulator,name=iPhone 17 Pro Max,OS=26.5' build

# 安装 + 启动（多端联调用两台）
xcrun simctl boot "iPhone 17 Pro"; xcrun simctl boot "iPhone 17 Pro Max"
APP=$(find ~/Library/Developer/Xcode/DerivedData/LiveChat-*/Build/Products/Debug-iphonesimulator -maxdepth 1 -name LiveChat.app | head -1)
xcrun simctl install booted "$APP"
xcrun simctl launch booted com.tango.LiveChat
```

## 状态

- [0036](../issues/0036-ios-rewrite-design.md) / [0037](../issues/0037-ios-spm-scaffold.md) ✅  
- [0038](../issues/0038-ios-auth-otp-keychain-login.md) OTP 登录 ✅  
- [0039](../issues/0039-ios-local-first-send-direct.md) 本地优先发送 ✅  
- **当前 frontier**：[0040](../issues/0040-ios-incremental-sync.md) 增量 sync  
- 后续：0041 → 0042（见 [0035](../issues/0035-ios-client-rewrite.md)）

### Redux 模块边界

```text
AppState { auth, chat }
AppAction { .auth(AuthAction), .chat(ChatAction) }
Features/Auth/   state + reducer + middleware + LoginView
Features/Chat/   state + reducer + middleware + list/thread views
App/AppStore.swift  combineReducers + pullback + factory
```

禁止把新 Feature 的 Action 继续塞进单一巨型文件；新增能力先开 Feature 目录。

## 边界提醒

- Store 只放视图真相；消息全量在 GRDB  
- 默认 `URLSessionWebSocketTransport`；不引入 Starscream  
- 进后台不硬撑 WS（功能票实现）
