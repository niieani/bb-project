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

  @Test("missing main-app registration is repaired instead of treated as terminal")
  func missingRegistration() {
    #expect(LaunchAtLoginRegistrationDecision.decide(.notFound) == .register)
    #expect(LaunchAtLoginRegistrationDecision.decide(.notRegistered) == .register)
    #expect(LaunchAtLoginRegistrationDecision.decide(.enabled) == .complete(.enabled))
    #expect(
      LaunchAtLoginRegistrationDecision.decide(.requiresApproval)
        == .complete(.requiresApproval))
  }
}

private struct StubLoginService: LaunchAtLoginService {
  let result: LaunchAtLoginState
  func ensureEnabled() async -> LaunchAtLoginState { result }
}
