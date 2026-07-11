import Foundation
import Testing

@testable import BBMenuBarCore

@Suite("bb process client")
struct ProcessBBClientTests {
  @Test("event stream drains large stderr concurrently")
  func drainsLargeStderr() async throws {
    let executable = try makeExecutable(
      """
      dd if=/dev/zero bs=65536 count=32 >&2 2>/dev/null
      printf '%s\n' '{"event":"operation_finished","operation":"sync","message":"done","result":"success"}'
      """)
    defer { try? FileManager.default.removeItem(at: executable.deletingLastPathComponent()) }
    var received: [OperationEvent] = []
    for try await event in await ProcessBBClient(executableURL: executable).sync(repository: nil) {
      received.append(event)
    }
    #expect(received.map(\.event) == ["operation_finished"])
  }

  @Test("canceling event consumption terminates the child process")
  func cancellationTerminatesChild() async throws {
    let directory = FileManager.default.temporaryDirectory.appending(
      path: "bb-cancel-\(UUID().uuidString)")
    try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
    defer { try? FileManager.default.removeItem(at: directory) }
    let started = directory.appending(path: "started")
    let stopped = directory.appending(path: "stopped")
    let executable = try makeExecutable(
      """
      trap 'touch "\(stopped.path)"; exit 0' TERM
      touch "\(started.path)"
      while true; do sleep 0.05; done
      """, directory: directory)
    let stream = await ProcessBBClient(executableURL: executable).sync(repository: nil)
    let consumer = Task {
      for try await _ in stream {}
    }
    await eventuallyFile(started)
    consumer.cancel()
    _ = await consumer.result
    await eventuallyFile(stopped)
    #expect(FileManager.default.fileExists(atPath: stopped.path))
  }

  @Test("targeted sync delivers JSON events before process completion")
  func incrementalTargetedSync() async throws {
    let directory = FileManager.default.temporaryDirectory.appending(
      path: "bb-stream-\(UUID().uuidString)")
    try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
    defer { try? FileManager.default.removeItem(at: directory) }
    let release = directory.appending(path: "release")
    let executable = directory.appending(path: "bb-stub")
    let script = """
      #!/bin/sh
      printf '%s\n' '{"event":"progress","operation":"sync","repository":"software/api","phase":"fetch","message":"Fetching origin"}'
      while [ ! -f '\(release.path)' ]; do sleep 0.01; done
      """
    try script.write(to: executable, atomically: true, encoding: .utf8)
    try FileManager.default.setAttributes([.posixPermissions: 0o755], ofItemAtPath: executable.path)
    let stream = await ProcessBBClient(executableURL: executable).sync(repository: "software/api")
    let first = Task {
      var iterator = stream.makeAsyncIterator()
      return try await iterator.next()
    }
    let event = try await first.value
    #expect(event?.repository == "software/api")
    #expect(event?.phase == "fetch")
    #expect(!FileManager.default.fileExists(atPath: release.path))
    try Data().write(to: release)
  }

  @Test("sync runs exact quiet command off the main actor and surfaces nonzero exit")
  @MainActor
  func syncInvocation() async throws {
    let directory = FileManager.default.temporaryDirectory.appending(
      path: "bb-client-\(UUID().uuidString)")
    try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
    defer { try? FileManager.default.removeItem(at: directory) }
    let log = directory.appending(path: "arguments")
    let release = directory.appending(path: "release")
    let executable = directory.appending(path: "bb-stub")
    let script = """
      #!/bin/sh
      printf '%s\\n' "$@" > '\(log.path.replacingOccurrences(of: "'", with: "'\\''"))'
      while [ ! -f '\(release.path.replacingOccurrences(of: "'", with: "'\\''"))' ]; do
        sleep 0.01
      done
      exit 7
      """
    try script.write(to: executable, atomically: true, encoding: .utf8)
    try FileManager.default.setAttributes([.posixPermissions: 0o755], ofItemAtPath: executable.path)
    let client = ProcessBBClient(executableURL: executable)
    let syncTask = Task { () -> BBClientError? in
      do {
        for try await _ in await client.sync(repository: nil) {}
        return nil
      } catch let error as BBClientError {
        return error
      } catch {
        Issue.record("unexpected error: \(error)")
        return nil
      }
    }
    for _ in 0..<1_000 where !FileManager.default.fileExists(atPath: log.path) {
      try await Task.sleep(for: .milliseconds(1))
    }
    #expect(FileManager.default.fileExists(atPath: log.path))
    try Data().write(to: release)
    let error = await syncTask.value

    #expect(error == .commandFailed(code: 7, detail: ""))
    #expect(try String(contentsOf: log, encoding: .utf8) == "sync\n--quiet\n--events-json\n")
  }

  @Test("fix uses stable repository and action identifiers")
  func fixInvocation() async throws {
    let directory = FileManager.default.temporaryDirectory.appending(path: "bb-fix-\(UUID().uuidString)")
    try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
    defer { try? FileManager.default.removeItem(at: directory) }
    let log = directory.appending(path: "arguments")
    let executable = try makeExecutable("printf '%s\\n' \"$@\" > '\(log.path)'", directory: directory)
    for try await _ in await ProcessBBClient(executableURL: executable).fix(repository: "software/api", action: "align-remote-format") {}
    #expect(try String(contentsOf: log, encoding: .utf8) == "fix\nsoftware/api\nalign-remote-format\n--events-json\n--no-refresh\n")
  }
}

private func makeExecutable(_ body: String, directory: URL? = nil) throws -> URL {
  let directory =
    directory
    ?? FileManager.default.temporaryDirectory.appending(path: "bb-process-\(UUID().uuidString)")
  try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
  let executable = directory.appending(path: "bb-stub")
  try ("#!/bin/sh\n" + body + "\n").write(to: executable, atomically: true, encoding: .utf8)
  try FileManager.default.setAttributes([.posixPermissions: 0o755], ofItemAtPath: executable.path)
  return executable
}

private func eventuallyFile(_ url: URL) async {
  for _ in 0..<500 {
    if FileManager.default.fileExists(atPath: url.path) { return }
    try? await Task.sleep(for: .milliseconds(2))
  }
}
