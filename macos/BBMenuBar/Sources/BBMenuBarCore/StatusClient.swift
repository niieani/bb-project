import Foundation

public protocol StatusClient: Sendable {
  func statusJSON() async throws -> Data
}

public struct ProcessStatusClient: StatusClient {
  private let executableURL: URL?

  public init(executableURL: URL? = nil) {
    self.executableURL = executableURL
  }

  public func statusJSON() async throws -> Data {
    let executable = try executableURL ?? Self.resolveExecutable()
    return try await Task.detached {
      let process = Process()
      let stdout = Pipe()
      let stderr = Pipe()
      process.executableURL = executable
      process.arguments = ["status", "--json"]
      process.standardOutput = stdout
      process.standardError = stderr
      try process.run()
      async let output = stdout.fileHandleForReading.readToEnd() ?? Data()
      async let errorOutput = stderr.fileHandleForReading.readToEnd() ?? Data()
      process.waitUntilExit()
      let (statusData, errorData) = try await (output, errorOutput)
      guard process.terminationStatus == 0 else {
        let detail = String(decoding: errorData, as: UTF8.self)
        throw StatusClientError.commandFailed(code: process.terminationStatus, detail: detail)
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
      throw StatusClientError.binaryNotFound
    }
    return executable
  }
}

public enum StatusClientError: Error, Equatable {
  case binaryNotFound
  case commandFailed(code: Int32, detail: String)
}
