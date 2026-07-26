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
                    text: store.binding(
                        get: { $0.auth.phoneInput },
                        send: { .auth(.setPhoneInput($0)) }
                    )
                )
                #if os(iOS)
                .textInputAutocapitalization(.never)
                .keyboardType(.phonePad)
                #endif
                .autocorrectionDisabled()
                .disabled(store.state.auth.phase == .code)

                if store.state.auth.phase == .code {
                    TextField(
                        "验证码",
                        text: store.binding(
                            get: { $0.auth.codeInput },
                            send: { .auth(.setCodeInput($0)) }
                        )
                    )
                    #if os(iOS)
                    .keyboardType(.numberPad)
                    .textInputAutocapitalization(.never)
                    #endif
                }

                if let error = store.state.auth.errorMessage {
                    Text(error)
                        .foregroundStyle(.red)
                        .font(.footnote)
                }

                Button(store.state.auth.phase == .phone ? "获取验证码" : "登录") {
                    if store.state.auth.phase == .phone {
                        store.dispatch(.auth(.requestCodeTapped))
                    } else {
                        store.dispatch(.auth(.verifyCodeTapped))
                    }
                }
                .disabled(store.state.auth.isBusy || store.state.auth.phoneInput.isEmpty)

                if store.state.auth.phase == .code {
                    Button("返回改手机号") {
                        store.dispatch(.auth(.resetToPhone))
                    }
                }
            }

            Section("提示") {
                Text("本地 mock 验证码通常为 123456")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                if let banner = store.state.auth.deviceBanner {
                    Text(banner)
                        .font(.caption.monospaced())
                        .textSelection(.enabled)
                }
            }
        }
    }
}
