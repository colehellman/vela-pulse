import Foundation
import SwiftData

/// Flat SwiftData model for a Vela Pulse article.
///
/// No `@Relationship` tags — SwiftData's relationship graph loading on access
/// causes frame drops in high-frequency list scrolling. All foreign keys are
/// stored as plain String IDs per TRD §5.1.
@Model
final class Article {
    @Attribute(.unique) var id: String
    @Attribute(.unique) var contentHash: String
    var title: String
    var canonicalURL: String
    var sourceDomain: String
    var publishedAt: Date
    var pulseScore: Double
    /// nil = global article; non-nil = private/user-added source.
    var userID: String?
    var syncedAt: Date

    init(
        id: String,
        contentHash: String,
        title: String,
        canonicalURL: String,
        sourceDomain: String,
        publishedAt: Date,
        pulseScore: Double,
        userID: String? = nil
    ) {
        self.id = id
        self.contentHash = contentHash
        self.title = title
        self.canonicalURL = canonicalURL
        self.sourceDomain = sourceDomain
        self.publishedAt = publishedAt
        self.pulseScore = pulseScore
        self.userID = userID
        self.syncedAt = .now
    }
}
