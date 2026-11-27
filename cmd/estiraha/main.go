// Command estiraha is the official entry point of Estiraha Next (Windows Desktop Media Library System).
// Phase 1A launches the real Wails v3 Windows Desktop PoC window + System Tray + WebView2 pre-check,
// while preserving --self-check for headless CI diagnostics.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/sadigking5-cmd/Estiraha-Next/frontend"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/api"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/app"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/config"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/db"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/secrets"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/tray"
)

type launchOptions struct {
	simulateMissingWebView2 bool
	headlessCheck           bool
	smokeReportPath         string
	holdMs                  int
}

func main() {
	selfCheck := flag.Bool("self-check", false, "Run non-destructive engine & PoC diagnostics in memory and exit")
	configPath := flag.String("config", "", "Optional path to JSON configuration file")
	simMissingWV2 := flag.Bool("simulate-missing-webview2", os.Getenv("ESTIRAHA_SIMULATE_MISSING_WEBVIEW2") == "1", "Simulate missing WebView2 runtime for verification testing")
	headless := flag.Bool("headless", false, "Suppress interactive MessageBox dialogs during automated tests")
	smokeReport := flag.String("poc-smoke-report", "", "Run automated Windows Desktop PoC smoke cycle, write JSON report, and quit cleanly")
	holdMs := flag.Int("poc-hold-ms", 4500, "Milliseconds to hold native window open during --poc-smoke-report for screenshot capture")
	uiPreviewAddr := flag.String("ui-preview-addr", "", "Optional local HTTP bind address (e.g. 127.0.0.1:18080) for headless CI UI verification")
	uiPreviewDir := flag.String("ui-preview-data-dir", "", "Optional isolated temp directory for --ui-preview-addr SQLite DB & backups")
	flag.Parse()

	cfg := config.Default()
	if *configPath != "" {
		loaded, err := config.Load(*configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
			os.Exit(1)
		}
		cfg = loaded
	} else if err := cfg.ValidateSafetyInvariants(); err != nil {
		fmt.Fprintf(os.Stderr, "safety invariant error: %v\n", err)
		os.Exit(1)
	}

	if *selfCheck {
		runSelfCheck(cfg, *simMissingWV2)
		return
	}

	if *uiPreviewAddr != "" {
		if err := runUIPreviewServer(cfg, *uiPreviewAddr, *uiPreviewDir); err != nil {
			fmt.Fprintf(os.Stderr, "ui-preview error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	code := runDesktopApp(cfg, launchOptions{
		simulateMissingWebView2: *simMissingWV2,
		headlessCheck:           *headless,
		smokeReportPath:         *smokeReport,
		holdMs:                  *holdMs,
	})
	os.Exit(code)
}

func runSelfCheck(cfg config.Config, simMissingWV2 bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	engine, err := db.Open(ctx, ":memory:")
	if err != nil {
		fmt.Fprintf(os.Stderr, "db open failed: %v\n", err)
		os.Exit(1)
	}
	defer engine.Close()

	ver, err := engine.Migrate(ctx, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "db migration failed: %v\n", err)
		os.Exit(1)
	}

	health, err := engine.HealthCheck(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "db health check failed: %v\n", err)
		os.Exit(1)
	}

	vault := secrets.NewDefaultVault()
	credProbe, err := secrets.RunDiagnosticProbe(vault)
	if err != nil {
		fmt.Fprintf(os.Stderr, "credential probe failed: %v\n", err)
		os.Exit(1)
	}

	wvStatus := app.DetectWebView2(simMissingWV2)
	iconBytes := tray.GenerateAppIconPNG()

	summary := map[string]any{
		"app":                cfg.App.Name,
		"version":            cfg.App.Version,
		"phase":              "Phase B — Full Media Library Management System",
		"go_version":         runtime.Version(),
		"wails_version":      api.WailsRuntimeVersion,
		"os":                 runtime.GOOS,
		"arch":               runtime.GOARCH,
		"schema_version":     ver,
		"registered_tables":  health.RegisteredTables,
		"foreign_keys_on":    health.ForeignKeys,
		"fts5_enabled":       health.FTS5Enabled,
		"secrets_backend":    vault.BackendName(),
		"credential_probe":   credProbe,
		"webview2_status":    wvStatus,
		"tray_icon_png_size": len(iconBytes),
		"safety_lock":        "ACTIVE (Zero unapproved file operations)",
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(summary)
}

func runUIPreviewServer(cfg config.Config, addr, dataDir string) error {
	ctx := context.Background()
	workDir := dataDir
	if workDir == "" {
		tmp, err := os.MkdirTemp("", "estiraha-ui-preview-*")
		if err != nil {
			return err
		}
		workDir = tmp
	} else if err := os.MkdirAll(workDir, 0o755); err != nil {
		return err
	}

	dbPath := filepath.Join(workDir, "library.sqlite")
	backupDir := filepath.Join(workDir, "Backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return err
	}

	engine, err := db.Open(ctx, dbPath)
	if err != nil {
		return err
	}
	defer engine.Close()

	if _, err := engine.Migrate(ctx, backupDir); err != nil {
		return err
	}

	distFS, err := frontend.DistFS()
	if err != nil {
		return err
	}

	vault := secrets.NewDefaultVault()
	libSvc := api.NewLibraryService(cfg, engine, vault, backupDir, nil)
	defer libSvc.Close()

	trayTracker := tray.NewStateTracker()
	pocSvc := api.NewPoCService(cfg, engine, vault, trayTracker, app.DetectWebView2(false), nil)

	handler := api.NewUIPreviewHandler(distFS, libSvc, pocSvc)
	fmt.Printf("Estiraha Next UI Preview listening on http://%s (dataDir=%s)\n", addr, workDir)
	return http.ListenAndServe(addr, handler)
}
