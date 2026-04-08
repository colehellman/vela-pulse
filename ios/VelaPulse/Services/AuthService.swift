import AuthenticationServices
import Foundation
import SwiftData

/// Manages Sign In With Apple flow and internal JWT lifecycle.
/// The JWT is stored in the Keychain (KeychainService), not SwiftData.
@MainActor
final class AuthService: NSObject, ObservableObject {
    @Published var currentUserID: String?
    @Published var isSignedIn: Bool = false
    @Published var error: String?

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
        error = nil
        let request = ASAuthorizationAppleIDProvider().createRequest()
        request.requestedScopes = [.email]
        let controller = ASAuthorizationController(authorizationRequests: [request])
        controller.delegate = self
        controller.performRequests()
    }

    func signOut() {
        KeychainService.delete()
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
            let token = KeychainService.load(),
            let user = (try? modelContext.fetch(FetchDescriptor<User>()))?.first
        else { return }
        api.authToken = token
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
        else {
            Task { @MainActor in self.error = "Could not read Apple credential." }
            return
        }

        Task { @MainActor in
            do {
                let resp = try await api.signInWithApple(idToken: idToken)

                // Store JWT in Keychain — never in SwiftData/SQLite.
                KeychainService.save(resp.token)
                api.authToken = resp.token

                // Persist only the stable user ID in SwiftData.
                let users = try modelContext.fetch(FetchDescriptor<User>())
                users.forEach { modelContext.delete($0) }
                modelContext.insert(User(id: resp.userId))
                try modelContext.save()

                currentUserID = resp.userId
                isSignedIn = true
            } catch {
                self.error = "Sign in failed. Please try again."
            }
        }
    }

    nonisolated func authorizationController(
        controller: ASAuthorizationController,
        didCompleteWithError error: Error
    ) {
        // ASAuthorizationError.canceled (code 1001) is user-initiated — don't show an alert.
        let asError = error as? ASAuthorizationError
        guard asError?.code != .canceled else { return }
        Task { @MainActor in self.error = "Sign in failed. Please try again." }
    }
}
