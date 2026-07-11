import Foundation
import Testing

@testable import BBMenuBarCore

@Suite("fleet notification coordination")
struct NotificationCoordinatorTests {
  @Test("submits one eligible fleet digest containing local and remote attention")
  func fleetDigest() async {
    let client = RecordingNotificationClient()
    let store = MemoryNotificationStore()
    let coordinator = NotificationCoordinator(
      client: client, store: store, now: { Date(timeIntervalSince1970: 10_000) })

    let result = await coordinator.process(
      attention: attention(fingerprint: "one", throttleMinutes: 60))

    #expect(result == .ready)
    let requests = await client.requests
    #expect(requests.count == 1)
    #expect(requests[0].items.map(\.name) == ["blocked", "remote"])
    #expect(requests[0].fingerprint == "one")
  }

  @Test("ignores ineligible items, deduplicates, resets after empty, and throttles changed sets")
  func decisions() async {
    let client = RecordingNotificationClient()
    let store = MemoryNotificationStore()
    let clock = MutableClock(Date(timeIntervalSince1970: 10_000))
    let coordinator = NotificationCoordinator(client: client, store: store, now: { clock.now })

    _ = await coordinator.process(attention: attention(fingerprint: "one", throttleMinutes: 60))
    _ = await coordinator.process(attention: attention(fingerprint: "one", throttleMinutes: 60))
    _ = await coordinator.process(attention: attention(fingerprint: "two", throttleMinutes: 60))
    #expect(await client.requests.count == 1)

    clock.now = Date(timeIntervalSince1970: 14_000)
    _ = await coordinator.process(attention: attention(fingerprint: "two", throttleMinutes: 60))
    #expect(await client.requests.count == 2)

    _ = await coordinator.process(attention: emptyAttention())
    clock.now = Date(timeIntervalSince1970: 18_000)
    _ = await coordinator.process(attention: attention(fingerprint: "two", throttleMinutes: 60))
    #expect(await client.requests.count == 3)
  }

