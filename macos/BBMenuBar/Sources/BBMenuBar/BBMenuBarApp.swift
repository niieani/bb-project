import AppKit
import BBMenuBarCore
import SwiftUI

@main
struct BBMenuBarApp: App {
  @State private var model: MenuBarModel

  init() {
    let arguments = CommandLine.arguments
    let runtime = PlatformRuntimeConfiguration(arguments: arguments)
    let executableURL: URL?
    if let argumentIndex = arguments.firstIndex(of: "--bb-executable") {
      guard arguments.indices.contains(argumentIndex + 1) else {
        fatalError("--bb-executable requires a path")
      }
      executableURL = URL(fileURLWithPath: arguments[argumentIndex + 1])
    } else {
      executableURL = nil
    }
    let client = ProcessBBClient(executableURL: executableURL)
    let model: MenuBarModel
    if runtime.platformServicesEnabled {
      let attentionRouter = AttentionWindowRouter()
      let notificationClient = SystemNotificationClient(router: attentionRouter)
      let notifications = NotificationCoordinator(
        client: notificationClient, store: UserDefaultsNotificationStateStore())
      model = MenuBarModel(client: client, notifications: notifications)
    } else {
      model = MenuBarModel(client: client)
    }
    _model = State(initialValue: model)
    model.start(events: SystemRefreshEventSource(interval: .seconds(300)))
    Task { await model.refresh() }
    if runtime.platformServicesEnabled {
      Task {
        await model.configureLaunchAtLogin(
          LaunchAtLoginCoordinator(service: SystemLaunchAtLoginService()))
      }
    }
  }

  var body: some Scene {
    MenuBarExtra {
      VStack(alignment: .leading, spacing: 10) {
        MenuDetailsView(presentation: model.presentation)

        Text(model.presentation.lastSync).font(.caption).foregroundStyle(.secondary)

        HStack {
          Button(model.isSyncing ? "Syncing…" : "Sync now") {
            Task { await model.syncNow() }
          }
          .disabled(model.isSyncing)
          Button("Refresh") {
            Task { await model.refresh() }
          }
          Button("Quit") {
            NSApplication.shared.terminate(nil)
          }
        }
      }
      .padding(12)
      .frame(width: 320)
    } label: {
      Text(model.title.text)
    }
    .menuBarExtraStyle(.window)
  }
}
