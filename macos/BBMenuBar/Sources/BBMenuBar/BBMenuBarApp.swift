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
    let model = MenuBarModel(client: ProcessStatusClient(executableURL: executableURL))
    _model = State(initialValue: model)
    Task { await model.refresh() }
  }

  var body: some Scene {
    MenuBarExtra {
      if let error = model.errorMessage {
        Text("bb status unavailable")
        Text(shortMenuTitle(error))
      } else {
        Text("bb fleet status")
      }
      Divider()
      Button("Refresh") {
        Task { await model.refresh() }
      }
      Button("Quit") {
        NSApplication.shared.terminate(nil)
      }
    } label: {
      Text(model.title.text)
    }
  }

  private func shortMenuTitle(_ title: String) -> String {
    title.count <= 30 ? title : String(title.prefix(27)) + "..."
  }
}