  @Test("permission and external failures are explicit and failed delivery is not persisted")
  func failures() async {
    let denied = RecordingNotificationClient(authorization: .denied)
    let deniedCoordinator = NotificationCoordinator(
      client: denied, store: MemoryNotificationStore())
    #expect(await deniedCoordinator.process(attention: attention()) == .permissionDenied)

    let failing = RecordingNotificationClient(submitError: TestError.failed)
    let store = MemoryNotificationStore()
    let coordinator = NotificationCoordinator(client: failing, store: store)
    #expect(await coordinator.process(attention: attention()) == .deliveryFailed("failed"))
    #expect(await store.saved.isEmpty)

    let unavailable = RecordingNotificationClient(authorization: .unavailable)
    #expect(
      await NotificationCoordinator(client: unavailable, store: MemoryNotificationStore()).process(
        attention: attention()) == .unavailable)
  }

  @Test("requests undetermined permission before submitting")
  func requestsPermission() async {
    let client = RecordingNotificationClient(authorization: .notDetermined)
    let coordinator = NotificationCoordinator(client: client, store: MemoryNotificationStore())

    #expect(await coordinator.process(attention: attention()) == .ready)
    #expect(await client.authorizationRequests == 1)
    #expect(await client.requests.count == 1)
  }

  @Test("declined and failed permission requests are explicit and do not submit")
  func permissionRequestFailures() async {
    let declined = RecordingNotificationClient(
      authorization: .notDetermined, authorizationResult: .success(false))
    let failed = RecordingNotificationClient(
      authorization: .notDetermined, authorizationResult: .failure(.failed))

    #expect(
      await NotificationCoordinator(client: declined, store: MemoryNotificationStore()).process(
        attention: attention()) == .permissionDenied)
    #expect(
      await NotificationCoordinator(client: failed, store: MemoryNotificationStore()).process(
        attention: attention()) == .permissionFailed("failed"))
    #expect(await declined.requests.isEmpty)
    #expect(await failed.requests.isEmpty)
  }

  @Test("recent blocked is excluded while eligible stale WIP submits")
  func eligibilityComesFromContract() async {
    let client = RecordingNotificationClient()
    let coordinator = NotificationCoordinator(client: client, store: MemoryNotificationStore())
    let attention = FleetAttention(
      items: [
        AttentionItem(
          machineID: "local", repoKey: "recent", name: "recent", state: .blocked,
          dominantReason: "push_failed", reasons: ["push_failed"], lastActivityAt: .now,
          eligible: false),
        AttentionItem(
          machineID: "local", repoKey: "stale", name: "stale", state: .wip,
          dominantReason: "dirty_tracked", reasons: ["dirty_tracked"],
          lastActivityAt: .distantPast, eligible: true),
      ], eligibleCount: 1, fingerprint: "stale-only", throttleMinutes: 60)

    #expect(await coordinator.process(attention: attention) == .ready)
    #expect(await client.requests.flatMap(\.items).map(\.name) == ["stale"])
  }

  @Test("store load and save failures render explicit persistence state")
  func persistenceFailures() async {
    let loadFailure = MemoryNotificationStore(loadError: .failed)
    let saveFailure = MemoryNotificationStore(saveError: .failed)

    let loadState = await NotificationCoordinator(
      client: RecordingNotificationClient(), store: loadFailure
    ).process(attention: attention())
    let saveState = await NotificationCoordinator(
      client: RecordingNotificationClient(), store: saveFailure
    ).process(attention: attention())

    #expect(loadState == .persistenceFailed("failed"))
    #expect(loadState.errorText == "Notification state failed: failed")
    #expect(saveState == .persistenceFailed("failed"))
    #expect(saveState.errorText == "Notification state failed: failed")
  }

  @Test("concurrent refreshes coalesce decision through persisted delivery")
  func concurrentDelivery() async {
    let client = SuspendedNotificationClient()
    let coordinator = NotificationCoordinator(client: client, store: MemoryNotificationStore())

    let first = Task { await coordinator.process(attention: attention()) }
    await client.waitUntilSubmitted()
    let second = Task { await coordinator.process(attention: attention()) }
    await Task.yield()
    #expect(await client.submitCount == 1)

    await client.resume()
    #expect(await first.value == .ready)
    #expect(await second.value == .ready)
    #expect(await client.submitCount == 1)
  }

  @Test("save failure retains process-lifetime dedupe")
  func saveFailureDedupe() async {
    let client = RecordingNotificationClient()
    let coordinator = NotificationCoordinator(
      client: client, store: MemoryNotificationStore(saveError: .failed))

    #expect(await coordinator.process(attention: attention()) == .persistenceFailed("failed"))
    #expect(await coordinator.process(attention: attention()) == .ready)
    #expect(await client.requests.count == 1)
  }

  @Test("clock rollback does not suppress a changed attention set")
  func clockRollback() async {
    let client = RecordingNotificationClient()
    let store = MemoryNotificationStore(
      state: NotificationDeliveryState(
        fingerprint: "old", submittedAt: Date(timeIntervalSince1970: 20_000)))
    let coordinator = NotificationCoordinator(
      client: client, store: store, now: { Date(timeIntervalSince1970: 10_000) })

    #expect(await coordinator.process(attention: attention(fingerprint: "new")) == .ready)
    #expect(await client.requests.count == 1)
  }

  @Test("production payload adapter decodes and routes exact digest items")
  @MainActor
  func payloadRouting() async throws {
    let router = RecordingNotificationRouter()
    let handler = NotificationResponseHandler(router: router)
    let request = FleetNotificationRequest(
      fingerprint: "digest", title: "title", body: "body",
      items: [
        FleetNotificationItem(
          machineID: "remote", repoKey: "software/api", name: "api", reason: "diverged")
      ])

    let payload = try NotificationPayloadCodec.encode(request)
    try await handler.route(encodedPayload: payload)

    #expect(router.shown == request.items)
  }

  @Test("notification response reveals the exact digest items")
  @MainActor
  func responseRouting() async {
    let router = RecordingNotificationRouter()
    let handler = NotificationResponseHandler(router: router)
    let items = [
      FleetNotificationItem(
        machineID: "remote", repoKey: "software/api", name: "api", reason: "diverged")
    ]

    await handler.route(items: items)

    #expect(router.shown == items)
  }

  private func attention(fingerprint: String = "one", throttleMinutes: Int = 60) -> FleetAttention {
    FleetAttention(
      items: [
        AttentionItem(
          machineID: "local", repoKey: "a", name: "blocked", state: .blocked,
          dominantReason: "diverged", reasons: ["diverged"], lastActivityAt: .distantPast,
          eligible: true),
        AttentionItem(
          machineID: "local", repoKey: "pending", name: "pending", state: .pending,
          dominantReason: "clone_required", reasons: ["clone_required"],
          lastActivityAt: .distantPast, eligible: false),
        AttentionItem(
          machineID: "remote", repoKey: "b", name: "remote", state: .wip, dominantReason: "dirty",
          reasons: ["dirty"], lastActivityAt: .distantPast, eligible: true),
      ], eligibleCount: 2, fingerprint: fingerprint, throttleMinutes: throttleMinutes)
  }

  private func emptyAttention() -> FleetAttention {
    FleetAttention(items: [], eligibleCount: 0, fingerprint: "", throttleMinutes: 60)
  }
}

