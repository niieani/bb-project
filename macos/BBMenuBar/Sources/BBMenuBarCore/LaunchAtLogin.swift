public enum LaunchAtLoginState: Equatable, Sendable {
  case enabled
  case requiresApproval
  case failed(String)

  public var errorText: String? {
    switch self {
    case .enabled: nil
    case .requiresApproval: "Launch at login requires approval in System Settings"
    case .failed(let detail): "Launch at login failed: \(detail)"
    }
  }
}

public enum LaunchAtLoginRegistrationStatus: Equatable, Sendable {
  case notRegistered
  case enabled
  case requiresApproval
  case notFound
}

public enum LaunchAtLoginRegistrationDecision: Equatable, Sendable {
  case register
  case complete(LaunchAtLoginState)

  public static func decide(
    _ status: LaunchAtLoginRegistrationStatus
  ) -> LaunchAtLoginRegistrationDecision {
    switch status {
    case .notRegistered, .notFound: .register
    case .enabled: .complete(.enabled)
    case .requiresApproval: .complete(.requiresApproval)
    }
  }
}

public protocol LaunchAtLoginService: Sendable {
  func ensureEnabled() async -> LaunchAtLoginState
}

public struct LaunchAtLoginCoordinator: Sendable {
  private let service: any LaunchAtLoginService

  public init(service: any LaunchAtLoginService) { self.service = service }

  public func ensureEnabled() async -> LaunchAtLoginState {
    await service.ensureEnabled()
  }
}
