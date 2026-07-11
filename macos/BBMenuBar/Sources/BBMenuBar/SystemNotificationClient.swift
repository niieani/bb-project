import BBMenuBarCore
import Foundation
@preconcurrency import UserNotifications

final class SystemNotificationClient: NSObject, NotificationClient, @unchecked Sendable {
  private let center: UNUserNotificationCenter
  private let responseDelegate: NotificationResponseDelegate

  @MainActor
  init(router: AttentionWindowRouter, center: UNUserNotificationCenter = .current()) {
    self.center = center
    responseDelegate = NotificationResponseDelegate(router: router)
    super.init()
    center.delegate = responseDelegate
  }

  func authorizationStatus() async -> NotificationAuthorization {
    let settings = await center.notificationSettings()
    return switch settings.authorizationStatus {
    case .notDetermined: .notDetermined
    case .denied: .denied
    case .authorized, .provisional, .ephemeral: .authorized
    @unknown default: .unavailable
    }
  }

  func requestAuthorization() async throws -> Bool {
    try await center.requestAuthorization(options: [.alert, .sound])
  }

  func submit(_ request: FleetNotificationRequest) async throws {
    let content = UNMutableNotificationContent()
    content.title = request.title
    content.body = request.body
    content.sound = .default
    content.userInfo = ["payload": try NotificationPayloadCodec.encode(request)]
    try await center.add(
      UNNotificationRequest(
        identifier: "bb-fleet-\(request.fingerprint)", content: content, trigger: nil))
  }
}

private final class NotificationResponseDelegate: NSObject, UNUserNotificationCenterDelegate,
  @unchecked Sendable
{
  private let handler: NotificationResponseHandler

  @MainActor init(router: AttentionWindowRouter) {
    handler = NotificationResponseHandler(router: router)
  }

  func userNotificationCenter(
    _: UNUserNotificationCenter,
    willPresent _: UNNotification,
    withCompletionHandler completionHandler: @escaping (UNNotificationPresentationOptions) -> Void
  ) {
    completionHandler([.banner, .sound])
  }

  func userNotificationCenter(
    _: UNUserNotificationCenter,
    didReceive response: UNNotificationResponse,
    withCompletionHandler completionHandler: @escaping () -> Void
  ) {
    guard
      let payload = response.notification.request.content.userInfo["payload"] as? String,
      let items = try? NotificationPayloadCodec.decode(payload).items
    else {
      completionHandler()
      return
    }
    let completion = NotificationResponseCompletion(completionHandler)
    Task { [handler, completion] in
      defer { completion.call() }
      await handler.route(items: items)
    }
  }
}

private final class NotificationResponseCompletion: @unchecked Sendable {
  private let completion: () -> Void

  init(_ completion: @escaping () -> Void) { self.completion = completion }
  func call() { completion() }
}
