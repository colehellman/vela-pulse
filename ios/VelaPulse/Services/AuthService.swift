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
        do {
            try KeychainService.delete()
        } catch {
            // JWT deletion failed — session may persist on next launch.
            // Surface to user so they know to restart the app.
            self.error = "Sign out may be incomplete. Please restart the app."
        }
        do {
            let users = try modelContext.fetch(FetchDescriptor<User>())
            users.forEach { modelContext.delete($0) }
            try modelContext.save()
        } catch {
            // SwiftData cleanup failed. JWT is gone so the user is signed out,
            // but the local User record may linger until next launch.
            self.error = "Sign out completed but local data could not be cleared. Restart the app if issues persist."
        }
        api.authToken = nil
        currentUserID = nil
        isSignedIn = false
    }

    // MARK: - Session restore

    private func restoreSession() {
        guard let token = KeychainService.load() else { return }
        do {
            guard let user = try modelContext.fetch(FetchDescriptor<User>()).first else { return }
            api.authToken = token
            currentUserID = user.id
            isSignedIn = true
        } catch {
            // SwiftData unavailable (e.g. schema migration failure after update).
            // JWT exists but we can't read the user record — force re-auth.
            self.error = "Could not restore your session. Please sign in again."
        }
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
                try KeychainService.save(resp.token)
                api.authToken = resp.token

                // Persist only the stable user ID in SwiftData.
                let users = try modelContext.fetch(FetchDescriptor<User>())
                users.forEach { modelContext.delete($0) }
                modelContext.insert(User(id: resp.userId))
                try modelContext.save()

                currentUserID = resp.userId
                isSignedIn = true
            } catch let urlError as URLError {
                self.error = urlError.code == .notConnectedToInternet
                    ? "No network connection. Check your internet and try again."
                    : "Could not reach the server. Please try again."
            } catch VelaError.httpError(let code) where (500..<600).contains(code) {
                self.error = "The server is temporarily unavailable. Please try again in a moment."
            } catch VelaError.httpError {
                self.error = "Sign in was rejected. Please try again or contact support."
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
