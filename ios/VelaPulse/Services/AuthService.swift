import AuthenticationServices
import Foundation
import SwiftData

/// Manages Sign In With Apple flow and internal JWT lifecycle.
@MainActor
final class AuthService: NSObject, ObservableObject {
    @Published var currentUserID: String?
    @Published var isSignedIn: Bool = false

    private let api: APIClient
    private let modelContext: ModelContext

    init(api: APIClient, modelContext: ModelContext) {
        self.api = api
        self.modelContext = modelContext
        super.init()
        restoreSession()
    }

    // MARK: - Sign In With Apple

    func signIn() {
        let request = ASAuthorizationAppleIDProvider().createRequest()
        request.requestedScopes = [.email]

        let controller = ASAuthorizationController(authorizationRequests: [request])
        controller.delegate = self
        controller.performRequests()
    }

    func signOut() {
        // Delete stored User model.
        do {
            let users = try modelContext.fetch(FetchDescriptor<User>())
            users.forEach { modelContext.delete($0) }
            try modelContext.save()
        } catch {}
        api.authToken = nil
        currentUserID = nil
        isSignedIn = false
    }

    // MARK: - Session restore

    private func restoreSession() {
        guard
            let user = (try? modelContext.fetch(FetchDescriptor<User>()))?.first,
            user.isTokenValid
        else { return }
        api.authToken = user.token
        currentUserID = user.id
        isSignedIn = true
    }
}

// MARK: - ASAuthorizationControllerDelegate

extension AuthService: ASAuthorizationControllerDelegate {
    nonisolated func authorizationController(
        controller: ASAuthorizationController,
        didCompleteWithAuthorization authorization: ASAuthorization
    ) {
        guard
            let credential = authorization.credential as? ASAuthorizationAppleIDCredential,
            let tokenData = credential.identityToken,
            let idToken = String(data: tokenData, encoding: .utf8)
        else { return }

        Task { @MainActor in
            do {
                let resp = try await api.signInWithApple(idToken: idToken)
                api.authToken = resp.token

                // Persist the user session.
                let expiry = Date().addingTimeInterval(30 * 24 * 3600)
                let user = User(id: resp.userId, token: resp.token, tokenExpiresAt: expiry)
                modelContext.insert(user)
                try modelContext.save()

                currentUserID = resp.userId
                isSignedIn = true
            } catch {
                // Surface error in production; log here for now.
                print("SIWA exchange failed: \(error)")
            }
        }
    }

    nonisolated func authorizationController(
        controller: ASAuthorizationController,
        didCompleteWithError error: Error
    ) {
        print("SIWA failed: \(error)")
    }
}
