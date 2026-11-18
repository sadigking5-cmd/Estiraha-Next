//go:build !windows

package main

import (
	"fmt"
	"os"

	"github.com/sadigking5-cmd/Estiraha-Next/internal/app"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/config"
)

func runDesktopApp(cfg config.Config, opts launchOptions) int {
	wv := app.DetectWebView2(opts.simulateMissingWebView2)
	if opts.simulateMissingWebView2 && !wv.Available {
		fmt.Fprintf(os.Stderr, "Simulated missing WebView2: %s\n", wv.GuidanceEn)
		return app.ExitCodeMissingWebView2
	}
	fmt.Printf("%s (%s) — Native Windows Desktop UI requires Windows x64 host. Use --self-check on Linux CI.\n", cfg.App.Name, cfg.App.Version)
	return 0
}
