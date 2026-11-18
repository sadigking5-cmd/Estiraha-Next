// Package api exposes typed Wails v3 service bindings and bidirectional event handlers
// between the Go core engine and the React/TypeScript desktop UI.
package api

import (
	"context"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/sadigking5-cmd/Estiraha-Next/internal/app"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/config"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/db"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/search"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/secrets"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/tray"
)

const WailsRuntimeVersion = "v3.1.0"

// WindowTrayController abstracts native window & System Tray actions so PoCService
// remains unit-testable on headless runners while controlling real Wails windows on Windows.
type WindowTrayController interface {
	HideToTray()
	RestoreFromTray()
	EmitEvent(name string, payload any)
	RequestQuit()
}

// PlatformDiagnostics summarizes verified runtime state for the Phase 1A PoC dashboard.
type PlatformDiagnostics struct {
	AppName          string            `json:"appName"`
	AppVersion       string            `json:"appVersion"`
	GoVersion        string            `json:"goVersion"`
	WailsVersion     string            `json:"wailsVersion"`
	OS               string            `json:"os"`
	Arch             string            `json:"arch"`
	ProcessID        int               `json:"processId"`
	ActiveLocale     string            `json:"activeLocale"`
	Direction        string            `json:"direction"`
	SidebarPlacement string            `json:"sidebarPlacement"`
	SchemaVersion    int               `json:"schemaVersion"`
	ForeignKeysOn    bool              `json:"foreignKeysOn"`
	FTS5Enabled      bool              `json:"fts5Enabled"`
	SecretsBackend   string            `json:"secretsBackend"`
	SafetyLockActive bool              `json:"safetyLockActive"`
	WebView2         app.WebView2Status `json:"webView2"`
	Tray             tray.Snapshot     `json:"tray"`
}

// BridgePingRequest is sent from React -> Go over typed Wails v3 RPC.
type BridgePingRequest struct {
	Message string `json:"message"`
	Locale  string `json:"locale"`
}

// BridgePingResponse is returned from Go -> React (and paired with an async Go -> React event).
type BridgePingResponse struct {
	OriginalMessage   string `json:"originalMessage"`
	NormalizedArabic  string `json:"normalizedArabic"`
	ReplyAr           string `json:"replyAr"`
	ReplyEn           string `json:"replyEn"`
	HandledByGoPID    int    `json:"handledByGoPid"`
	RoundtripSequence int    `json:"roundtripSequence"`
	TimestampUTC      string `json:"timestampUtc"`
}

// BackendEventPayload is emitted asynchronously from Go -> React over Wails EventManager.
type BackendEventPayload struct {
	EventName string `json:"eventName"`
	MessageAr string `json:"messageAr"`
	MessageEn string `json:"messageEn"`
	Sequence  int    `json:"sequence"`
	EmittedAt string `json:"emittedAt"`
}

// PoCService implements Phase 1A desktop proof-of-concept bindings for Wails v3.
type PoCService struct {
	mu           sync.Mutex
	cfg          config.Config
	dbEngine     *db.Engine
	vault        secrets.Vault
	trayTracker  *tray.StateTracker
	controller   WindowTrayController
	webView2     app.WebView2Status
	activeLocale string
	pingCounter  int
}

func NewPoCService(
	cfg config.Config,
	dbEngine *db.Engine,
	vault secrets.Vault,
	trayTracker *tray.StateTracker,
	webView2 app.WebView2Status,
	ctrl WindowTrayController,
) *PoCService {
	locale := cfg.App.DefaultLanguage
	if locale != "ar" && locale != "en" {
		locale = "ar"
	}
	return &PoCService{
		cfg:          cfg,
		dbEngine:     dbEngine,
		vault:        vault,
		trayTracker:  trayTracker,
		controller:   ctrl,
		webView2:     webView2,
		activeLocale: locale,
	}
}

// GetPlatformDiagnostics returns real runtime metrics from Go, SQLite, WebView2, and Tray.
func (s *PoCService) GetPlatformDiagnostics() (PlatformDiagnostics, error) {
	s.mu.Lock()
	locale := s.activeLocale
	s.mu.Unlock()

	dir := "rtl"
	sidebar := "right"
	if locale == "en" {
		dir = "ltr"
		sidebar = "left"
	}

	var health db.HealthReport
	if s.dbEngine != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		h, err := s.dbEngine.HealthCheck(ctx)
		if err != nil {
			return PlatformDiagnostics{}, err
		}
		health = h
	}

	var traySnap tray.Snapshot
	if s.trayTracker != nil {
		traySnap = s.trayTracker.Snapshot()
	}

	backendName := "unavailable"
	if s.vault != nil {
		backendName = s.vault.BackendName()
	}

	return PlatformDiagnostics{
		AppName:          s.cfg.App.Name,
		AppVersion:       s.cfg.App.Version,
		GoVersion:        runtime.Version(),
		WailsVersion:     WailsRuntimeVersion,
		OS:               runtime.GOOS,
		Arch:             runtime.GOARCH,
		ProcessID:        os.Getpid(),
		ActiveLocale:     locale,
		Direction:        dir,
		SidebarPlacement: sidebar,
		SchemaVersion:    health.SchemaVersion,
		ForeignKeysOn:    health.ForeignKeys,
		FTS5Enabled:      health.FTS5Enabled,
		SecretsBackend:   backendName,
		SafetyLockActive: s.cfg.ValidateSafetyInvariants() == nil,
		WebView2:         s.webView2,
		Tray:             traySnap,
	}, nil
}

