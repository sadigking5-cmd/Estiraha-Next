//go:build !windows

package app

// DetectWebView2 provides deterministic status reporting on non-Windows CI/dev hosts.
func DetectWebView2(simulateMissing bool) WebView2Status {
	if simulateMissing {
		return BuildMissingWebView2Status(true)
	}
	return WebView2Status{
		Available:    false,
		Version:      "non-windows-host",
		Source:       "linux-ci-host",
		GuidanceAr:   "فحص WebView2 الأصلي يعمل على نظام Windows فقط.",
		GuidanceEn:   "Native WebView2 detection executes on Windows hosts.",
		BootstrapURL: WebView2DownloadURL,
	}
}

func NotifyMissingWebView2Interactive(_ WebView2Status) {}
