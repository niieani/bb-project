public struct PlatformRuntimeConfiguration: Equatable, Sendable {
  public let platformServicesEnabled: Bool

  public init(arguments: [String]) {
    let developmentLaunch = arguments.contains("--bb-executable")
    let explicitPlatformTesting = arguments.contains("--enable-platform-services")
    platformServicesEnabled = !developmentLaunch || explicitPlatformTesting
  }
}
