import Foundation
import SwiftData

/// Local representation of the authenticated user.
/// Stored in SwiftData so auth state persists across launches.
@Model
final class User {
    @Attribute(.unique) var id: String       // internal UUID from gateway
    var token: String                         // internal JWT (stored here for restore; Keychain preferred for production)
    var tokenExpiresAt: Date
    var createdAt: Date

    init(id: String, token: String, tokenExpiresAt: Date) {
        self.id = id
        self.token = token
        self.tokenExpiresAt = tokenExpiresAt
        self.createdAt = .now
    }

    var isTokenValid: Bool {
        tokenExpiresAt > Date()
    }
}
