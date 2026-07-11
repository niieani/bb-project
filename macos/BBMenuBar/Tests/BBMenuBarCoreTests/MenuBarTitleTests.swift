import Foundation
import Testing

@testable import BBMenuBarCore

@Suite("menu bar title")
struct MenuBarTitleTests {
  @Test("all-green shows check and local repository count")
  func healthy() throws {
    let state = try MenuBarTitleState.statusJSON(fixture(named: "all-synced"))
    #expect(state == .healthy(repoCount: 1))
    #expect(state.text == "✅ 1")
  }

  @Test("attention shows warning and eligible fleet count")
  func attention() throws {
    let state = try MenuBarTitleState.statusJSON(fixture(named: "mixed"))
    #expect(state == .attention(count: 4))
    #expect(state.text == "⚠️ 4")
  }

  @Test("malformed JSON becomes an explicit error")
  func malformed() {
    #expect(throws: (any Error).self) {
      try MenuBarTitleState.statusJSON(Data("not json".utf8))
    }
    #expect(MenuBarTitleState.error.text == "⛔ Error")
  }

  @Test("client failure becomes an explicit error")
  @MainActor
  func clientFailure() async {
    let model = MenuBarModel(client: StubStatusClient(result: .failure(TestFailure())))
    await model.refresh()
    #expect(model.title == .error)
    #expect(model.presentation.errors == ["Status unavailable: TestFailure()"])
  }

  @Test("malformed client payload maps to the view-model error state")
  @MainActor
  func malformedClientPayload() async {
    let model = MenuBarModel(client: StubStatusClient(result: .success(Data("not json".utf8))))
    await model.refresh()
    #expect(model.title == .error)
    #expect(model.presentation.errors.first?.hasPrefix("Status unavailable:") == true)
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

private struct TestFailure: Error {}

private struct StubStatusClient: BBClient {
  let result: Result<Data, any Error>

  func statusJSON() async throws -> Data {
    try result.get()
  }

  func overviewJSON() async throws -> Data {
    Data(#"{"machines":[],"repos":[],"synced_everywhere":0,"warnings":[]}"#.utf8)
  }

  func sync() async throws {}
}
