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
              VStack(alignment: .leading, spacing: 1) {
                Text(item.title)
                Text(item.detail).font(.caption).foregroundStyle(.secondary)
              }
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
