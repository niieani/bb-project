import Foundation
import Testing

@testable import BBMenuBarCore

@Suite("bb status contract")
struct StatusContractTests {
  @Test("decodes every shared Go fixture", arguments: ["all-synced", "mixed"])
  func decodesSharedFixture(_ name: String) throws {
    let contract = try JSONDecoder.bb.decode(StatusContract.self, from: fixture(named: name))

    #expect(contract.machineID == "machine-a")
    #expect(contract.summary.total == contract.repos.count)
  }

  @Test("preserves mixed fleet attention policy output")
  func mixedAttention() throws {
    let contract = try JSONDecoder.bb.decode(StatusContract.self, from: fixture(named: "mixed"))

    #expect(contract.attention.eligibleCount == 4)
    #expect(
      contract.attention.items.contains {
        $0.machineID == "machine-b" && $0.repoKey == "references/remote-only" && $0.eligible
      })
    #expect(contract.attention.items.contains { $0.state == .pending && !$0.eligible })
    #expect(contract.lastSync?.event == "sync_run")
  }

  @Test("decodes Go RFC 3339 nanosecond timestamps")
  func fractionalTimestamp() throws {
    struct Timestamp: Decodable { let at: Date }
    let value = try JSONDecoder.bb.decode(
      Timestamp.self,
      from: Data(#"{"at":"2026-07-10T12:00:00.123456789-07:00"}"#.utf8))
    #expect(value.at.timeIntervalSince1970 > 0)
  }

  private func fixture(named name: String) throws -> Data {
    let repository = URL(fileURLWithPath: #filePath)
      .deletingLastPathComponent()
      .deletingLastPathComponent()
      .deletingLastPathComponent()
      .deletingLastPathComponent()
      .deletingLastPathComponent()
    return try Data(contentsOf: repository.appending(path: "fixtures/status/\(name).json"))
  }
}
