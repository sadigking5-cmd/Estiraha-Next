//go:build windows

package app

import (
	"os/exec"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

const webView2ClientGUID = `{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`

// DetectWebView2 checks official Windows Registry keys (HKLM WOW6432Node, HKLM Native, HKCU)
// for a valid installed Microsoft Edge WebView2 Evergreen Runtime version (`pv` != "" && != "0.0.0.0").
func DetectWebView2(simulateMissing bool) WebView2Status {
	if simulateMissing {
		return BuildMissingWebView2Status(true)
	}

	candidates := []struct {
		root   registry.Key
		path   string
		source string
	}{
		{
			root:   registry.LOCAL_MACHINE,
			path:   `SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\` + webView2ClientGUID,
			source: "HKLM\\WOW6432Node",
		},
		{
			root:   registry.LOCAL_MACHINE,
			path:   `SOFTWARE\Microsoft\EdgeUpdate\Clients\` + webView2ClientGUID,
			source: "HKLM\\Software",
		},
		{
			root:   registry.CURRENT_USER,
			path:   `Software\Microsoft\EdgeUpdate\Clients\` + webView2ClientGUID,
			source: "HKCU\\Software",
		},
	}

	for _, c := range candidates {
		k, err := registry.OpenKey(c.root, c.path, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		ver, _, valErr := k.GetStringValue("pv")
		_ = k.Close()
		ver = strings.TrimSpace(ver)
		if valErr == nil && ver != "" && ver != "0.0.0.0" {
			return WebView2Status{
				Available:    true,
				Version:      ver,
				Source:       c.source,
				BootstrapURL: WebView2DownloadURL,
			}
		}
	}

	return BuildMissingWebView2Status(false)
}

// NotifyMissingWebView2Interactive displays a native Win32 bilingual MessageBoxW dialog
// offering to open the official Microsoft WebView2 Evergreen download page before exiting cleanly.
func NotifyMissingWebView2Interactive(status WebView2Status) {
	user32 := syscall.NewLazyDLL("user32.dll")
	messageBoxW := user32.NewProc("MessageBoxW")

	titlePtr, _ := syscall.UTF16PtrFromString("Estiraha Next — مطلوب WebView2 / WebView2 Required")
	msgText := status.GuidanceAr + "\r\n\r\n" +
		status.GuidanceEn + "\r\n\r\n" +
		"هل ترغب في فتح صفحة التنزيل الرسمية من Microsoft الآن؟\r\n" +
		"Open the official Microsoft WebView2 download page now?"
	msgPtr, _ := syscall.UTF16PtrFromString(msgText)

	// MB_YESNO (0x00000004) | MB_ICONWARNING (0x00000030) = 0x34; IDYES = 6
	const mbYesNoWarning = 0x00000034
	const idYes = 6
	ret, _, _ := messageBoxW.Call(
		0,
		uintptr(unsafe.Pointer(msgPtr)),
		uintptr(unsafe.Pointer(titlePtr)),
		mbYesNoWarning,
	)
	if ret == idYes {
		_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", WebView2DownloadURL).Start()
	}
}
