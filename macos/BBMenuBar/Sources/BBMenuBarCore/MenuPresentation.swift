import Foundation

public struct MenuPresentation: Equatable, Sendable {
  public let sections: [MenuSection]
  public let lastSync: String
  public let errors: [String]

  static func make(
    status: StatusContract?,
    overview: OverviewContract?,
    statusError: String?,
    overviewError: String?,
    now: Date
  ) -> Self {
    var sections: [MenuSection] = []
    if let status {
      let blocked = status.attention.items
        .filter { $0.machineID == status.machineID && $0.state == .blocked }
        .map(MenuItem.init)
      let staleWIP = status.attention.items
        .filter { $0.machineID == status.machineID && $0.state == .wip && $0.eligible }
        .map(MenuItem.init)
      let overviewIdentities = Set(
        overview?.repos.flatMap { repo in
          repo.cells.filter(\.present).map { "\($0.machineID)\u{0}\(repo.repoKey)" }
        } ?? [])
      let remote = status.attention.items
        .filter {
          $0.machineID != status.machineID && $0.eligible
            && overviewIdentities.contains("\($0.machineID)\u{0}\($0.repoKey)")
        }
        .map(MenuItem.init)
      if !blocked.isEmpty { sections.append(MenuSection(title: "Blocked", items: blocked)) }
      if !staleWIP.isEmpty { sections.append(MenuSection(title: "Stale WIP", items: staleWIP)) }
      if !remote.isEmpty { sections.append(MenuSection(title: "Other machines", items: remote)) }
    }

    var errors: [String] = []
    if let statusError { errors.append("Status unavailable: \(statusError)") }
    if let overviewError { errors.append("Overview unavailable: \(overviewError)") }
    errors.append(contentsOf: status?.sourceWarnings.map { "Fleet state: \($0)" } ?? [])
    errors.append(contentsOf: overview?.warnings.map { "Overview: \($0)" } ?? [])

    return MenuPresentation(
      sections: sections,
      lastSync: lastSyncText(status?.lastSync?.at, now: now),
      errors: errors)
  }
}

public struct MenuSection: Equatable, Identifiable, Sendable {
  public var id: String { title }
  public let title: String
  public let items: [MenuItem]
}

public struct MenuItem: Equatable, Identifiable, Sendable {
  public let id: String
  public let title: String
  public let detail: String

  init(attention: AttentionItem) {
    id = "\(attention.machineID)\u{0}\(attention.repoKey)"
    title =
      attention.name.count <= 30
      ? attention.name : String(attention.name.prefix(27)) + "..."
    let reason = attention.dominantReason.replacingOccurrences(of: "_", with: " ")
    detail = attention.machineID + " · " + reason
  }
}

private func lastSyncText(_ date: Date?, now: Date) -> String {
  guard let date else { return "No successful sync yet" }
  let seconds = max(0, Int(now.timeIntervalSince(date)))
  if seconds >= 86_400 { return "Last sync \(seconds / 86_400)d ago" }
  if seconds >= 3_600 { return "Last sync \(seconds / 3_600)h ago" }
  return "Last sync \(seconds / 60)m ago"
}