// SetActiveLocale switches active locale ('ar' -> RTL/Right Sidebar, 'en' -> LTR/Left Sidebar)
// and emits a live Go -> React event confirming directional synchronization.
func (s *PoCService) SetActiveLocale(locale string) (PlatformDiagnostics, error) {
	if locale != "ar" && locale != "en" {
		locale = "ar"
	}
	s.mu.Lock()
	s.activeLocale = locale
	ctrl := s.controller
	s.pingCounter++
	seq := s.pingCounter
	s.mu.Unlock()

	if ctrl != nil {
		ctrl.EmitEvent("estiraha:locale-changed", BackendEventPayload{
			EventName: "estiraha:locale-changed",
			MessageAr: "تمت مزامنة اللغة والاتجاه من محرك Go بنجاح",
			MessageEn: "Locale and layout direction synchronized by Go engine",
			Sequence:  seq,
			EmittedAt: time.Now().UTC().Format(time.RFC3339Nano),
		})
	}
	return s.GetPlatformDiagnostics()
}

// PingGoFromReact tests React -> Go RPC invocation AND triggers an immediate Go -> React event.
func (s *PoCService) PingGoFromReact(req BridgePingRequest) BridgePingResponse {
	s.mu.Lock()
	s.pingCounter++
	seq := s.pingCounter
	ctrl := s.controller
	s.mu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	norm := search.NormalizeQuery(req.Message)

	resp := BridgePingResponse{
		OriginalMessage:   req.Message,
		NormalizedArabic:  norm,
		ReplyAr:           "تم استلام الرسالة في محرك Go ومعالجتها بنجاح: " + req.Message,
		ReplyEn:           "Message received and processed by Go engine: " + req.Message,
		HandledByGoPID:    os.Getpid(),
		RoundtripSequence: seq,
		TimestampUTC:      now,
	}

	if ctrl != nil {
		ctrl.EmitEvent("estiraha:go-push", BackendEventPayload{
			EventName: "estiraha:go-push",
			MessageAr: "بث عكسي حي من Go إلى React (تسلسل #" + itoa(seq) + ")",
			MessageEn: "Live push event from Go to React (seq #" + itoa(seq) + ")",
			Sequence:  seq,
			EmittedAt: now,
		})
	}
	return resp
}

// TriggerGoEvent emits a standalone asynchronous push event from Go -> React.
func (s *PoCService) TriggerGoEvent() BackendEventPayload {
	s.mu.Lock()
	s.pingCounter++
	seq := s.pingCounter
	ctrl := s.controller
	s.mu.Unlock()

	ev := BackendEventPayload{
		EventName: "estiraha:go-push",
		MessageAr: "إشعار فوري مرسل من الخلفية (Go) إلى الواجهة (React)",
		MessageEn: "Instant asynchronous notification pushed from Go to React",
		Sequence:  seq,
		EmittedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if ctrl != nil {
		ctrl.EmitEvent(ev.EventName, ev)
	}
	return ev
}

// TestCredentialManager executes a safe write/read/SHA256-verify/delete cycle on Windows Credential Manager.
func (s *PoCService) TestCredentialManager() (secrets.ProbeReport, error) {
	return secrets.RunDiagnosticProbe(s.vault)
}

// HideWindowToTray hides the desktop window to the Windows System Tray.
func (s *PoCService) HideWindowToTray() tray.Snapshot {
	s.mu.Lock()
	ctrl := s.controller
	s.mu.Unlock()
	if ctrl != nil {
		ctrl.HideToTray()
	} else if s.trayTracker != nil {
		s.trayTracker.RecordHide()
	}
	if s.trayTracker != nil {
		return s.trayTracker.Snapshot()
	}
	return tray.Snapshot{}
}

// RestoreWindowFromTray restores the desktop window from the Windows System Tray.
func (s *PoCService) RestoreWindowFromTray() tray.Snapshot {
	s.mu.Lock()
	ctrl := s.controller
	s.mu.Unlock()
	if ctrl != nil {
		ctrl.RestoreFromTray()
	} else if s.trayTracker != nil {
		s.trayTracker.RecordRestore()
	}
	if s.trayTracker != nil {
		return s.trayTracker.Snapshot()
	}
	return tray.Snapshot{}
}

// RunTrayCycle hides the window to System Tray and automatically restores it after delayMs
// so the user (or automated Windows runner) can visually verify both transitions.
func (s *PoCService) RunTrayCycle(delayMs int) tray.Snapshot {
	if delayMs < 250 {
		delayMs = 250
	}
	if delayMs > 5000 {
		delayMs = 5000
	}
	s.HideWindowToTray()
	go func() {
		time.Sleep(time.Duration(delayMs) * time.Millisecond)
		s.RestoreWindowFromTray()
	}()
	if s.trayTracker != nil {
		return s.trayTracker.Snapshot()
	}
	return tray.Snapshot{}
}

// RequestCleanQuit performs an orderly application exit with zero orphan processes.
func (s *PoCService) RequestCleanQuit() {
	s.mu.Lock()
	ctrl := s.controller
	s.mu.Unlock()
	if ctrl != nil {
		ctrl.RequestQuit()
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits [20]byte
	i := len(digits)
	for n > 0 {
		i--
		digits[i] = byte('0' + (n % 10))
		n /= 10
	}
	return string(digits[i:])
}
