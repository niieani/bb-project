import Foundation

public protocol BBClient: Sendable {
  func statusJSON() async throws -> Data
  func overviewJSON() async throws -> Data
  func sync(repository: String?) async -> AsyncThrowingStream<OperationEvent, Error>
  func fix(repository: String, action: String) async -> AsyncThrowingStream<OperationEvent, Error>
}

extension BBClient {
  public func fix(repository: String, action: String) async -> AsyncThrowingStream<OperationEvent, Error> {
    AsyncThrowingStream { $0.finish(throwing: BBClientError.commandFailed(code: 2, detail: "fix is unavailable")) }
  }
}

public struct OperationEvent: Decodable, Equatable, Sendable {
  public let event: String
  public let operation: String
  public let repository: String?
  public let phase: String?
  public let message: String
  public let result: String?
  public let error: String?
  public let completed: Int?
  public let total: Int?

  public init(
    event: String, operation: String, repository: String?, phase: String?, message: String,
    result: String?, error: String?, completed: Int? = nil, total: Int? = nil
  ) {
    self.event = event
    self.operation = operation
    self.repository = repository
    self.phase = phase
    self.message = message
    self.result = result
    self.error = error
    self.completed = completed
    self.total = total
  }
}

public struct ProcessBBClient: BBClient {
  private let executableURL: URL?

  public init(executableURL: URL? = nil) {
    self.executableURL = executableURL
  }

  public func statusJSON() async throws -> Data {
    try await run(arguments: ["status", "--json"])
  }

  public func overviewJSON() async throws -> Data {
    try await run(arguments: ["overview", "--json"])
  }

  public func sync(repository: String?) async -> AsyncThrowingStream<OperationEvent, Error> {
    var arguments = ["sync", "--quiet", "--events-json"]
    if let repository { arguments += ["--repo", repository] }
    return eventStream(arguments: arguments)
  }

  public func fix(repository: String, action: String) async -> AsyncThrowingStream<OperationEvent, Error> {
    eventStream(arguments: ["fix", repository, action, "--events-json", "--no-refresh"])
  }

  private func eventStream(arguments: [String]) -> AsyncThrowingStream<OperationEvent, Error> {
    AsyncThrowingStream { continuation in
      let lifetime = ProcessLifetime()
      DispatchQueue.global(qos: .userInitiated).async {
        do {
          let executable = try executableURL ?? Self.resolveExecutable()
          let process = Process()
          let stdout = Pipe()
          let stderr = Pipe()
          process.executableURL = executable
          process.arguments = arguments
          process.standardOutput = stdout
          process.standardError = stderr
          try process.run()
          lifetime.register(process)
          defer { lifetime.clear() }
          let errorData = LockedData()
          let errorRead = DispatchGroup()
          errorRead.enter()
          DispatchQueue.global(qos: .userInitiated).async {
            errorData.set((try? stderr.fileHandleForReading.readToEnd()) ?? Data())
            errorRead.leave()
          }
          var buffer = Data()
          while true {
            let chunk = stdout.fileHandleForReading.availableData
            if chunk.isEmpty { break }
            buffer.append(chunk)
            while let newline = buffer.firstIndex(of: 0x0A) {
              let line = Data(buffer[..<newline])
              buffer.removeSubrange(...newline)
              if !line.isEmpty {
                continuation.yield(try JSONDecoder().decode(OperationEvent.self, from: line))
              }
            }
          }
          process.waitUntilExit()
          errorRead.wait()
          let detail = String(decoding: errorData.value(), as: UTF8.self)
          guard process.terminationStatus == 0 else {
            throw BBClientError.commandFailed(code: process.terminationStatus, detail: detail)
          }
          continuation.finish()
        } catch { continuation.finish(throwing: error) }
      }
      continuation.onTermination = { _ in
        lifetime.cancel()
      }
    }
  }

  private func run(arguments: [String]) async throws -> Data {
    let executable = try executableURL ?? Self.resolveExecutable()
    return try await withCheckedThrowingContinuation { continuation in
      DispatchQueue.global(qos: .userInitiated).async {
        do {
          let process = Process()
          let stdout = Pipe()
          let stderr = Pipe()
          process.executableURL = executable
          process.arguments = arguments
          process.standardOutput = stdout
          process.standardError = stderr
          try process.run()
          let output = LockedData()
          let errorOutput = LockedData()
          let reads = DispatchGroup()
          for (handle, destination) in [
            (stdout.fileHandleForReading, output),
            (stderr.fileHandleForReading, errorOutput),
          ] {
            reads.enter()
            DispatchQueue.global(qos: .userInitiated).async {
              destination.set((try? handle.readToEnd()) ?? Data())
              reads.leave()
            }
          }
          process.waitUntilExit()
          reads.wait()
          guard process.terminationStatus == 0 else {
            throw BBClientError.commandFailed(
              code: process.terminationStatus,
              detail: String(decoding: errorOutput.value(), as: UTF8.self))
          }
          continuation.resume(returning: output.value())
        } catch {
          continuation.resume(throwing: error)
        }
      }
    }
  }

  private static func resolveExecutable() throws -> URL {
    let environment = ProcessInfo.processInfo.environment
    var candidates = environment["PATH", default: ""]
      .split(separator: ":")
      .map { URL(fileURLWithPath: String($0)).appending(path: "bb") }
    candidates += [
      URL(fileURLWithPath: "/opt/homebrew/bin/bb"),
      URL(fileURLWithPath: "/usr/local/bin/bb"),
      FileManager.default.homeDirectoryForCurrentUser.appending(path: "bin/bb"),
    ]
    guard
      let executable = candidates.first(where: {
        FileManager.default.isExecutableFile(atPath: $0.path)
      })
    else {
      throw BBClientError.binaryNotFound
    }
    return executable
  }
}

private final class LockedData: @unchecked Sendable {
  private let lock = NSLock()
  private var data = Data()

  func set(_ data: Data) { lock.withLock { self.data = data } }

  func value() -> Data { lock.withLock { data } }
}

private final class ProcessLifetime: @unchecked Sendable {
  private let lock = NSLock()
  private var process: Process?
  private var canceled = false

  func register(_ process: Process) {
    let shouldTerminate = lock.withLock {
      self.process = process
      return canceled
    }
    if shouldTerminate, process.isRunning { process.terminate() }
  }

  func clear() {
    lock.withLock { process = nil }
  }

  func cancel() {
    lock.withLock {
      canceled = true
      guard let process, process.isRunning else { return }
      process.terminate()
    }
  }
}

public enum BBClientError: Error, Equatable {
  case binaryNotFound
  case commandFailed(code: Int32, detail: String)
}
