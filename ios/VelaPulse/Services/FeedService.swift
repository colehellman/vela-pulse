import Foundation
import SwiftData

/// Pull-based feed synchronisation per TRD §5.1.
///
/// - Page 1 always fetches fresh (new snapshot_id, clears cursor).
/// - Page 2+ uses the stored snapshotID + nextCursor.
/// - On 410 Gone (snapshot TTL exceeded), falls back to Page 1.
/// - Articles are upserted into SwiftData by contentHash (stable dedup key).
@MainActor
final class FeedService: ObservableObject {
    @Published var articles: [Article] = []
    @Published var isLoading = false
    @Published var error: Error?

    private let api: APIClient
    private let modelContext: ModelContext
    private let freezeWindow = FreezeWindow()

    private var currentSnapshotID: String?
    private var nextCursor: String?
    private var hasMore = true

    init(api: APIClient, modelContext: ModelContext) {
        self.api = api
        self.modelContext = modelContext
    }

    // MARK: - Public

    /// Fetch Page 1 (fresh pull). Clears pagination state and applies freeze window.
    func refresh() async {
        guard !isLoading else { return }
        isLoading = true
        error = nil
        currentSnapshotID = nil
        nextCursor = nil
        hasMore = true
        freezeWindow.clear()

        do {
            let resp = try await api.fetchFeed(limit: 20)
            let fetched = try upsert(dtos: resp.articles)
            currentSnapshotID = resp.snapshotId
            nextCursor = resp.nextCursor
            hasMore = resp.nextCursor != nil

            freezeWindow.freeze(articles: fetched)
            articles = freezeWindow.apply(to: fetched)
        } catch {
            self.error = error
        }
        isLoading = false
    }

    /// Load the next page using the current snapshot. Falls back to refresh() on 410.
    func loadMore() async {
        guard !isLoading, hasMore,
              let snapID = currentSnapshotID,
              let cursor = nextCursor
        else { return }

        isLoading = true
        do {
            let resp = try await api.fetchFeedPage(snapshotID: snapID, cursor: cursor, limit: 20)
            let fetched = try upsert(dtos: resp.articles)
            nextCursor = resp.nextCursor
            hasMore = resp.nextCursor != nil
            // Don't update freeze window on subsequent pages.
            articles = freezeWindow.apply(to: articles + fetched)
        } catch VelaError.snapshotExpired {
            // TRD §4.2: snapshot TTL exceeded — force a fresh page-1 pull.
            await refresh()
        } catch {
            self.error = error
        }
        isLoading = false
    }

    // MARK: - Private

    /// Upserts DTOs into SwiftData by contentHash, with a legacy fallback to id for
    /// rows persisted before contentHash stored the real server hash.
    @discardableResult
    private func upsert(dtos: [APIClient.ArticleDTO]) throws -> [Article] {
        var result: [Article] = []
        for dto in dtos {
            let existing = try modelContext.fetch(
                FetchDescriptor<Article>(
                    predicate: #Predicate {
                        $0.contentHash == dto.contentHash || $0.id == dto.id
                    }
                )
            ).first

            if let existing {
                existing.contentHash = dto.contentHash
                existing.title = dto.title
                existing.canonicalURL = dto.canonicalURL
                existing.sourceDomain = dto.sourceDomain
                existing.publishedAt = dto.publishedAt
                existing.pulseScore = dto.pulseScore
                existing.syncedAt = .now
                result.append(existing)
            } else {
                let article = Article(
                    id: dto.id,
                    contentHash: dto.contentHash,
                    title: dto.title,
                    canonicalURL: dto.canonicalURL,
                    sourceDomain: dto.sourceDomain,
                    publishedAt: dto.publishedAt,
                    pulseScore: dto.pulseScore
                )
                modelContext.insert(article)
                result.append(article)
            }
        }
        try modelContext.save()
        return result
    }
}
