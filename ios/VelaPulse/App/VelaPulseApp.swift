import SwiftData
import SwiftUI

@main
struct VelaPulseApp: App {
    private let container: ModelContainer = {
        let schema = Schema([Article.self, User.self])
        let config = ModelConfiguration(schema: schema, isStoredInMemoryOnly: false)
        return try! ModelContainer(for: schema, configurations: config)
    }()

    var body: some Scene {
        WindowGroup {
            AppRootView()
                .modelContainer(container)
        }
    }
}
