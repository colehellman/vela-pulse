import Foundation

/// Prevents the top-10 articles from being re-ordered within a 120-second window
/// after a fresh Page 1 fetch. This prevents "flicker" when pulse scores update
/// between pulls (TRD §2.1 Temporal Pinning).
final class FreezeWindow {
    private let windowDuration: TimeInterval
    private var frozenUntil: Date = .distantPast
    private var frozenTopIDs: [String] = []

    init(windowDuration: TimeInterval = 120) {
        self.windowDuration = windowDuration
    }

    var isActive: Bool { Date() < frozenUntil }

    /// Call this when a fresh Page 1 response arrives.
    /// Captures the IDs of the first 10 articles and freezes their order.
    func freeze(articles: [Article]) {
        frozenTopIDs = articles.prefix(10).map(\.id)
        frozenUntil = Date().addingTimeInterval(windowDuration)
    }

    /// Returns articles with the frozen top-10 order preserved.
    /// Articles not in the frozen set are appended after, sorted by pulseScore.
    func apply(to articles: [Article]) -> [Article] {
        guard isActive, !frozenTopIDs.isEmpty else { return articles }

        let lookup = Dictionary(uniqueKeysWithValues: articles.map { ($0.id, $0) })
        let frozenTop = frozenTopIDs.compactMap { lookup[$0] }
        let frozenSet = Set(frozenTopIDs)
        let rest = articles.filter { !frozenSet.contains($0.id) }
        return frozenTop + rest
    }

    /// Clears the freeze, e.g. on manual pull-to-refresh.
    func clear() {
        frozenUntil = .distantPast
        frozenTopIDs = []
    }
}
