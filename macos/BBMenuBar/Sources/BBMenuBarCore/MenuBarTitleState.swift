import Foundation

public enum MenuBarTitleState: Equatable, Sendable {
  case loading
  case healthy(repoCount: Int)
  case attention(count: Int)
  case error

  public static func statusJSON(_ data: Data) throws -> Self {
    let status = try JSONDecoder.bb.decode(StatusContract.self, from: data)
    if status.attention.eligibleCount > 0 {
      return .attention(count: status.attention.eligibleCount)
    }
    return .healthy(repoCount: status.summary.total)
  }

  public var text: String {
    switch self {
    case .loading: "…"
    case .healthy(let repoCount): "✅ \(repoCount)"
    case .attention(let count): "⚠️ \(count)"
    case .error: "⛔ Error"
    }
  }
}
