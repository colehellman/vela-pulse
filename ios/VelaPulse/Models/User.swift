import Foundation
import SwiftData

/// Local representation of the authenticated user.
/// Only the stable user ID is persisted in SwiftData.
/// The JWT is stored in the Keychain via KeychainService — never in SQLite.
@Model
final class User {
    @Attribute(.unique) var id: String  // internal UUID from gateway
    var createdAt: Date

    init(id: String) {
        self.id = id
        self.createdAt = .now
    }
}
