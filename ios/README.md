# LiveChat iOS（重写脚手架 · 0037）

纯 SPM 多 package + 薄 App target。决策见 [`docs/ios-client-rewrite.md`](../docs/ios-client-rewrite.md) / Spec 13。

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

## HITL：请你现在创建 Xcode 工程

Agent 已写好 packages 与 `App/Sources/LiveChatApp.swift`，**还差可运行的 `.xcodeproj`**。请按下面做完后，把工程路径发回对话：

1. **Xcode → File → New → Project → App**
   - Product Name: `LiveChat`
   - Interface: **SwiftUI**
   - Language: **Swift**
   - 最低系统：**iOS 17**
2. **保存位置**：建议  
   `/Users/bigapple/Developments/LiveChat/ios/LiveChat/`  
   （会得到 `ios/LiveChat/LiveChat.xcodeproj`）
3. **删除** Xcode 自动生成的 `ContentView.swift` / 默认 `*App.swift`（若与样板冲突）
4. **把** `ios/App/Sources/LiveChatApp.swift` **拖进 App target**（勾选 Copy 与否均可，推荐引用仓库内路径）
5. **File → Add Package Dependencies → Add Local…**，依次添加：
   - `ios/Packages/ChatPresentation`（会递归拉到 Application / Infrastructure / Domain）
   - 若 Xcode 未自动解析本地传递依赖，再分别 Add Local：`ChatApplication`、`ChatInfrastructure`、`ChatDomain`
6. App target → **General → Frameworks**：勾选 `ChatPresentation`（及需要直接 `import` 的库）
7. **ATS / 明文 HTTP（开发）**：App target → Info → 增加例外，或在自定义 Info.plist：

```xml
<key>NSAppTransportSecurity</key>
<dict>
  <key>NSAllowsLocalNetworking</key>
  <true/>
</dict>
```

8. 选模拟器 **Run**；应看到 “LiveChat / SPM scaffold ready”

完成后回复例如：`工程已建好：ios/LiveChat/LiveChat.xcodeproj`，我再跑 `xcodebuild` 做联编验收并关闭 0037 AC。

## 边界提醒

- Store 只放视图真相；消息全量在 GRDB  
- 默认 `URLSessionWebSocketTransport`；不引入 Starscream  
- 进后台不硬撑 WS（功能票实现）
