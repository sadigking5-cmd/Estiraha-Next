//go:build !windows

package secrets

import "sync"

// EphemeralMemoryVault provides an isolated, non-persistent in-memory vault
// for Linux CI/sandbox testing where Windows Credential Manager is unavailable.
type EphemeralMemoryVault struct {
	mu    sync.RWMutex
	store map[string]string
}

func NewDefaultVault() Vault {
	return &EphemeralMemoryVault{store: make(map[string]string)}
}

func (v *EphemeralMemoryVault) BackendName() string {
	return "ephemeral-ci-memory-vault"
}

func (v *EphemeralMemoryVault) Set(targetKey, secretValue string) error {
	if targetKey == "" || secretValue == "" {
		return ErrEmptyTargetOrValue
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	v.store[targetKey] = secretValue
	return nil
}

func (v *EphemeralMemoryVault) Get(targetKey string) (string, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	val, ok := v.store[targetKey]
	if !ok {
		return "", ErrSecretNotFound
	}
	return val, nil
}

func (v *EphemeralMemoryVault) Delete(targetKey string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	delete(v.store, targetKey)
	return nil
}
