import Foundation
import TGReduxKit
import ChatApplication
import ChatInfrastructure

@MainActor
func makeAuthMiddleware(services: AppServices) -> Middleware<AppState, AppAction> {
    { store, action, next in
        next(action)
        guard case .auth(let authAction) = action else { return }

        switch authAction {
        case .bootstrap:
            store.runTask(id: CancellationID("auth.bootstrap")) {
                do {
                    let deviceID = try services.auth.currentDeviceID()
                    if let session = try services.auth.restoredSession() {
                        await store.dispatch(
                            .auth(.sessionRestored(userID: session.userID, deviceID: deviceID))
                        )
                        await loadDevices(store: store, services: services, token: session.accessToken)
                        await store.dispatch(.chat(.syncTapped))
                        await store.dispatch(.chat(.realtimeEnsureStarted))
                        await store.dispatch(.chat(.registerPushTokenTapped))
                    } else {
                        await store.dispatch(.auth(.setDeviceBanner("device: \(deviceID)")))
                    }
                } catch {
                    await store.dispatch(.auth(.failed(error.localizedDescription)))
                }
            }
        case .requestCodeTapped:
            let phone = store.state.auth.phoneInput.trimmingCharacters(in: .whitespacesAndNewlines)
            store.runTask(id: CancellationID("auth.requestCode")) {
                await store.dispatch(.auth(.busy(true)))
                do {
                    _ = try await services.auth.requestCode(phone: phone)
                    await store.dispatch(.auth(.codeRequested))
                } catch {
                    await store.dispatch(.auth(.failed(error.localizedDescription)))
                }
            }
        case .verifyCodeTapped:
            let phone = store.state.auth.phoneInput.trimmingCharacters(in: .whitespacesAndNewlines)
            let code = store.state.auth.codeInput.trimmingCharacters(in: .whitespacesAndNewlines)
            store.runTask(id: CancellationID("auth.verifyCode")) {
                await store.dispatch(.auth(.busy(true)))
                do {
                    let deviceID = try services.auth.currentDeviceID()
                    let tokens = try await services.auth.verifyCode(
                        phone: phone,
                        code: code,
                        deviceID: deviceID,
                        platform: "ios"
                    )
                    await store.dispatch(
                        .auth(.loginSucceeded(userID: tokens.userID, deviceID: deviceID))
                    )
                    await loadDevices(store: store, services: services, token: tokens.accessToken)
                    await store.dispatch(.chat(.syncTapped))
                    await store.dispatch(.chat(.realtimeEnsureStarted))
                    await store.dispatch(.chat(.registerPushTokenTapped))
                } catch {
                    await store.dispatch(.auth(.failed(error.localizedDescription)))
                }
            }
        case .logoutTapped:
            store.runTask(id: CancellationID("auth.logout")) {
                await services.realtime.stop(reason: "logout")
                try? await services.auth.logout()
                await store.dispatch(.auth(.loggedOut))
            }
        case .refreshDevicesTapped:
            store.runTask(id: CancellationID("auth.devices")) {
                guard let session = try? services.auth.restoredSession() else { return }
                await loadDevices(store: store, services: services, token: session.accessToken)
            }
        default:
            break
        }
    }
}

@MainActor
private func loadDevices(
    store: Store<AppState, AppAction>,
    services: AppServices,
    token: String
) async {
    do {
        let devices = try await services.auth.listDevices(accessToken: token)
        let lines = devices.map { d in
            let mark = (d.isCurrent == true) ? " *" : ""
            return "\(d.deviceID) (\(d.platform)) sv=\(d.sessionVersion)\(mark)"
        }
        await store.dispatch(.auth(.devicesLoaded(lines)))
    } catch {
        await store.dispatch(.auth(.failed(error.localizedDescription)))
    }
}
