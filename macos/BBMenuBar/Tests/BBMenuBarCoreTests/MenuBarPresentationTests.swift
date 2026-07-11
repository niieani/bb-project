import Foundation
import Testing

@testable import BBMenuBarCore

@Suite("menu presentation")
struct MenuBarPresentationTests {
  @Test("renders populated local and remote sections with stale sync")
  @MainActor
  func populatedSections() async {
    let client = MenuStubClient(
      status: .success(try! fixtureData(area: "status", name: "mixed")),
      overview: .success(try! fixtureData(area: "overview", name: "mixed")))
    let model = MenuBarModel(
      client: client,
      now: {
        try! Date("2026-07-10T12:00:00Z", strategy: .iso8601)
      })

    await model.refresh()

    #expect(
      model.presentation.sections.map(\.title) == [
        "Blocked", "Ready to sync", "Stale WIP", "Other machines",
      ])
    #expect(model.presentation.sections[0].items.map(\.title) == ["blocked", "recent"])
    #expect(model.presentation.sections[1].items.map(\.title) == ["synced"])
    #expect(model.presentation.sections[1].items[0].actions[0].label == "Sync")
    #expect(model.presentation.sections[2].items.map(\.title) == ["stale"])
    #expect(model.presentation.sections[3].items.map(\.title) == ["remote-only", "remote"])
    #expect(model.presentation.lastSync == "Last sync 30m ago")
  }

  @Test("caps visible repository titles at thirty characters")
  func titleLength() {
    let item = MenuItem(
      attention: AttentionItem(
        machineID: "machine-a",
        repoKey: "software/a-very-long-repository-name-that-keeps-going",
        name: "a-very-long-repository-name-that-keeps-going",
        state: .blocked,
        dominantReason: "diverged",
        reasons: ["diverged"],
        lastActivityAt: .distantPast,
        eligible: true))
    #expect(item.title == "a-very-long-repository-name...")
    #expect(item.title.count == 30)
  }

  @Test("repository status tones match the CLI tier palette")
  func statusTones() {
    #expect(menuItem(state: .synced).statusTone == .synced)
    #expect(menuItem(state: .pending).statusTone == .pending)
    #expect(menuItem(state: .wip).statusTone == .wip)
    #expect(menuItem(state: .blocked).statusTone == .blocked)
    #expect(RepoStatusTone.synced.lightHex == "#1A7F37")
    #expect(RepoStatusTone.synced.darkHex == "#3FB950")
    #expect(RepoStatusTone.pending.lightHex == "#9A6700")
    #expect(RepoStatusTone.pending.darkHex == "#D29922")
    #expect(RepoStatusTone.wip.lightHex == "#FFAF00")
    #expect(RepoStatusTone.wip.darkHex == "#FFAF00")
    #expect(RepoStatusTone.blocked.lightHex == "#CF222E")
    #expect(RepoStatusTone.blocked.darkHex == "#F85149")
  }

  @Test("omits empty attention sections")
  @MainActor
  func emptySections() async {
    let client = MenuStubClient(
      status: .success(try! fixtureData(area: "status", name: "all-synced")),
      overview: .success(
        Data(#"{"machines":[],"repos":[],"synced_everywhere":0,"warnings":[]}"#.utf8)))
    let model = MenuBarModel(client: client)

    await model.refresh()

    #expect(model.presentation.sections.isEmpty)
    #expect(model.presentation.lastSync == "No successful sync yet")
  }

  @Test("shows per-source errors without hiding valid status")
  @MainActor
  func overviewError() async {
    let client = MenuStubClient(
      status: .success(try! fixtureData(area: "status", name: "all-synced")),
      overview: .failure(MenuTestFailure.overview))
    let model = MenuBarModel(client: client)

    await model.refresh()

    #expect(model.title == .healthy(repoCount: 1))
    #expect(model.presentation.errors == ["Overview unavailable: overview"])
  }

  @Test("status error is explicit alongside a valid overview")
  @MainActor
  func statusError() async {
    let client = MenuStubClient(
      status: .failure(MenuTestFailure.status),
      overview: .success(try! fixtureData(area: "overview", name: "mixed")))
    let model = MenuBarModel(client: client)

    await model.refresh()

    #expect(model.title == .error)
    #expect(model.presentation.errors == ["Status unavailable: status"])
  }

  @Test("sync now invokes sync then refreshes both sources")
  @MainActor
  func syncNow() async {
    let client = RecordingMenuClient(
      status: try! fixtureData(area: "status", name: "all-synced"),
      overview: Data(#"{"machines":[],"repos":[],"synced_everywhere":0,"warnings":[]}"#.utf8))
    let model = MenuBarModel(client: client)

    await model.syncNow()

    let calls = await client.calls()
    #expect(calls.first == "sync")
    #expect(calls.dropFirst().sorted() == ["overview", "status"])
  }

  @Test("failed sync is visible and still refreshes both sources")
  @MainActor
  func failedSyncRefreshes() async {
    let client = RecordingMenuClient(
      status: try! fixtureData(area: "status", name: "all-synced"),
      overview: Data(#"{"machines":[],"repos":[],"synced_everywhere":0,"warnings":[]}"#.utf8),
      syncError: MenuTestFailure.sync)
    let model = MenuBarModel(client: client)

    await model.syncNow()

    let calls = await client.calls()
    #expect(calls.first == "sync")
    #expect(calls.dropFirst().sorted() == ["overview", "status"])
    #expect(model.presentation.errors.contains("Sync failed: sync"))
  }

  @Test("repository failure is attributed, competing sync is rejected, and completion refreshes")
  @MainActor
  func repositoryFailureAndSerialization() async {
    let client = EventMenuClient(
      status: try! fixtureData(area: "status", name: "mixed"),
      overview: try! fixtureData(area: "overview", name: "mixed"))
    let model = MenuBarModel(client: client)
    let operation = Task { await model.sync(repository: "software/synced") }
    await eventually { await client.syncCalls() == 1 }
    #expect(model.activeRepository == "software/synced")
    await model.sync(repository: "software/other")
    #expect(await client.syncCalls() == 1)
    await client.send(
      OperationEvent(
        event: "repository_finished", operation: "sync", repository: "software/synced",
        phase: "complete", message: "Repository needs attention", result: "failure",
        error: "pull failed"))
    await client.send(
      OperationEvent(
        event: "operation_finished", operation: "sync", repository: "software/synced",
        phase: nil, message: "Sync failed", result: "failure", error: nil))
    await client.fail(MenuTestFailure.sync)
    await operation.value
    #expect(model.repositoryFailures["software/synced"] == "pull failed")
    #expect(model.operationStatus == "pull failed")
    #expect(model.activeRepository == nil)
    #expect(await client.refreshCalls() == 2)
  }

  @Test("interval and wake events each refresh")
  @MainActor
  func refreshEvents() async {
    let client = RecordingMenuClient(
      status: try! fixtureData(area: "status", name: "all-synced"),
      overview: Data(#"{"machines":[],"repos":[],"synced_everywhere":0,"warnings":[]}"#.utf8))
    let events = ManualRefreshEvents()
    let model = MenuBarModel(client: client)
    model.start(events: events)

    events.send(.interval)
    events.send(.wake)
    await eventually { await client.calls().count == 4 }

    let calls = await client.calls()
    #expect(calls.filter { $0 == "status" }.count == 2)
    #expect(calls.filter { $0 == "overview" }.count == 2)
  }

  @Test("notification failures render while valid repository state remains visible")
  @MainActor
  func notificationFailure() async {
    let client = MenuStubClient(
      status: .success(try! fixtureData(area: "status", name: "mixed")),
      overview: .success(try! fixtureData(area: "overview", name: "mixed")))
    let notifications = NotificationCoordinator(
      client: FailingMenuNotificationClient(), store: MenuNotificationStore())
    let model = MenuBarModel(client: client, notifications: notifications)

    await model.refresh()

    #expect(model.title == .attention(count: 4))
    #expect(model.presentation.errors.contains("Notification delivery failed: overview"))
  }

  @Test("launch-at-login approval state renders explicitly")
  @MainActor
  func loginApproval() async {
    let model = MenuBarModel(
      client: MenuStubClient(
        status: .success(try! fixtureData(area: "status", name: "all-synced")),
        overview: .success(
          Data(#"{"machines":[],"repos":[],"synced_everywhere":0,"warnings":[]}"#.utf8))))
    await model.refresh()

    await model.configureLaunchAtLogin(
      LaunchAtLoginCoordinator(service: ApprovalLoginService()))

    #expect(
      model.presentation.errors.contains(
        "Launch at login requires approval in System Settings"))
  }
}

private func menuItem(state: RepoState) -> MenuItem {
  MenuItem(
    attention: AttentionItem(
      machineID: "machine-a", repoKey: "software/repo", name: "repo", state: state,
      dominantReason: "reason", reasons: ["reason"], lastActivityAt: .distantPast,
      eligible: true))
}

private enum MenuTestFailure: String, Error, CustomStringConvertible {
  case status, overview, sync
  var description: String { rawValue }
}

private struct MenuStubClient: BBClient {
  let status: Result<Data, MenuTestFailure>
  let overview: Result<Data, MenuTestFailure>

  func statusJSON() async throws -> Data { try status.get() }
  func overviewJSON() async throws -> Data { try overview.get() }
  func sync(repository: String?) async -> AsyncThrowingStream<OperationEvent, Error> {
    AsyncThrowingStream { $0.finish() }
  }
}

private actor RecordingMenuClient: BBClient {
  let status: Data
  let overview: Data
  let syncError: MenuTestFailure?
  private var recorded: [String] = []

  init(status: Data, overview: Data, syncError: MenuTestFailure? = nil) {
    self.status = status
    self.overview = overview
    self.syncError = syncError
  }

  func statusJSON() async throws -> Data {
    recorded.append("status")
    return status
  }

  func overviewJSON() async throws -> Data {
    recorded.append("overview")
    return overview
  }

  func sync(repository: String?) async -> AsyncThrowingStream<OperationEvent, Error> {
    recorded.append(repository.map { "sync:\($0)" } ?? "sync")
    return AsyncThrowingStream { continuation in
      if let syncError { continuation.finish(throwing: syncError) } else { continuation.finish() }
    }
  }
  func calls() -> [String] { recorded }
}

private actor EventMenuClient: BBClient {
  let status: Data
  let overview: Data
  private var syncCount = 0
  private var refreshCount = 0
  private let stream: AsyncThrowingStream<OperationEvent, Error>
  private let continuation: AsyncThrowingStream<OperationEvent, Error>.Continuation

  init(status: Data, overview: Data) {
    self.status = status
    self.overview = overview
    (stream, continuation) = AsyncThrowingStream.makeStream()
  }
  func statusJSON() async throws -> Data {
    refreshCount += 1
    return status
  }
  func overviewJSON() async throws -> Data {
    refreshCount += 1
    return overview
  }
  func sync(repository: String?) async -> AsyncThrowingStream<OperationEvent, Error> {
    syncCount += 1
    return stream
  }
  func syncCalls() -> Int { syncCount }
  func refreshCalls() -> Int { refreshCount }
  func send(_ event: OperationEvent) { continuation.yield(event) }
  func finish() { continuation.finish() }
  func fail(_ error: MenuTestFailure) { continuation.finish(throwing: error) }
}

@MainActor
private final class ManualRefreshEvents: RefreshEventSource, @unchecked Sendable {
  private let stream: AsyncStream<RefreshEvent>
  private let continuation: AsyncStream<RefreshEvent>.Continuation

  init() {
    (stream, continuation) = AsyncStream.makeStream()
  }

  func events() -> AsyncStream<RefreshEvent> { stream }
  func send(_ event: RefreshEvent) { continuation.yield(event) }
}

private func fixtureData(area: String, name: String) throws -> Data {
  var repository = URL(fileURLWithPath: #filePath)
  for _ in 0..<5 { repository.deleteLastPathComponent() }
  return try Data(contentsOf: repository.appending(path: "fixtures/\(area)/\(name).json"))
}

private func eventually(_ condition: @escaping @Sendable () async -> Bool) async {
  for _ in 0..<100 {
    if await condition() { return }
    try? await Task.sleep(for: .milliseconds(5))
  }
}

private actor FailingMenuNotificationClient: NotificationClient {
  func authorizationStatus() async -> NotificationAuthorization { .authorized }
  func requestAuthorization() async throws -> Bool { true }
  func submit(_: FleetNotificationRequest) async throws { throw MenuTestFailure.overview }
}

private actor MenuNotificationStore: NotificationStateStore {
  func load() async throws -> NotificationDeliveryState? { nil }
  func save(_: NotificationDeliveryState) async throws {}
}

private struct ApprovalLoginService: LaunchAtLoginService {
  func ensureEnabled() async -> LaunchAtLoginState { .requiresApproval }
}
