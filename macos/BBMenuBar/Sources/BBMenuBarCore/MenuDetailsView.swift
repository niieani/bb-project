import SwiftUI

public struct MenuDetailsView: View {
  private let presentation: MenuPresentation

  public init(presentation: MenuPresentation) {
    self.presentation = presentation
  }

  public var body: some View {
    ScrollView(.vertical) {
      LazyVStack(alignment: .leading, spacing: 10) {
        ForEach(presentation.sections) { section in
          VStack(alignment: .leading, spacing: 4) {
            Text(section.title).font(.headline)
            ForEach(section.items) { item in
              MenuItemRow(item: item)
            }
          }
          Divider()
        }

        ForEach(presentation.errors, id: \.self) { error in
          Text(error).font(.caption).foregroundStyle(.red)
        }
      }
      .frame(maxWidth: .infinity, alignment: .leading)
      .fixedSize(horizontal: false, vertical: true)
    }
    .scrollIndicators(.visible)
    .frame(maxHeight: 480)
  }
}

private struct MenuItemRow: View {
  let item: MenuItem
  @Environment(\.colorScheme) private var colorScheme

  var body: some View {
    VStack(alignment: .leading, spacing: 1) {
      HStack(spacing: 7) {
        Circle()
          .fill(
            Color(hex: colorScheme == .dark ? item.statusTone.darkHex : item.statusTone.lightHex)
          )
          .frame(width: 8, height: 8)
          .accessibilityHidden(true)
        Text(item.title)
      }
      Text(item.detail).font(.caption).foregroundStyle(.secondary).padding(.leading, 15)
    }
    .accessibilityElement(children: .combine)
    .accessibilityLabel("\(item.title), \(item.statusTone.accessibilityName), \(item.detail)")
  }
}

extension RepoStatusTone {
  fileprivate var accessibilityName: String {
    switch self {
    case .synced: "synced"
    case .pending: "pending"
    case .wip: "work in progress"
    case .blocked: "blocked"
    }
  }
}

extension Color {
  fileprivate init(hex: String) {
    let value = UInt64(hex.dropFirst(), radix: 16) ?? 0
    self.init(
      red: Double((value >> 16) & 0xFF) / 255,
      green: Double((value >> 8) & 0xFF) / 255,
      blue: Double(value & 0xFF) / 255)
  }
}
