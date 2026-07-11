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

    #expect(model.presentation.sections.map(\.title) == ["Blocked", "Stale WIP", "Other machines"])
    #expect(model.presentation.sections[0].items.map(\.title) == ["blocked", "recent"])
    #expect(model.presentation.sections[1].items.map(\.title) == ["stale"])
    #expect(model.presentation.sections[2].items.map(\.title) == ["remote-only", "remote"])
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
  func sync() async throws {}
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

  func sync() async throws {
    recorded.append("sync")
    if let syncError { throw syncError }
  }
  func calls() -> [String] { recorded }
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
