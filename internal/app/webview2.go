// Package app coordinates the Estiraha Next Windows Desktop application lifecycle,
// WebView2 pre-flight verification, Wails v3 window & System Tray wiring, and clean shutdown.
package app

// ExitCodeMissingWebView2 is returned cleanly when Microsoft Edge WebView2 Runtime is absent.
const ExitCodeMissingWebView2 = 78

// WebView2DownloadURL is Microsoft's official Evergreen Bootstrapper link.
const WebView2DownloadURL = "https://go.microsoft.com/fwlink/p/?LinkId=2124703"

// WebView2Status captures the runtime detection state of Microsoft Edge WebView2.
type WebView2Status struct {
	Available     bool   `json:"available"`
	Version       string `json:"version"`
	Source        string `json:"source"`
	SimulatedMiss bool   `json:"simulatedMiss"`
	GuidanceAr    string `json:"guidanceAr"`
	GuidanceEn    string `json:"guidanceEn"`
	BootstrapURL  string `json:"bootstrapUrl"`
}

// BuildMissingWebView2Status constructs a deterministic bilingual diagnostic payload
// when WebView2 Runtime is missing or simulated as missing during testing.
func BuildMissingWebView2Status(simulated bool) WebView2Status {
	return WebView2Status{
		Available:     false,
		Version:       "",
		Source:        "missing",
		SimulatedMiss: simulated,
		GuidanceAr:    "مكوّن Microsoft Edge WebView2 غير مثبّت على هذا الجهاز. يرجى تثبيته لتشغيل واجهة Estiraha Next بأمان.",
		GuidanceEn:    "Microsoft Edge WebView2 Runtime is not installed on this Windows PC. Please install the Evergreen Runtime to launch Estiraha Next.",
		BootstrapURL:  WebView2DownloadURL,
	}
}
