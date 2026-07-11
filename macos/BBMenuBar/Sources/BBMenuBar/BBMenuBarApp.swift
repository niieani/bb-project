import AppKit
import BBMenuBarCore
import SwiftUI

@main
struct BBMenuBarApp: App {
  @State private var model: MenuBarModel

  init() {
    let arguments = CommandLine.arguments
    let executableURL: URL?
    if let argumentIndex = arguments.firstIndex(of: "--bb-executable") {
      guard arguments.indices.contains(argumentIndex + 1) else {
        fatalError("--bb-executable requires a path")
      }
      executableURL = URL(fileURLWithPath: arguments[argumentIndex + 1])
    } else {
      executableURL = nil
    }
    let model = MenuBarModel(client: ProcessBBClient(executableURL: executableURL))
    _model = State(initialValue: model)
    model.start(events: SystemRefreshEventSource(interval: .seconds(300)))
    Task { await model.refresh() }
  }

  var body: some Scene {
    MenuBarExtra {
      VStack(alignment: .leading, spacing: 10) {
        ForEach(model.presentation.sections) { section in
          VStack(alignment: .leading, spacing: 4) {
            Text(section.title).font(.headline)
            ForEach(section.items) { item in
              VStack(alignment: .leading, spacing: 1) {
                Text(item.title)
                Text(item.detail).font(.caption).foregroundStyle(.secondary)
              }
            }
          }
          Divider()
        }

        ForEach(model.presentation.errors, id: \.self) { error in
          Text(error).font(.caption).foregroundStyle(.red)
        }

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
