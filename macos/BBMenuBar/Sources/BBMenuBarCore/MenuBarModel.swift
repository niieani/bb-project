import Foundation
import Observation

public enum RefreshEvent: Sendable {
  case interval
  case wake
}

@MainActor
public protocol RefreshEventSource: Sendable {
  func events() -> AsyncStream<RefreshEvent>
}

@MainActor
@Observable
public final class MenuBarModel {
  public private(set) var title: MenuBarTitleState = .loading
  public private(set) var presentation = MenuPresentation(
    sections: [], lastSync: "No successful sync yet", errors: [])
  public private(set) var isSyncing = false

  private let client: any BBClient
  private let now: @Sendable () -> Date
  private var eventTask: Task<Void, Never>?

  public init(client: any BBClient, now: @escaping @Sendable () -> Date = Date.init) {
    self.client = client
    self.now = now
  }

  public func refresh() async {
    async let statusLoad = Self.loadStatus(client)
    async let overviewLoad = Self.loadOverview(client)
    let (status, overview) = await (statusLoad, overviewLoad)

    let statusValue: StatusContract?
    let statusError: String?
    switch status {
    case .success(let value):
      statusValue = value
      statusError = nil
      title =
        value.attention.eligibleCount > 0
        ? .attention(count: value.attention.eligibleCount)
        : .healthy(repoCount: value.summary.total)
    case .failure(let error):
      statusValue = nil
      statusError = error
      title = .error
    }

    let overviewValue: OverviewContract?
    let overviewError: String?
    switch overview {
    case .success(let value):
      overviewValue = value
      overviewError = nil
    case .failure(let error):
      overviewValue = nil
      overviewError = error
    }

    presentation = .make(
      status: statusValue,
      overview: overviewValue,
      statusError: statusError,
      overviewError: overviewError,
      now: now())
  }

  public func syncNow() async {
    isSyncing = true
    defer { isSyncing = false }
    let syncError: String?
    do {
      try await client.sync()
      syncError = nil
    } catch {
      syncError = String(describing: error)
    }
    await refresh()
    if let syncError {
      presentation = MenuPresentation(
        sections: presentation.sections,
        lastSync: presentation.lastSync,
        errors: presentation.errors + ["Sync failed: \(syncError)"])
    }
  }

  public func start(events: any RefreshEventSource) {
    eventTask?.cancel()
    eventTask = Task { [weak self] in
      for await _ in events.events() {
        guard !Task.isCancelled, let self else { return }
        await self.refresh()
      }
    }
  }

  isolated deinit {
    eventTask?.cancel()
  }

  private nonisolated static func loadStatus(_ client: any BBClient) async -> LoadResult<
    StatusContract
  > {
    do {
      return .success(
        try JSONDecoder.bb.decode(StatusContract.self, from: await client.statusJSON()))
    } catch {
      return .failure(String(describing: error))
    }
  }

  private nonisolated static func loadOverview(_ client: any BBClient) async -> LoadResult<
    OverviewContract
  > {
    do {
      return .success(
        try JSONDecoder.bb.decode(OverviewContract.self, from: await client.overviewJSON()))
    } catch {
      return .failure(String(describing: error))
    }
  }
}

private enum LoadResult<Value: Sendable>: Sendable {
  case success(Value)
  case failure(String)
}
