import AppKit
import BBMenuBarCore
import SwiftUI

@MainActor
final class AttentionWindowRouter: NotificationRouter {
  private var window: NSWindow?

  func show(items: [FleetNotificationItem]) {
    let window = window ?? makeWindow()
    window.contentView = NSHostingView(rootView: AttentionWindow(items: items))
    self.window = window
    NSApplication.shared.activate(ignoringOtherApps: true)
    window.makeKeyAndOrderFront(nil)
  }

  private func makeWindow() -> NSWindow {
    let window = NSWindow(
      contentRect: NSRect(x: 0, y: 0, width: 460, height: 360),
      styleMask: [.titled, .closable, .miniaturizable, .resizable],
      backing: .buffered,
      defer: false)
    window.title = "Repositories needing attention"
    window.center()
    window.isReleasedWhenClosed = false
    return window
  }
}

private struct AttentionWindow: View {
  let items: [FleetNotificationItem]

  var body: some View {
    List(items, id: \.self) { item in
      VStack(alignment: .leading, spacing: 3) {
        Text(item.name).font(.headline)
        Text("\(item.machineID) · \(item.reason)").foregroundStyle(.secondary)
        Text(item.repoKey).font(.caption).foregroundStyle(.tertiary)
      }
      .padding(.vertical, 3)
    }
    .frame(minWidth: 420, minHeight: 300)
  }
}
