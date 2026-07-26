import SwiftUI
import TGReduxKit

public struct LoginView: View {
    @Environment(Store<AppState, AppAction>.self) private var store

    public init() {}

    public var body: some View {
        Form {
            Section("登录") {
                TextField(
                    "手机号 E.164",
                    text: store.binding(get: { $0.phoneInput }, send: { .setPhoneInput($0) })
                )
                #if os(iOS)
                .textInputAutocapitalization(.never)
                .keyboardType(.phonePad)
                #endif
                .autocorrectionDisabled()
                .disabled(store.state.authPhase == .code)

                if store.state.authPhase == .code {
                    TextField(
                        "验证码",
                        text: store.binding(get: { $0.codeInput }, send: { .setCodeInput($0) })
                    )
                    #if os(iOS)
                    .keyboardType(.numberPad)
                    .textInputAutocapitalization(.never)
                    #endif
                }

                if let error = store.state.authError {
                    Text(error)
                        .foregroundStyle(.red)
                        .font(.footnote)
                }

                Button(store.state.authPhase == .phone ? "获取验证码" : "登录") {
                    if store.state.authPhase == .phone {
                        store.dispatch(.requestCodeTapped)
                    } else {
                        store.dispatch(.verifyCodeTapped)
                    }
                }
                .disabled(store.state.isAuthBusy || store.state.phoneInput.isEmpty)

                if store.state.authPhase == .code {
                    Button("返回改手机号") {
                        store.dispatch(.resetAuthToPhone)
                    }
                }
            }

            Section("提示") {
                Text("本地 mock 验证码通常为 123456")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                if let banner = store.state.connectionBanner {
                    Text(banner)
                        .font(.caption.monospaced())
                        .textSelection(.enabled)
                }
            }
        }
    }
}

public struct LoggedInHomeView: View {
    @Environment(Store<AppState, AppAction>.self) private var store

    public init() {}

    public var body: some View {
        NavigationStack {
            List {
                Section("会话") {
                    LabeledContent("user_id", value: store.state.userID.map(String.init) ?? "-")
                    LabeledContent("device_id", value: store.state.deviceID ?? "-")
                }
                Section("本账号设备 GET /v1/devices") {
                    if store.state.deviceSummaries.isEmpty {
                        Text("暂无设备列表")
                            .foregroundStyle(.secondary)
                    } else {
                        ForEach(store.state.deviceSummaries, id: \.self) { line in
                            Text(line)
                                .font(.caption.monospaced())
                        }
                    }
                    Button("刷新设备列表") {
                        store.dispatch(.refreshDevicesTapped)
                    }
                }
            }
            .navigationTitle("LiveChat")
            .toolbar {
                ToolbarItem(placement: .primaryAction) {
                    Button("退出") {
                        store.dispatch(.logoutTapped)
                    }
                }
            }
        }
    }
}
