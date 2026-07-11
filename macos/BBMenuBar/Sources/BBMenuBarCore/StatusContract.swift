import Foundation

public struct StatusContract: Decodable, Sendable {
  public let machineID: String
  public let repos: [StatusRepo]
  public let summary: StatusSummary
  public let lastSync: StatusLastSync?
  public let attention: FleetAttention
  public let sourceWarnings: [String]

  enum CodingKeys: String, CodingKey {
    case machineID = "machine_id"
    case repos, summary
    case lastSync = "last_sync"
    case attention
    case sourceWarnings = "source_warnings"
  }
}

public struct StatusRepo: Decodable, Sendable {
  public let repoKey: String
  public let name: String
  public let catalog: String
  public let path: String
  public let state: RepoState
  public let reasons: [String]
  public let warnings: [String]
  public let lastActivityAt: Date

  enum CodingKeys: String, CodingKey {
    case repoKey = "repo_key"
    case name, catalog, path, state, reasons, warnings
    case lastActivityAt = "last_activity_at"
  }
}

public enum RepoState: String, Decodable, Sendable {
  case synced, pending, wip, blocked
}

public struct StatusSummary: Decodable, Sendable {
  public let total: Int
  public let synced: Int
  public let pending: Int
  public let wip: Int
  public let blocked: Int
  public let warnings: Int
}

public struct StatusLastSync: Decodable, Sendable {
  public let at: Date
  public let machine: String
  public let event: String
  public let detail: String
}

public struct FleetAttention: Decodable, Sendable {
  public let items: [AttentionItem]
  public let eligibleCount: Int
  public let fingerprint: String
  public let throttleMinutes: Int

  enum CodingKeys: String, CodingKey {
    case items
    case eligibleCount = "eligible_count"
    case fingerprint
    case throttleMinutes = "throttle_minutes"
  }
}

public struct AttentionItem: Decodable, Sendable {
  public let machineID: String
  public let repoKey: String
  public let name: String
  public let state: RepoState
  public let dominantReason: String
  public let reasons: [String]
  public let lastActivityAt: Date
  public let eligible: Bool

  enum CodingKeys: String, CodingKey {
    case machineID = "machine_id"
    case repoKey = "repo_key"
    case name, state
    case dominantReason = "dominant_reason"
    case reasons
    case lastActivityAt = "last_activity_at"
    case eligible
  }
}

extension JSONDecoder {
  public static var bb: JSONDecoder {
    let decoder = JSONDecoder()
    decoder.dateDecodingStrategy = .custom { decoder in
      let value = try decoder.singleValueContainer().decode(String.self)
      if let date = try? Date(value, strategy: .iso8601) {
        return date
      }
      if let date = try? Date(
        value,
        strategy: .iso8601.year().month().day().time(includingFractionalSeconds: true).timeZone(
          separator: .colon))
      {
        return date
      }
      throw DecodingError.dataCorruptedError(
        in: try decoder.singleValueContainer(),
        debugDescription: "Invalid RFC 3339 timestamp: \(value)")
    }
    return decoder
  }
}