private enum TestError: String, Error, CustomStringConvertible {
  case failed
  var description: String { rawValue }
}

private actor RecordingNotificationClient: NotificationClient {
  let authorization: NotificationAuthorization
  let submitError: TestError?
  let authorizationResult: Result<Bool, TestError>
  private(set) var requests: [FleetNotificationRequest] = []
  private(set) var authorizationRequests = 0
  init(
    authorization: NotificationAuthorization = .authorized,
    authorizationResult: Result<Bool, TestError> = .success(true),
    submitError: TestError? = nil
  ) {
    self.authorization = authorization
    self.authorizationResult = authorizationResult
    self.submitError = submitError
  }
  func authorizationStatus() async -> NotificationAuthorization { authorization }
  func requestAuthorization() async throws -> Bool {
    authorizationRequests += 1
    return try authorizationResult.get()
  }
  func submit(_ request: FleetNotificationRequest) async throws {
    if let submitError { throw submitError }
    requests.append(request)
  }
}

private actor MemoryNotificationStore: NotificationStateStore {
  private var state: NotificationDeliveryState?
  private let loadError: TestError?
  private let saveError: TestError?
  private(set) var saved: [NotificationDeliveryState] = []
  init(
    state: NotificationDeliveryState? = nil,
    loadError: TestError? = nil,
    saveError: TestError? = nil
  ) {
    self.state = state
    self.loadError = loadError
    self.saveError = saveError
  }
  func load() async throws -> NotificationDeliveryState? {
    if let loadError { throw loadError }
    return state
  }
  func save(_ value: NotificationDeliveryState) async throws {
    if let saveError { throw saveError }
    state = value
    saved.append(value)
  }
}

private actor SuspendedNotificationClient: NotificationClient {
  private(set) var submitCount = 0
  private var submissionStarted: CheckedContinuation<Void, Never>?
  private var submissionResume: CheckedContinuation<Void, Never>?

  func authorizationStatus() async -> NotificationAuthorization { .authorized }
  func requestAuthorization() async throws -> Bool { true }
  func submit(_: FleetNotificationRequest) async throws {
    submitCount += 1
    submissionStarted?.resume()
    submissionStarted = nil
    await withCheckedContinuation { submissionResume = $0 }
  }
  func waitUntilSubmitted() async {
    if submitCount > 0 { return }
    await withCheckedContinuation { submissionStarted = $0 }
  }
  func resume() {
    submissionResume?.resume()
    submissionResume = nil
  }
}

private final class MutableClock: @unchecked Sendable {
  var now: Date
  init(_ now: Date) { self.now = now }
}

@MainActor
private final class RecordingNotificationRouter: NotificationRouter {
  var shown: [FleetNotificationItem] = []
  func show(items: [FleetNotificationItem]) { shown = items }
}
