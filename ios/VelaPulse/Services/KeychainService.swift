import Foundation
import Security

/// Keychain wrapper for storing the gateway JWT.
/// Token is stored as a generic password keyed to service + account so it
/// survives app reinstalls on the same device when iCloud Keychain is enabled.
enum KeychainService {
    private static let service = "com.vela.pulse"
    private static let account = "gateway-jwt"

    /// Saves (or overwrites) the token in the Keychain.
    static func save(_ token: String) {
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
        SecItemAdd(attrs as CFDictionary, nil)
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

    /// Removes the token from the Keychain (called on sign-out).
    static func delete() {
        let query: [CFString: Any] = [
            kSecClass:       kSecClassGenericPassword,
            kSecAttrService: service,
            kSecAttrAccount: account,
        ]
        SecItemDelete(query as CFDictionary)
    }
}
