import SwiftUI

/// Main feed list.
///
/// Freeze window behaviour (TRD §2.1): top-10 article positions are locked for
/// 120 seconds after Page 1 loads. The FeedService.freezeWindow handles this —
/// articles is already reordered before assignment to the @Published property.
struct FeedView: View {
    @EnvironmentObject var feed: FeedService
    @EnvironmentObject var auth: AuthService

    var body: some View {
        NavigationStack {
            Group {
                if feed.articles.isEmpty && feed.isLoading {
                    ProgressView("Loading feed…")
                        .frame(maxWidth: .infinity, maxHeight: .infinity)
                } else if feed.articles.isEmpty && feed.error == nil {
                    ContentUnavailableView(
                        "No articles yet",
                        systemImage: "newspaper",
                        description: Text("Pull to refresh")
                    )
                } else {
                    List {
                        ForEach(feed.articles, id: \.id) { article in
                            ArticleRowView(article: article)
                                .onAppear {
                                    if article.id == feed.articles.last?.id {
                                        Task { await feed.loadMore() }
                                    }
                                }
                        }

                        if feed.isLoading {
                            HStack {
                                Spacer()
                                ProgressView()
                                Spacer()
                            }
                        }
                    }
                    .listStyle(.plain)
                }
            }
            .navigationTitle("Vela Pulse")
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    if auth.isSignedIn {
                        Button("Sign Out") { auth.signOut() }
                    } else {
                        Button("Sign In") { auth.signIn() }
                    }
                }
            }
            .refreshable {
                await feed.refresh()
            }
            .task {
                if feed.articles.isEmpty {
                    await feed.refresh()
                }
            }
            // Feed error alert
            .alert("Feed unavailable", isPresented: Binding(
                get: { feed.error != nil },
                set: { if !$0 { feed.error = nil } }
            )) {
                Button("Retry") { Task { await feed.refresh() } }
                Button("Dismiss", role: .cancel) { feed.error = nil }
            } message: {
                Text(feed.error?.localizedDescription ?? "Could not load articles.")
            }
            // Auth error alert
            .alert("Sign in failed", isPresented: Binding(
                get: { auth.error != nil },
                set: { if !$0 { auth.error = nil } }
            )) {
                Button("OK", role: .cancel) { auth.error = nil }
            } message: {
                Text(auth.error ?? "Please try again.")
            }
        }
    }
}
