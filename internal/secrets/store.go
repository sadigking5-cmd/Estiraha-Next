// Package secrets abstracts OS-protected credential storage.
// On Windows deployments, API keys are stored exclusively in Windows Credential Manager
// (Advapi32 CredWriteW / CredReadW / CredDeleteW via github.com/danieljoos/wincred)
// and NEVER persisted in SQLite, JSON configs, logs, UI payloads, or Git repositories.
package secrets

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/sadigking5-cmd/Estiraha-Next/internal/domain"
)

const DiagnosticProbeTarget = "EstirahaNext/Phase1A/DiagnosticProbe"

var (
	ErrSecretNotFound     = errors.New("secrets: credential not found")
	ErrEmptyTargetOrValue = errors.New("secrets: target key and secret value must be non-empty")
)

// Vault defines secure secret storage operations.
type Vault interface {
	BackendName() string
	Set(targetKey string, secretValue string) error
	Get(targetKey string) (string, error)
	Delete(targetKey string) error
}

// ProbeReport summarizes a non-leaking write/read/verify/delete cycle on the Vault.
// Raw secret material is NEVER stored in this struct.
type ProbeReport struct {
	Backend            string `json:"backend"`
	TargetKey          string `json:"targetKey"`
	WriteSucceeded     bool   `json:"writeSucceeded"`
	ReadSucceeded      bool   `json:"readSucceeded"`
	HashMatched        bool   `json:"hashMatched"`
	DeleteSucceeded    bool   `json:"deleteSucceeded"`
	ConfirmedPurged    bool   `json:"confirmedPurged"`
	PayloadSHA256Short string `json:"payloadSha256Short"`
	TestedAtUTC        string `json:"testedAtUtc"`
}

// RunDiagnosticProbe executes a full lifecycle check on the underlying Vault
// using a random ephemeral token and immediately purges it. Zero secret values leak.
func RunDiagnosticProbe(v Vault) (ProbeReport, error) {
	ephemeralSecret := "estiraha-ephemeral-" + domain.NewImmutableID()
	expectedSum := sha256.Sum256([]byte(ephemeralSecret))
	shortHash := hex.EncodeToString(expectedSum[:])[:12]

	report := ProbeReport{
		Backend:            v.BackendName(),
		TargetKey:          DiagnosticProbeTarget,
		PayloadSHA256Short: shortHash,
		TestedAtUTC:        time.Now().UTC().Format(time.RFC3339),
	}

	if err := v.Set(DiagnosticProbeTarget, ephemeralSecret); err != nil {
		return report, err
	}
	report.WriteSucceeded = true

	// Ensure cleanup always runs even if intermediate verification fails
	defer func() {
		_ = v.Delete(DiagnosticProbeTarget)
	}()

	readBack, err := v.Get(DiagnosticProbeTarget)
	if err != nil {
		return report, err
	}
	report.ReadSucceeded = true

	actualSum := sha256.Sum256([]byte(readBack))
	report.HashMatched = (actualSum == expectedSum)
	if !report.HashMatched {
		return report, errors.New("secrets probe: SHA-256 mismatch on credential readback")
	}

	if err := v.Delete(DiagnosticProbeTarget); err != nil {
		return report, err
	}
	report.DeleteSucceeded = true

	_, errAfterDelete := v.Get(DiagnosticProbeTarget)
	report.ConfirmedPurged = errors.Is(errAfterDelete, ErrSecretNotFound)
	if !report.ConfirmedPurged {
		return report, errors.New("secrets probe: credential still readable after delete")
	}

	return report, nil
}
