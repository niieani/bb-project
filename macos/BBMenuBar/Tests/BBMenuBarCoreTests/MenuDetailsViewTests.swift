import AppKit
import SwiftUI
import Testing

@testable import BBMenuBarCore

@Suite("menu details scrolling")
struct MenuDetailsViewTests {
  @Test("repository row renders supplied action label and attributed failure")
  @MainActor
  func actionAndFailure() {
    let item = MenuItem(
      attention: AttentionItem(
        machineID: "machine-a", repoKey: "software/api", name: "api", state: .pending,
        dominantReason: "clone_required", reasons: ["clone_required"],
        lastActivityAt: .distantPast, eligible: false),
      actions: [ProjectAction(kind: "sync", id: "sync", label: "Update")])
    let row = MenuItemOperationPresentation(item: item, failure: "pull failed")
    #expect(row.actionLabel == "Update")
    #expect(row.failure == "pull failed")
  }

  @Test("one fix is direct and several fixes use a selection label")
  func fixLabels() {
    let attention = AttentionItem(machineID: "machine-a", repoKey: "software/api", name: "api", state: .blocked, dominantReason: "catalog_mismatch", reasons: ["catalog_mismatch"], lastActivityAt: .distantPast, eligible: true)
    let one = MenuItem(attention: attention, actions: [ProjectAction(kind: "fix", id: "move-to-catalog", label: "Move to expected catalog")])
    #expect(MenuItemOperationPresentation(item: one, failure: nil).actionLabel == "Fix")
    let many = MenuItem(attention: attention, actions: [ProjectAction(kind: "fix", id: "move-to-catalog", label: "Move"), ProjectAction(kind: "fix", id: "align-remote-format", label: "Align")])
    #expect(MenuItemOperationPresentation(item: many, failure: nil).actionLabel == "Fix…")
  }

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
