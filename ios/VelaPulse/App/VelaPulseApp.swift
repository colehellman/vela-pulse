import SwiftData
import SwiftUI

@main
struct VelaPulseApp: App {
    private let container: ModelContainer
    @StateObject private var env: AppEnvironment

    init() {
        let schema = Schema([Article.self, User.self])
        let config = ModelConfiguration(schema: schema, isStoredInMemoryOnly: false)
        let c = try! ModelContainer(for: schema, configurations: config)
        container = c
        // Share one ModelContext between the service layer and SwiftUI's environment.
        _env = StateObject(wrappedValue: AppEnvironment(modelContext: ModelContext(c)))
    }

    var body: some Scene {
        WindowGroup {
            FeedView()
                .environmentObject(env.feed)
                .environmentObject(env.auth)
                .modelContainer(container)
        }
    }
}
