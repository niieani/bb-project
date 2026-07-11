import Foundation
import Testing

@testable import BBMenuBarCore

@Suite("bb process client")
struct ProcessBBClientTests {
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
        try await client.sync()
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
    #expect(try String(contentsOf: log, encoding: .utf8) == "sync\n--quiet\n")
  }
}
