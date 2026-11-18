//go:build windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/sadigking5-cmd/Estiraha-Next/frontend"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/api"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/app"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/config"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/db"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/secrets"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/tray"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

type wailsDesktopController struct {
	wailsApp    *application.App
	mainWin     *application.WebviewWindow
	sysTray     *application.SystemTray
	trayTracker *tray.StateTracker
	quitOnce    sync.Once
}

func (c *wailsDesktopController) HideToTray() {
	if c.mainWin != nil {
		c.mainWin.Hide()
	}
	if c.trayTracker != nil {
		c.trayTracker.RecordHide()
	}
}

func (c *wailsDesktopController) RestoreFromTray() {
	if c.mainWin != nil {
		c.mainWin.Show()
		c.mainWin.Focus()
	}
	if c.trayTracker != nil {
		c.trayTracker.RecordRestore()
	}
}

func (c *wailsDesktopController) EmitEvent(name string, payload any) {
	if c.wailsApp != nil {
		c.wailsApp.Event.Emit(name, payload)
	}
}

func (c *wailsDesktopController) RequestQuit() {
	c.quitOnce.Do(func() {
		if c.sysTray != nil {
			c.sysTray.Destroy()
		}
		if c.trayTracker != nil {
			c.trayTracker.MarkDestroyed()
		}
		if c.wailsApp != nil {
			c.wailsApp.Quit()
		}
	})
}

type pocSmokeReport struct {
	Success               bool                  `json:"success"`
	TimestampUTC          string                `json:"timestampUtc"`
	Diagnostics           api.PlatformDiagnostics `json:"diagnostics"`
	BridgePing            api.BridgePingResponse `json:"bridgePing"`
	CredentialManagerTest secrets.ProbeReport   `json:"credentialManagerTest"`
	TrayAfterCycle        tray.Snapshot         `json:"trayAfterCycle"`
}

