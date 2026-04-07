import Foundation

/// Encodes and decodes the Base64(published_at_unix | article_id) cursor per TRD §4.2.
enum CursorCoder {
    /// Decodes a Base64 cursor returned by the gateway into its components.
    static func decode(_ cursor: String) -> (publishedAt: Date, articleID: String)? {
        guard
            let data = Data(base64Encoded: cursor),
            let raw = String(data: data, encoding: .utf8)
        else { return nil }

        let parts = raw.split(separator: "|", maxSplits: 1)
        guard parts.count == 2,
              let unix = TimeInterval(parts[0])
        else { return nil }

        return (Date(timeIntervalSince1970: unix), String(parts[1]))
    }

    /// Encodes a (publishedAt, articleID) pair into the Base64 cursor format.
    static func encode(publishedAt: Date, articleID: String) -> String {
        let raw = "\(Int64(publishedAt.timeIntervalSince1970))|\(articleID)"
        return Data(raw.utf8).base64EncodedString()
    }
}
