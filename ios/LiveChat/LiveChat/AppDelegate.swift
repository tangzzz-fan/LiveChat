import UIKit
import UserNotifications
import TGReduxKit
import ChatPresentation
import ChatApplication
import ChatInfrastructure

/// 承接 APNs 回调与静默推送。模拟器无 UI「模拟静默唤醒」亦可演示同一 sync 路径。
final class AppDelegate: NSObject, UIApplicationDelegate, UNUserNotificationCenterDelegate {
    var store: StoreOfApp?
    var services: AppServices?

    func application(
        _ application: UIApplication,
        didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]? = nil
    ) -> Bool {
        UNUserNotificationCenter.current().delegate = self
        return true
    }

    func application(
        _ application: UIApplication,
        didRegisterForRemoteNotificationsWithDeviceToken deviceToken: Data
    ) {
        let hex = PushTokenFactory.hexToken(from: deviceToken)
        store?.dispatch(.chat(.pushTokenReceived(hex)))
    }

    func application(
        _ application: UIApplication,
        didFailToRegisterForRemoteNotificationsWithError error: Error
    ) {
        // 模拟器常见失败：保留 mock token 路径即可。
        store?.dispatch(
            .chat(.setPushTokenBanner("APNs 注册失败（可用 mock）· \(error.localizedDescription)"))
        )
    }

    /// Silent / 可见推送唤醒：只跑 SyncExecutor（与 0040 同一路径），不启 WS。
    func application(
        _ application: UIApplication,
        didReceiveRemoteNotification userInfo: [AnyHashable: Any],
        fetchCompletionHandler completionHandler: @escaping (UIBackgroundFetchResult) -> Void
    ) {
        guard let store, let services else {
            completionHandler(.failed)
            return
        }
        Task {
            let outcome = await services.silentWake.handleWake(reason: "apns_remote")
            switch outcome {
            case .success(let result):
                await MainActor.run {
                    store.dispatch(
                        .chat(.syncFinished(applied: result.appliedCount, cursor: result.cursor))
                    )
                }
                completionHandler(result.appliedCount > 0 ? .newData : .noData)
            case .failure:
                completionHandler(.failed)
            }
        }
    }

    func requestNotificationAuthorizationAndRegister() {
        UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .sound, .badge]) { granted, _ in
            guard granted else { return }
            DispatchQueue.main.async {
                UIApplication.shared.registerForRemoteNotifications()
            }
        }
    }
}
