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
      let task = Task.detached {
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
          async let errorData = stderr.fileHandleForReading.readToEnd() ?? Data()
          var buffer = Data()
          while true {
            try Task.checkCancellation()
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
          let detail = String(decoding: try await errorData, as: UTF8.self)
          guard process.terminationStatus == 0 else {
            throw BBClientError.commandFailed(code: process.terminationStatus, detail: detail)
          }
          continuation.finish()
        } catch { continuation.finish(throwing: error) }
      }
      continuation.onTermination = { _ in
        task.cancel()
        lifetime.cancel()
      }
    }
  }

  private func run(arguments: [String]) async throws -> Data {
    let executable = try executableURL ?? Self.resolveExecutable()
    return try await Task.detached {
      let process = Process()
      let stdout = Pipe()
      let stderr = Pipe()
      process.executableURL = executable
      process.arguments = arguments
      process.standardOutput = stdout
      process.standardError = stderr
      try process.run()
      async let output = stdout.fileHandleForReading.readToEnd() ?? Data()
      async let errorOutput = stderr.fileHandleForReading.readToEnd() ?? Data()
      process.waitUntilExit()
      let (statusData, errorData) = try await (output, errorOutput)
      guard process.terminationStatus == 0 else {
        let detail = String(decoding: errorData, as: UTF8.self)
        throw BBClientError.commandFailed(code: process.terminationStatus, detail: detail)
      }
      return statusData
    }.value
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
