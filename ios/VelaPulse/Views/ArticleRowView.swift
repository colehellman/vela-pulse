import SwiftUI

struct ArticleRowView: View {
    let article: Article

    @State private var showSafari = false

    var body: some View {
        Button {
            guard URL(string: article.canonicalURL) != nil else { return }
            showSafari = true
        } label: {
            VStack(alignment: .leading, spacing: 6) {
                Text(article.title)
                    .font(.headline)
                    .lineLimit(2)
                    .foregroundStyle(.primary)

                HStack(spacing: 8) {
                    Text(article.sourceDomain)
                        .font(.caption)
                        .foregroundStyle(.secondary)

                    Spacer()

                    Text(article.publishedAt.formatted(.relative(presentation: .named)))
                        .font(.caption2)
                        .foregroundStyle(.tertiary)

                    // Pulse score badge — only shown when score is meaningful (> 1.0).
                    if article.pulseScore > 1.0 {
                        Text(String(format: "%.0f", article.pulseScore))
                            .font(.caption2.bold())
                            .padding(.horizontal, 6)
                            .padding(.vertical, 2)
                            .background(.tint.opacity(0.15))
                            .foregroundStyle(.tint)
                            .clipShape(Capsule())
                    }
                }
            }
            .padding(.vertical, 4)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .sheet(isPresented: $showSafari) {
            if let url = URL(string: article.canonicalURL) {
                SafariView(url: url)
                    .ignoresSafeArea()
            }
        }
    }
}
