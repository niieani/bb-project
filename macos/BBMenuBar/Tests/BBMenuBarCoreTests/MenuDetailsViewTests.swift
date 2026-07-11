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
    #expect(MenuDetailsLayout.height(for: presentation) == 480)
    let host = NSHostingView(rootView: MenuDetailsView(presentation: presentation))
    let fittingSize = host.fittingSize
    host.frame = NSRect(origin: .zero, size: fittingSize)
    host.layoutSubtreeIfNeeded()

    let scrollView = descendants(of: host).compactMap { $0 as? NSScrollView }.first
    #expect(fittingSize.height == 480)
    #expect(scrollView?.frame.height == 480)
  }
}

@MainActor
private func descendants(of view: NSView) -> [NSView] {
  view.subviews.flatMap { [$0] + descendants(of: $0) }
}
