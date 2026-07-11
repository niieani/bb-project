import Foundation
import Testing

@testable import BBMenuBarCore

@Suite("system refresh events")
struct SystemRefreshEventSourceTests {
  @Test("maps timer and wake streams")
  @MainActor
  func timerAndWake() async {
    let sleeper = ManualSleeper()
    let wake = ManualWakeEvents()
    let source = SystemRefreshEventSource(
      interval: .seconds(300),
      wakeEvents: { wake.stream },
      sleep: { duration in try await sleeper.sleep(duration) })

    let collection = Task { @MainActor in
      var received: [RefreshEvent] = []
      for await event in source.events() {
        received.append(event)
        if received.count == 2 { break }
      }
      return received
    }
    await sleeper.resume()
    wake.send()
    let received = await collection.value

    #expect(Set(received.map(String.init(describing:))) == Set(["interval", "wake"]))
    let durations = await sleeper.requestedDurations()
    #expect(!durations.isEmpty)
    #expect(durations.allSatisfy { $0 == .seconds(300) })
  }
}

private actor ManualSleeper {
  private var continuations: [CheckedContinuation<Void, Error>] = []
  private var durations: [Duration] = []

  func sleep(_ duration: Duration) async throws {
    durations.append(duration)
    if durations.count > 1 { throw CancellationError() }
    try await withCheckedThrowingContinuation { continuations.append($0) }
  }

  func resume() async {
    while continuations.isEmpty { await Task.yield() }
    continuations.removeFirst().resume()
  }

  func requestedDurations() -> [Duration] { durations }
}

private final class ManualWakeEvents: @unchecked Sendable {
  let stream: AsyncStream<Void>
  private let continuation: AsyncStream<Void>.Continuation

  init() {
    (stream, continuation) = AsyncStream.makeStream()
  }

  func send() { continuation.yield(()) }
}
