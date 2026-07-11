import Foundation

public enum NotificationAuthorization: Sendable {
  case notDetermined, authorized, denied, unavailable
}

public struct FleetNotificationItem: Codable, Hashable, Sendable {
  public let machineID: String
  public let repoKey: String
  public let name: String
  public let reason: String
}

public struct FleetNotificationRequest: Equatable, Sendable {
  public let fingerprint: String
  public let title: String
  public let body: String
  public let items: [FleetNotificationItem]
}

public protocol NotificationClient: Sendable {
  func authorizationStatus() async -> NotificationAuthorization
  func requestAuthorization() async throws -> Bool
  func submit(_ request: FleetNotificationRequest) async throws
}

@MainActor
public protocol NotificationRouter: Sendable {
  func show(items: [FleetNotificationItem])
}

public struct NotificationResponseHandler: Sendable {
  private let router: any NotificationRouter

  public init(router: any NotificationRouter) { self.router = router }

  public func route(items: [FleetNotificationItem]) async {
    await router.show(items: items)
  }

  public func route(encodedPayload: String) async throws {
    await router.show(items: try NotificationPayloadCodec.decode(encodedPayload).items)
  }
}

public enum NotificationPayloadCodec {
  public static func encode(_ request: FleetNotificationRequest) throws -> String {
    let payload = NotificationPayload(fingerprint: request.fingerprint, items: request.items)
    return String(decoding: try JSONEncoder().encode(payload), as: UTF8.self)
  }

  public static func decode(_ encoded: String) throws -> NotificationPayload {
    try JSONDecoder().decode(NotificationPayload.self, from: Data(encoded.utf8))
  }
}

public struct NotificationPayload: Codable, Equatable, Sendable {
  public let fingerprint: String
  public let items: [FleetNotificationItem]
}

public struct NotificationDeliveryState: Codable, Equatable, Sendable {
  public let fingerprint: String?
  public let submittedAt: Date?

  public init(fingerprint: String?, submittedAt: Date?) {
    self.fingerprint = fingerprint
    self.submittedAt = submittedAt
  }
}

public protocol NotificationStateStore: Sendable {
  func load() async throws -> NotificationDeliveryState?
  func save(_ state: NotificationDeliveryState) async throws
}

public enum NotificationState: Equatable, Sendable {
  case ready
  case permissionDenied
  case unavailable
  case permissionFailed(String)
  case persistenceFailed(String)
  case deliveryFailed(String)

  public var errorText: String? {
    switch self {
    case .ready: nil
    case .permissionDenied: "Notifications denied in System Settings"
    case .unavailable: "Notifications unavailable"
    case .permissionFailed(let detail): "Notification permission failed: \(detail)"
    case .persistenceFailed(let detail): "Notification state failed: \(detail)"
    case .deliveryFailed(let detail): "Notification delivery failed: \(detail)"
    }
  }
}

public actor NotificationCoordinator {
  private let client: any NotificationClient
  private let store: any NotificationStateStore
  private let now: @Sendable () -> Date
  private var submittedFingerprint: String?
  private var tail: Task<Void, Never>?

  public init(
    client: any NotificationClient,
    store: any NotificationStateStore,
    now: @escaping @Sendable () -> Date = Date.init
  ) {
    self.client = client
    self.store = store
    self.now = now
  }

  public func process(attention: FleetAttention) async -> NotificationState {
    let predecessor = tail
    let operation = Task { [weak self] in
      await predecessor?.value
      guard let self else { return NotificationState.unavailable }
      return await self.perform(attention: attention)
    }
    tail = Task { _ = await operation.value }
    return await operation.value
  }

  private func perform(attention: FleetAttention) async -> NotificationState {
    let authorization = await client.authorizationStatus()
    switch authorization {
    case .denied: return .permissionDenied
    case .unavailable: return .unavailable
    case .notDetermined:
      do {
        guard try await client.requestAuthorization() else { return .permissionDenied }
      } catch {
        return .permissionFailed(String(describing: error))
      }
    case .authorized: break
    }

    let previous: NotificationDeliveryState?
    do {
      previous = try await store.load()
    } catch {
      return .persistenceFailed(String(describing: error))
    }

    let eligible = attention.items.filter(\.eligible)
    guard !eligible.isEmpty, !attention.fingerprint.isEmpty else {
      submittedFingerprint = nil
      if previous?.fingerprint != nil {
        do {
          try await store.save(
            NotificationDeliveryState(fingerprint: nil, submittedAt: previous?.submittedAt))
        } catch {
          return .persistenceFailed(String(describing: error))
        }
      }
      return .ready
    }
    guard previous?.fingerprint != attention.fingerprint,
      submittedFingerprint != attention.fingerprint
    else { return .ready }

    let currentTime = now()
    if let submittedAt = previous?.submittedAt {
      let elapsed = currentTime.timeIntervalSince(submittedAt)
      if elapsed >= 0, elapsed < Double(attention.throttleMinutes * 60) { return .ready }
    }

    let items = eligible.map {
      FleetNotificationItem(
        machineID: $0.machineID,
        repoKey: $0.repoKey,
        name: $0.name,
        reason: $0.dominantReason.replacingOccurrences(of: "_", with: " "))
    }
    let lines = items.prefix(4).map { "\($0.name) (\($0.machineID)): \($0.reason)" }
    let overflow = items.count > 4 ? "\n+\(items.count - 4) more" : ""
    let request = FleetNotificationRequest(
      fingerprint: attention.fingerprint,
      title: "\(items.count) bb \(items.count == 1 ? "repository" : "repositories") need attention",
      body: lines.joined(separator: "\n") + overflow,
      items: items)
    do {
      try await client.submit(request)
    } catch {
      return .deliveryFailed(String(describing: error))
    }
    submittedFingerprint = attention.fingerprint
    do {
      try await store.save(
        NotificationDeliveryState(fingerprint: attention.fingerprint, submittedAt: currentTime))
    } catch {
      return .persistenceFailed(String(describing: error))
    }
    return .ready
  }
}
