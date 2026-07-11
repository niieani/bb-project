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
