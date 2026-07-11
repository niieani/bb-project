import Testing

@testable import BBMenuBarCore

@Suite("platform runtime configuration")
struct PlatformRuntimeConfigurationTests {
  @Test("installed launch enables production platform services")
  func installed() {
    #expect(PlatformRuntimeConfiguration(arguments: ["BBMenuBar"]).platformServicesEnabled)
  }

  @Test("fixture executable launch disables production platform services")
  func development() {
    #expect(
      !PlatformRuntimeConfiguration(arguments: ["BBMenuBar", "--bb-executable", "/tmp/bb"])
        .platformServicesEnabled)
  }

  @Test("development launch can explicitly opt into platform testing")
  func optIn() {
    #expect(
      PlatformRuntimeConfiguration(arguments: [
        "BBMenuBar", "--bb-executable", "/tmp/bb", "--enable-platform-services",
      ]).platformServicesEnabled)
  }
}
