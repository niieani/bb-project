import AppKit
import Foundation

@MainActor
public final class SystemRefreshEventSource: RefreshEventSource {
  private let interval: Duration
  private let wakeEvents: @MainActor @Sendable () -> AsyncStream<Void>
  private let sleep: @Sendable (Duration) async throws -> Void

  public convenience init(interval: Duration) {
    self.init(
      interval: interval,
      wakeEvents: {
        AsyncStream { continuation in
          let task = Task {
            let notifications = NSWorkspace.shared.notificationCenter.notifications(
              named: NSWorkspace.didWakeNotification)
            for await _ in notifications {
              guard !Task.isCancelled else { return }
              continuation.yield(())
            }
          }
          continuation.onTermination = { _ in task.cancel() }
        }
      },
      sleep: { try await Task.sleep(for: $0) })
  }

  init(
    interval: Duration,
    wakeEvents: @escaping @MainActor @Sendable () -> AsyncStream<Void>,
    sleep: @escaping @Sendable (Duration) async throws -> Void
  ) {
    self.interval = interval
    self.wakeEvents = wakeEvents
    self.sleep = sleep
  }

  public func events() -> AsyncStream<RefreshEvent> {
    let wakeEvents = wakeEvents()
    return AsyncStream { continuation in
      let timerTask = Task {
        while !Task.isCancelled {
          do {
            try await sleep(interval)
          } catch {
            return
          }
          continuation.yield(.interval)
        }
      }
      let wakeTask = Task {
        for await _ in wakeEvents {
          guard !Task.isCancelled else { return }
          continuation.yield(.wake)
        }
      }
      continuation.onTermination = { _ in
        timerTask.cancel()
        wakeTask.cancel()
      }
    }
  }
}
