import Foundation

public struct OverviewContract: Decodable, Sendable {
  public let machines: [OverviewMachine]
  public let repos: [OverviewRepo]
  public let syncedEverywhere: Int
  public let warnings: [String]

  enum CodingKeys: String, CodingKey {
    case machines, repos, warnings
    case syncedEverywhere = "synced_everywhere"
  }
}

public struct OverviewMachine: Decodable, Sendable {
  public let id: String
  public let here: Bool
  public let published: Bool
  public let updatedAt: Date?
  public let stale: Bool

  enum CodingKeys: String, CodingKey {
    case id, here, published, stale
    case updatedAt = "updated_at"
  }
}

public struct OverviewRepo: Decodable, Sendable {
  public let repoKey: String
  public let cells: [OverviewCell]
  public let syncedEverywhere: Bool

  enum CodingKeys: String, CodingKey {
    case repoKey = "repo_key"
    case cells
    case syncedEverywhere = "synced_everywhere"
  }
}

public struct OverviewCell: Decodable, Sendable {
  public let machineID: String
  public let present: Bool
  public let state: RepoState?
  public let reasons: [String]
  public let warnings: [String]
  public let lastActivityAt: Date

  enum CodingKeys: String, CodingKey {
    case machineID = "machine_id"
    case present, state, reasons, warnings
    case lastActivityAt = "last_activity_at"
  }
}
