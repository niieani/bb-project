import BBMenuBarCore
import ServiceManagement

struct SystemLaunchAtLoginService: LaunchAtLoginService {
  func ensureEnabled() async -> LaunchAtLoginState {
    await MainActor.run {
      let service = SMAppService.mainApp
      switch LaunchAtLoginRegistrationDecision.decide(service.status.registrationStatus) {
      case .complete(let state):
        return state
      case .register:
        do {
          try service.register()
          switch LaunchAtLoginRegistrationDecision.decide(service.status.registrationStatus) {
          case .complete(let state): return state
          case .register: return .failed("registration did not create the installed application service")
          }
        } catch {
          return .failed(String(describing: error))
        }
      }
    }
  }
}

private extension SMAppService.Status {
  var registrationStatus: LaunchAtLoginRegistrationStatus {
    switch self {
    case .notRegistered: .notRegistered
    case .enabled: .enabled
    case .requiresApproval: .requiresApproval
    case .notFound: .notFound
    @unknown default: .notFound
    }
  }
}
