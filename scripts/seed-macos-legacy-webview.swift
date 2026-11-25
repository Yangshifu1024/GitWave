import AppKit
import WebKit

// Use the same origin and persistent data store as the installed Tauri app.
// The harness runs this helper under the fixture bundle ID and home directory.
@MainActor
final class PreferenceFixture: NSObject, WKURLSchemeHandler, WKNavigationDelegate {
    let settings: String
    var webView: WKWebView?

    init(settings: String) {
        self.settings = settings
    }

    func start() {
        let config = WKWebViewConfiguration()
        config.websiteDataStore = .default()
        config.setURLSchemeHandler(self, forURLScheme: "tauri")
        let view = WKWebView(frame: .zero, configuration: config)
        webView = view
        view.navigationDelegate = self
        view.load(URLRequest(url: URL(string: "tauri://localhost/")!))
    }

    func webView(_ webView: WKWebView, start task: any WKURLSchemeTask) {
        guard let url = task.request.url else {
            task.didFailWithError(URLError(.badURL))
            return
        }
        let page = Data("<!doctype html><meta charset=utf-8><title>Preference fixture</title>".utf8)
        task.didReceive(URLResponse(url: url, mimeType: "text/html", expectedContentLength: page.count, textEncodingName: "utf-8"))
        task.didReceive(page)
        task.didFinish()
    }

    func webView(_ webView: WKWebView, stop task: any WKURLSchemeTask) {}

    func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
        webView.evaluateJavaScript("""
            (() => {
              const entries = Object.entries(\(settings));
              for (const [key, value] of entries) localStorage.setItem(key, value);
              return entries.every(([key, value]) => localStorage.getItem(key) === value);
            })()
            """) { result, error in
            guard error == nil, result as? Bool == true else {
                fputs("Could not write the WebView preference fixture: \(String(describing: error))\n", stderr)
                exit(1)
            }
            // WebKit batches localStorage writes on the data-store process.
            DispatchQueue.main.asyncAfter(deadline: .now() + 2) {
                webView.configuration.websiteDataStore.fetchDataRecords(ofTypes: [WKWebsiteDataTypeLocalStorage]) { _ in
                    exit(0)
                }
            }
        }
    }
}

@main
enum SeedPreferences {
    @MainActor
    static func main() throws {
        guard CommandLine.arguments.count == 2 else {
            fputs("Pass the WebView preference fixture JSON path\n", stderr)
            exit(1)
        }
        let input = try Data(contentsOf: URL(fileURLWithPath: CommandLine.arguments[1]))
        var preferences = try JSONDecoder().decode([String: String].self, from: input)
        preferences["fixture-not-a-preference"] = "must not be exported"
        let settings = try JSONEncoder().encode(preferences)
        let app = NSApplication.shared
        app.setActivationPolicy(.prohibited)
        let fixture = PreferenceFixture(settings: String(decoding: settings, as: UTF8.self))
        DispatchQueue.main.async { fixture.start() }
        DispatchQueue.main.asyncAfter(deadline: .now() + 20) {
            fputs("Timed out writing the WebView preference fixture\n", stderr)
            exit(1)
        }
        withExtendedLifetime(fixture) { app.run() }
    }
}
