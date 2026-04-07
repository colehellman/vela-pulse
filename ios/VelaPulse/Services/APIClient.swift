import Foundation

/// Lightweight URLSession wrapper for Vela Pulse gateway API calls.
final class APIClient {
    private let baseURL: URL
    private let session: URLSession
    var authToken: String?

    init(baseURL: URL, session: URLSession = .shared) {
        self.baseURL = baseURL
        self.session = session
    }

    struct FeedResponse: Decodable {
        let articles: [ArticleDTO]
        let nextCursor: String?
        let snapshotId: String
        let total: Int

        enum CodingKeys: String, CodingKey {
            case articles
            case nextCursor = "next_cursor"
            case snapshotId = "snapshot_id"
            case total
        }
    }

    struct ArticleDTO: Decodable {
        let id: String
        let title: String
        let canonicalURL: String
        let sourceDomain: String
        let publishedAt: Date
        let pulseScore: Double

        enum CodingKeys: String, CodingKey {
            case id, title
            case canonicalURL  = "canonical_url"
            case sourceDomain  = "source_domain"
            case publishedAt   = "published_at"
            case pulseScore    = "pulse_score"
        }
    }

    struct SIWAResponse: Decodable {
        let token: String
        let userId: String
        enum CodingKeys: String, CodingKey {
            case token
            case userId = "user_id"
        }
    }

    // MARK: - Feed

    /// Fetches Page 1 of the feed (fresh merge).
    func fetchFeed(limit: Int = 20) async throws -> FeedResponse {
        var url = baseURL.appending(path: "/v1/feed")
        url.append(queryItems: [URLQueryItem(name: "limit", value: "\(limit)")])
        return try await get(url: url)
    }

    /// Fetches a subsequent page using an existing snapshot.
    func fetchFeedPage(snapshotID: String, cursor: String, limit: Int = 20) async throws -> FeedResponse {
        var url = baseURL.appending(path: "/v1/feed")
        url.append(queryItems: [
            URLQueryItem(name: "snapshot_id", value: snapshotID),
            URLQueryItem(name: "cursor", value: cursor),
            URLQueryItem(name: "limit", value: "\(limit)"),
        ])
        return try await get(url: url)
    }

    // MARK: - Auth

    /// Exchanges an Apple id_token for a Vela internal JWT.
    func signInWithApple(idToken: String) async throws -> SIWAResponse {
        let url = baseURL.appending(path: "/v1/auth/siwa")
        var req = URLRequest(url: url)
        req.httpMethod = "POST"
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        req.httpBody = try JSONEncoder().encode(["id_token": idToken])
        return try await perform(request: req)
    }

    // MARK: - Private

    private func get<T: Decodable>(url: URL) async throws -> T {
        var req = URLRequest(url: url)
        req.httpMethod = "GET"
        return try await perform(request: req)
    }

    private func perform<T: Decodable>(request: URLRequest) async throws -> T {
        var req = request
        if let token = authToken {
            req.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601

        let (data, response) = try await session.data(for: req)
        guard let http = response as? HTTPURLResponse else {
            throw URLError(.badServerResponse)
        }

        if http.statusCode == 410 {
            // Snapshot expired — caller should re-fetch Page 1.
            throw VelaError.snapshotExpired
        }
        guard (200..<300).contains(http.statusCode) else {
            throw VelaError.httpError(http.statusCode)
        }
        return try decoder.decode(T.self, from: data)
    }
}

enum VelaError: Error {
    case snapshotExpired
    case httpError(Int)
}
