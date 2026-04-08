import Foundation
import Security

/// Keychain wrapper for storing the gateway JWT.
/// Token is stored as a generic password keyed to service + account.
/// Survives app reinstalls (device Keychain persists across reinstalls by default).
/// Not synced to iCloud — kSecAttrSynchronizable is not set.
enum KeychainService {
    private static let service = "com.vela.pulse"
    private static let account = "gateway-jwt"

    enum KeychainError: Error {
        case saveFailed(OSStatus)
        case deleteFailed(OSStatus)
    }

    /// Saves (or overwrites) the token in the Keychain. Throws on write failure.
    static func save(_ token: String) throws {
        let data = Data(token.utf8)
        let query: [CFString: Any] = [
            kSecClass:       kSecClassGenericPassword,
            kSecAttrService: service,
            kSecAttrAccount: account,
        ]
        // Delete any existing item first so we can overwrite cleanly.
        SecItemDelete(query as CFDictionary)

        var attrs = query
        attrs[kSecValueData] = data
        // kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly: accessible after first
        // unlock post-boot, device-only (excludes iCloud/iTunes backups).
        attrs[kSecAttrAccessible] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
        let status = SecItemAdd(attrs as CFDictionary, nil)
        guard status == errSecSuccess else {
            throw KeychainError.saveFailed(status)
        }
    }

    /// Loads the token from the Keychain. Returns nil if absent or unreadable.
    static func load() -> String? {
        let query: [CFString: Any] = [
            kSecClass:            kSecClassGenericPassword,
            kSecAttrService:      service,
            kSecAttrAccount:      account,
            kSecReturnData:       kCFBooleanTrue!,
            kSecMatchLimit:       kSecMatchLimitOne,
        ]
        var result: AnyObject?
        let status = SecItemCopyMatching(query as CFDictionary, &result)
        guard status == errSecSuccess,
              let data = result as? Data,
              let token = String(data: data, encoding: .utf8)
        else { return nil }
        return token
    }

    /// Removes the token from the Keychain (called on sign-out). Throws on failure.
    /// errSecItemNotFound is treated as success — item was already gone.
    @discardableResult
    static func delete() throws -> OSStatus {
        let query: [CFString: Any] = [
            kSecClass:       kSecClassGenericPassword,
            kSecAttrService: service,
            kSecAttrAccount: account,
        ]
        let status = SecItemDelete(query as CFDictionary)
        guard status == errSecSuccess || status == errSecItemNotFound else {
            throw KeychainError.deleteFailed(status)
        }
        return status
    }
}
