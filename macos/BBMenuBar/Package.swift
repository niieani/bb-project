// swift-tools-version: 6.2

import PackageDescription

let package = Package(
  name: "BBMenuBar",
  platforms: [.macOS(.v14)],
  products: [
    .library(name: "BBMenuBarCore", targets: ["BBMenuBarCore"]),
    .executable(name: "BBMenuBar", targets: ["BBMenuBar"]),
  ],
  targets: [
    .target(name: "BBMenuBarCore"),
    .executableTarget(name: "BBMenuBar", dependencies: ["BBMenuBarCore"]),
    .testTarget(name: "BBMenuBarCoreTests", dependencies: ["BBMenuBarCore"]),
  ]
)