func runDesktopApp(cfg config.Config, opts launchOptions) int {
	// 1. Pre-flight check for Microsoft Edge WebView2 Runtime
	wvStatus := app.DetectWebView2(opts.simulateMissingWebView2)
	if !wvStatus.Available {
		if opts.smokeReportPath != "" {
			raw, _ := json.MarshalIndent(wvStatus, "", "  ")
			_ = os.WriteFile(opts.smokeReportPath, raw, 0o600)
		} else if !opts.headlessCheck {
			app.NotifyMissingWebView2Interactive(wvStatus)
		}
		fmt.Fprintf(os.Stderr, "WebView2 unavailable: %s\n", wvStatus.GuidanceEn)
		return app.ExitCodeMissingWebView2
	}

	// 2. Initialize isolated in-memory Phase 0 SQLite schema for PoC diagnostics
	// (Strictly obeys Phase 1A constraint: zero new migrations, zero user disk touches)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	dbEngine, err := db.Open(ctx, ":memory:")
	if err != nil {
		fmt.Fprintf(os.Stderr, "db open failed: %v\n", err)
		return 1
	}
	defer dbEngine.Close()

	if _, err := dbEngine.Migrate(ctx, ""); err != nil {
		fmt.Fprintf(os.Stderr, "db migrate failed: %v\n", err)
		return 1
	}

	vault := secrets.NewDefaultVault()
	trayTracker := tray.NewStateTracker()
	ctrl := &wailsDesktopController{trayTracker: trayTracker}
	pocSvc := api.NewPoCService(cfg, dbEngine, vault, trayTracker, wvStatus, ctrl)

	assetsFS, err := frontend.DistFS()
	if err != nil {
		fmt.Fprintf(os.Stderr, "embedded frontend dist missing: %v\n", err)
		return 1
	}

	iconBytes := tray.GenerateAppIconPNG()

	wailsApp := application.New(application.Options{
		Name:        "Estiraha Next",
		Description: "Estiraha Next — Windows Desktop Media Library System (Phase 1A PoC)",
		Icon:        iconBytes,
		Services: []application.Service{
			application.NewService(pocSvc),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assetsFS),
		},
		Windows: application.WindowsOptions{
			DisableQuitOnLastWindowClosed: true,
		},
		OnShutdown: func() {
			if ctrl.sysTray != nil {
				ctrl.sysTray.Destroy()
			}
			trayTracker.MarkDestroyed()
			_ = dbEngine.Close()
		},
	})
	ctrl.wailsApp = wailsApp

	// Create real Windows WebView2 desktop window
	mainWin := wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "estiraha-main",
		Title:            "Estiraha Next — استراحة نكست (Phase 1A Desktop PoC)",
		Width:            1240,
		Height:           820,
		MinWidth:         960,
		MinHeight:        640,
		BackgroundColour: application.NewRGBA(11, 17, 32, 255),
		URL:              "/",
	})
	ctrl.mainWin = mainWin

	// Intercept window close button (X) to hide cleanly into System Tray instead of orphaning state
	mainWin.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		ctrl.HideToTray()
		e.Cancel()
	})

	// Configure native Windows System Tray & bilingual context menu
	sysTray := wailsApp.SystemTray.New()
	sysTray.SetIcon(iconBytes)
	sysTray.SetTooltip("Estiraha Next — استراحة نكست")

	trayMenu := wailsApp.NewMenu()
	trayMenu.Add("إظهار النافذة / Restore Window").OnClick(func(_ *application.Context) {
		ctrl.RestoreFromTray()
	})
	trayMenu.Add("إخفاء إلى الشريط / Hide to Tray").OnClick(func(_ *application.Context) {
		ctrl.HideToTray()
	})
	trayMenu.AddSeparator()
	trayMenu.Add("خروج آمن / Quit Cleanly").OnClick(func(_ *application.Context) {
		ctrl.RequestQuit()
	})
	sysTray.SetMenu(trayMenu)
	sysTray.OnClick(func() {
		snap := trayTracker.Snapshot()
		if snap.WindowVisible {
			ctrl.HideToTray()
		} else {
			ctrl.RestoreFromTray()
		}
	})
	trayTracker.MarkInitialized()
	ctrl.sysTray = sysTray

	// Listen for client-emitted events (React -> Go Event Bus verification)
	wailsApp.Event.On("estiraha:react-event", func(ev *application.CustomEvent) {
		wailsApp.Event.Emit("estiraha:go-event-ack", map[string]any{
			"receivedEvent": ev.Name,
			"ackFromGoPid":  os.Getpid(),
			"timestampUtc":  time.Now().UTC().Format(time.RFC3339Nano),
		})
	})

	// Optional automated Windows Runner smoke-test orchestration
	if opts.smokeReportPath != "" {
		go func() {
			hold := time.Duration(opts.holdMs) * time.Millisecond
			if hold < 2000*time.Millisecond {
				hold = 2000 * time.Millisecond
			}
			// Wait for native HWND + WebView2 surface render so external screenshot captures live UI
			time.Sleep(hold)

			ping := pocSvc.PingGoFromReact(api.BridgePingRequest{
				Message: "اختبار استراحة نكست أحمد زكي",
				Locale:  "ar",
			})
			credReport, credErr := pocSvc.TestCredentialManager()

			// Exercise real System Tray hide & restore on the live Windows HWND
			pocSvc.HideWindowToTray()
			time.Sleep(400 * time.Millisecond)
			traySnap := pocSvc.RestoreWindowFromTray()
			time.Sleep(500 * time.Millisecond)

			diag, diagErr := pocSvc.GetPlatformDiagnostics()
			report := pocSmokeReport{
				Success:               credErr == nil && diagErr == nil && credReport.ConfirmedPurged && traySnap.HideCount >= 1 && traySnap.RestoreCount >= 1,
				TimestampUTC:          time.Now().UTC().Format(time.RFC3339),
				Diagnostics:           diag,
				BridgePing:            ping,
				CredentialManagerTest: credReport,
				TrayAfterCycle:        traySnap,
			}
			raw, _ := json.MarshalIndent(report, "", "  ")
			_ = os.WriteFile(opts.smokeReportPath, raw, 0o600)

			ctrl.RequestQuit()
		}()
	}

	if err := wailsApp.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "wails run error: %v\n", err)
		return 1
	}
	return 0
}
