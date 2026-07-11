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
  public private(set) var activeRepository: String?
  public private(set) var operationStatus: String?
  public private(set) var repositoryFailures: [String: String] = [:]

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
    await sync(repository: nil)
  }

  public func sync(repository: String?) async {
    await runOperation(repository: repository, name: "Sync") {
      await self.client.sync(repository: repository)
    }
  }

  public func fix(repository: String, action: String) async {
    await runOperation(repository: repository, name: "Fix") {
      await self.client.fix(repository: repository, action: action)
    }
  }

  private func runOperation(
    repository: String?, name: String,
    makeStream: @escaping @Sendable () async -> AsyncThrowingStream<OperationEvent, Error>
  ) async {
    guard !isSyncing else { return }
    isSyncing = true
    activeRepository = repository
    defer {
      isSyncing = false
      activeRepository = nil
    }
    let syncError: String?
    var reportedFailure = false
    do {
      let stream = await makeStream()
      for try await event in stream {
        if let eventRepository = event.repository,
          event.event == "repository_started" || event.event == "progress"
        {
          activeRepository = eventRepository
        }
        if event.result == "failure" {
          reportedFailure = true
          let detail = event.error.flatMap { $0.isEmpty ? nil : $0 } ?? event.message
          if let eventRepository = event.repository {
            if event.event == "repository_finished" || repositoryFailures[eventRepository] == nil {
              repositoryFailures[eventRepository] = detail
            }
            operationStatus = repositoryFailures[eventRepository] ?? detail
          } else {
            operationStatus = detail
          }
          continue
        }
        if event.event == "operation_finished", reportedFailure { continue }
        operationStatus = event.message
      }
      syncError = nil
    } catch {
      syncError = String(describing: error)
      if let failedRepository = activeRepository ?? repository {
        if let specificFailure = repositoryFailures[failedRepository] {
          operationStatus = specificFailure
        } else {
          repositoryFailures[failedRepository] = syncError
          operationStatus = "\(name) failed: \(syncError!)"
        }
      } else {
        operationStatus = "\(name) failed: \(syncError!)"
      }
    }
    await refresh()
    if let syncError {
      basePresentation = MenuPresentation(
        sections: basePresentation.sections,
        lastSync: basePresentation.lastSync,
        errors: basePresentation.errors + ["\(name) failed: \(syncError)"])
      applyPlatformState()
    } else if !reportedFailure {
      operationStatus = "\(name) completed"
      if let repository { repositoryFailures[repository] = nil }
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
