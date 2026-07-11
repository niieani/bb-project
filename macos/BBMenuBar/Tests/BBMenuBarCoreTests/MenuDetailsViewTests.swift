import AppKit
import SwiftUI
import Testing

@testable import BBMenuBarCore

@Suite("menu details scrolling")
struct MenuDetailsViewTests {
  @Test("long popup content is hosted in a vertical scroll view")
  @MainActor
  func longContentScrolls() {
    let presentation = MenuPresentation(
      sections: [],
      lastSync: "Last sync 1m ago",
      errors: (1...100).map { "Attention item \($0)" })
    let host = NSHostingView(rootView: MenuDetailsView(presentation: presentation))
    host.frame = NSRect(x: 0, y: 0, width: 320, height: 480)
    host.layoutSubtreeIfNeeded()

    #expect(descendants(of: host).contains { $0 is NSScrollView })
  }
}

@MainActor
private func descendants(of view: NSView) -> [NSView] {
  view.subviews.flatMap { [$0] + descendants(of: $0) }
}
