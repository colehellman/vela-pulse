import SwiftData
import SwiftUI

@main
struct VelaPulseApp: App {
    private let container: ModelContainer
    @StateObject private var env: AppEnvironment

    init() {
        let schema = Schema([Article.self, User.self])
        let persistConfig = ModelConfiguration(schema: schema, isStoredInMemoryOnly: false)
        do {
            container = try ModelContainer(for: schema, configurations: persistConfig)
        } catch {
            // Store is corrupt or inaccessible (e.g. failed migration after update).
            // Fall back to in-memory so the app can at least launch. Data will not
            // persist this session; user will need to sign in again.
            let memConfig = ModelConfiguration(schema: schema, isStoredInMemoryOnly: true)
            container = try! ModelContainer(for: schema, configurations: memConfig)
        }
        // Share one ModelContext between the service layer and SwiftUI's environment.
        _env = StateObject(wrappedValue: AppEnvironment(modelContext: ModelContext(container)))
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
