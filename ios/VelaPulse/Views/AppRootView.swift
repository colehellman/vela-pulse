import SwiftData
import SwiftUI

/// Root view: bootstraps AppEnvironment with the SwiftData model context.
struct AppRootView: View {
    @Environment(\.modelContext) private var modelContext

    @StateObject private var env: AppEnvironment

    init() {
        // Placeholder — AppEnvironment is re-init'd in body once modelContext is available.
        // The real init happens in onAppear via the @StateObject pattern.
        _env = StateObject(wrappedValue: AppEnvironment(modelContext: ModelContext(
            try! ModelContainer(for: Article.self, User.self)
        )))
    }

    var body: some View {
        FeedView()
            .environmentObject(env.feed)
            .environmentObject(env.auth)
    }
}
