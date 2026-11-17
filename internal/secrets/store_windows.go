//go:build windows

package secrets

// WindowsCredManagerVault binds to Windows Credential Manager (Advapi32 Generic Credentials).
// Phase 0 registers the Windows target architecture; Win32 syscall binding is scheduled for Phase 1 PoC.
type WindowsCredManagerVault struct {
	memFallback map[string]string
}

func NewDefaultVault() Vault {
	return &WindowsCredManagerVault{memFallback: make(map[string]string)}
}

func (v *WindowsCredManagerVault) BackendName() string {
	return "windows-credential-manager"
}

func (v *WindowsCredManagerVault) Set(targetKey, secretValue string) error {
	if targetKey == "" || secretValue == "" {
		return ErrEmptyTargetOrValue
	}
	v.memFallback[targetKey] = secretValue
	return nil
}

func (v *WindowsCredManagerVault) Get(targetKey string) (string, error) {
	val, ok := v.memFallback[targetKey]
	if !ok {
		return "", ErrSecretNotFound
	}
	return val, nil
}

func (v *WindowsCredManagerVault) Delete(targetKey string) error {
	delete(v.memFallback, targetKey)
	return nil
}
