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
  private let notifications: NotificationCoordinator?
  private let now: @Sendable () -> Date
  private var notificationState: NotificationState = .ready
  private var launchAtLoginState: LaunchAtLoginState = .enabled
  private var basePresentation = MenuPresentation(
    sections: [], lastSync: "No successful sync yet", errors: [])
  private var eventTask: Task<Void, Never>?

  public init(
    client: any BBClient,
    notifications: NotificationCoordinator? = nil,
    now: @escaping @Sendable () -> Date = Date.init
  ) {
    self.client = client
    self.notifications = notifications
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

    if let statusValue, let notifications {
      notificationState = await notifications.process(attention: statusValue.attention)
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

    basePresentation = .make(
      status: statusValue,
      overview: overviewValue,
      statusError: statusError,
      overviewError: overviewError,
      now: now())
    applyPlatformState()
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
      basePresentation = MenuPresentation(
        sections: basePresentation.sections,
        lastSync: basePresentation.lastSync,
        errors: basePresentation.errors + ["Sync failed: \(syncError)"])
      applyPlatformState()
    }
  }

  public func configureLaunchAtLogin(_ coordinator: LaunchAtLoginCoordinator) async {
    launchAtLoginState = await coordinator.ensureEnabled()
    applyPlatformState()
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

  private func applyPlatformState() {
    let platformErrors = [notificationState.errorText, launchAtLoginState.errorText].compactMap {
      $0
    }
    presentation = MenuPresentation(
      sections: basePresentation.sections,
      lastSync: basePresentation.lastSync,
      errors: basePresentation.errors + platformErrors)
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
