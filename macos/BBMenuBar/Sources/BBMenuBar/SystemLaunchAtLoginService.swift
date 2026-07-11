import BBMenuBarCore
import ServiceManagement

struct SystemLaunchAtLoginService: LaunchAtLoginService {
  func ensureEnabled() async -> LaunchAtLoginState {
    await MainActor.run {
      let service = SMAppService.mainApp
      switch service.status {
      case .enabled:
        return .enabled
      case .requiresApproval:
        return .requiresApproval
      case .notFound:
        return .failed("installed application not found")
      case .notRegistered:
        do {
          try service.register()
          return service.status == .enabled ? .enabled : .requiresApproval
        } catch {
          return .failed(String(describing: error))
        }
      @unknown default:
        return .failed("unknown Service Management status")
      }
    }
  }
}
