import Foundation
import SwiftData
import SwiftUI

/// Dependency injection container.
/// Constructed once at app launch and injected via .environment().
@MainActor
final class AppEnvironment: ObservableObject {
    let api: APIClient
    let auth: AuthService
    let feed: FeedService

    init(modelContext: ModelContext) {
        // Default gateway URL; override via VELA_GATEWAY_URL env var for dev.
        let baseURL = URL(string: ProcessInfo.processInfo.environment["VELA_GATEWAY_URL"]
            ?? "https://api.vela.pulse")!

        api  = APIClient(baseURL: baseURL)
        auth = AuthService(api: api, modelContext: modelContext)
        feed = FeedService(api: api, modelContext: modelContext)
    }
}
