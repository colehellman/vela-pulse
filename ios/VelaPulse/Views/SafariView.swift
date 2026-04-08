import SafariServices
import SwiftUI

/// UIViewControllerRepresentable wrapper around SFSafariViewController.
/// Presents articles in-app without leaving Vela Pulse.
struct SafariView: UIViewControllerRepresentable {
    let url: URL

    func makeUIViewController(context: Context) -> SFSafariViewController {
        let config = SFSafariViewController.Configuration()
        config.entersReaderIfAvailable = false
        return SFSafariViewController(url: url, configuration: config)
    }

    func updateUIViewController(_ vc: SFSafariViewController, context: Context) {}
}
