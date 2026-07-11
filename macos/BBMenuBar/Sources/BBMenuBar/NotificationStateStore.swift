import BBMenuBarCore
import Foundation

actor UserDefaultsNotificationStateStore: NotificationStateStore {
  private let key = "notification-delivery-state-v1"

  func load() async throws -> NotificationDeliveryState? {
    guard let data = UserDefaults.standard.data(forKey: key) else { return nil }
    return try JSONDecoder().decode(NotificationDeliveryState.self, from: data)
  }

  func save(_ state: NotificationDeliveryState) async throws {
    UserDefaults.standard.set(try JSONEncoder().encode(state), forKey: key)
  }
}
