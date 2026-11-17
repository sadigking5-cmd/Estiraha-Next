// Package secrets abstracts OS-protected credential storage.
// On Windows production deployments, API keys are stored exclusively in
// Windows Credential Manager (Advapi32 CredWriteW / CredReadW / CredDeleteW)
// and NEVER persisted in SQLite, JSON configs, or Git repositories.
package secrets

import "errors"

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
