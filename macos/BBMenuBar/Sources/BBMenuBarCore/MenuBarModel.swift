import Foundation
import Observation

@MainActor
@Observable
public final class MenuBarModel {
  public private(set) var title: MenuBarTitleState = .loading
  public private(set) var errorMessage: String?

  private let client: any StatusClient

  public init(client: any StatusClient) {
    self.client = client
  }

  public func refresh() async {
    do {
      title = try MenuBarTitleState.statusJSON(await client.statusJSON())
      errorMessage = nil
    } catch {
      title = .error
      errorMessage = String(describing: error)
    }
  }
}
