import Testing

@testable import BBMenuBarCore

@Suite("launch at login")
struct LaunchAtLoginTests {
  @Test("registration states are explicit")
  func states() async {
    #expect(
      await LaunchAtLoginCoordinator(service: StubLoginService(result: .enabled)).ensureEnabled()
        == .enabled)
    #expect(
      await LaunchAtLoginCoordinator(service: StubLoginService(result: .requiresApproval))
        .ensureEnabled() == .requiresApproval)
    #expect(
      await LaunchAtLoginCoordinator(service: StubLoginService(result: .failed("registration")))
        .ensureEnabled() == .failed("registration"))
  }
}

private struct StubLoginService: LaunchAtLoginService {
  let result: LaunchAtLoginState
  func ensureEnabled() async -> LaunchAtLoginState { result }
}
