package unit_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image/png"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sadigking5-cmd/Estiraha-Next/internal/api"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/app"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/config"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/db"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/secrets"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/tray"
)

type mockWindowTrayController struct {
	mu      sync.Mutex
	tracker *tray.StateTracker
	events  []string
	quitHit bool
}

func (m *mockWindowTrayController) HideToTray() {
	m.tracker.RecordHide()
}

func (m *mockWindowTrayController) RestoreFromTray() {
	m.tracker.RecordRestore()
}

func (m *mockWindowTrayController) EmitEvent(name string, _ any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, name)
}

func (m *mockWindowTrayController) RequestQuit() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.quitHit = true
	m.tracker.MarkDestroyed()
}

func (m *mockWindowTrayController) EmittedCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.events)
}

func TestTrayIconGeneratesValid64x64PNG(t *testing.T) {
	raw := tray.GenerateAppIconPNG()
	if len(raw) == 0 {
		t.Fatal("expected non-empty PNG icon bytes")
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("failed to decode generated tray PNG icon: %v", err)
	}
	bounds := img.Bounds()
	if bounds.Dx() != 64 || bounds.Dy() != 64 {
		t.Fatalf("expected 64x64 tray icon, got %dx%d", bounds.Dx(), bounds.Dy())
	}
}

func TestWebView2MissingFallbackStatus(t *testing.T) {
	st := app.DetectWebView2(true)
	if st.Available {
		t.Fatal("expected simulated missing WebView2 to report Available=false")
	}
	if !st.SimulatedMiss || st.GuidanceAr == "" || st.GuidanceEn == "" || st.BootstrapURL != app.WebView2DownloadURL {
		t.Fatalf("unexpected missing WebView2 payload: %+v", st)
	}
}

func TestPoCServiceBidirectionalBridgeRTLTrayAndZeroSecretLeak(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dbEngine, err := db.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("db open failed: %v", err)
	}
	defer dbEngine.Close()
	if _, err := dbEngine.Migrate(ctx, ""); err != nil {
		t.Fatalf("db migrate failed: %v", err)
	}

	cfg := config.Default()
	vault := secrets.NewDefaultVault()
	tracker := tray.NewStateTracker()
	tracker.MarkInitialized()
	ctrl := &mockWindowTrayController{tracker: tracker}

	svc := api.NewPoCService(cfg, dbEngine, vault, tracker, app.DetectWebView2(false), ctrl)

	// 1. Default Arabic RTL + Right Sidebar
	diagAr, err := svc.GetPlatformDiagnostics()
	if err != nil {
		t.Fatalf("GetPlatformDiagnostics failed: %v", err)
	}
	if diagAr.ActiveLocale != "ar" || diagAr.Direction != "rtl" || diagAr.SidebarPlacement != "right" {
		t.Fatalf("expected ar/rtl/right default, got %+v", diagAr)
	}

	// 2. Switch to English LTR + Left Sidebar & verify Go->React event emission
	diagEn, err := svc.SetActiveLocale("en")
	if err != nil {
		t.Fatalf("SetActiveLocale(en) failed: %v", err)
	}
	if diagEn.ActiveLocale != "en" || diagEn.Direction != "ltr" || diagEn.SidebarPlacement != "left" {
		t.Fatalf("expected en/ltr/left after switch, got %+v", diagEn)
	}
	if ctrl.EmittedCount() < 1 {
		t.Fatal("expected Go->React event on locale switch")
	}

	// 3. React -> Go RPC Ping + Go -> React push event
	pingResp := svc.PingGoFromReact(api.BridgePingRequest{
		Message: "أحمد زكي — الناظر 2000",
		Locale:  "ar",
	})
	if pingResp.NormalizedArabic != "احمد زكي — الناظر 2000" {
		t.Fatalf("unexpected Arabic normalization: %q", pingResp.NormalizedArabic)
	}
	if ctrl.EmittedCount() < 2 {
		t.Fatal("expected Go->React push event during PingGoFromReact")
	}

	// 4. System Tray Hide -> Restore -> Clean Quit
	afterHide := svc.HideWindowToTray()
	if afterHide.WindowVisible || afterHide.HideCount != 1 {
		t.Fatalf("unexpected tray state after hide: %+v", afterHide)
	}
	afterRestore := svc.RestoreWindowFromTray()
	if !afterRestore.WindowVisible || afterRestore.RestoreCount != 1 {
		t.Fatalf("unexpected tray state after restore: %+v", afterRestore)
	}

	// 5. Credential Manager diagnostic probe + strict zero-secret-leak check
	probe, err := svc.TestCredentialManager()
	if err != nil {
		t.Fatalf("TestCredentialManager failed: %v", err)
	}
	if !probe.WriteSucceeded || !probe.ReadSucceeded || !probe.HashMatched || !probe.DeleteSucceeded || !probe.ConfirmedPurged {
		t.Fatalf("incomplete credential probe lifecycle: %+v", probe)
	}
	serialized, _ := json.Marshal(probe)
	if strings.Contains(string(serialized), "estiraha-ephemeral-") {
		t.Fatalf("SECURITY VIOLATION: raw secret leaked in ProbeReport JSON: %s", string(serialized))
	}

	// 6. Clean Quit destroys tray state
	svc.RequestCleanQuit()
	if !ctrl.quitHit || !tracker.Snapshot().Destroyed {
		t.Fatal("expected clean quit to mark controller quit and destroy tray state")
	}
}
