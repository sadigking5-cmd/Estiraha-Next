//go:build windows

package secrets

import (
	"errors"

	"github.com/danieljoos/wincred"
)

// WindowsCredManagerVault stores secrets natively in Windows Credential Manager
// using Win32 Advapi32 Generic Credentials encrypted per-user by Windows DPAPI.
type WindowsCredManagerVault struct{}

func NewDefaultVault() Vault {
	return &WindowsCredManagerVault{}
}

func (v *WindowsCredManagerVault) BackendName() string {
	return "windows-credential-manager (Advapi32/DPAPI)"
}

func (v *WindowsCredManagerVault) Set(targetKey, secretValue string) error {
	if targetKey == "" || secretValue == "" {
		return ErrEmptyTargetOrValue
	}
	cred := wincred.NewGenericCredential(targetKey)
	cred.UserName = "EstirahaNextLocalUser"
	cred.CredentialBlob = []byte(secretValue)
	cred.Persist = wincred.PersistLocalMachine
	cred.Comment = "Managed locally by Estiraha Next Desktop — Never synced to cloud"
	return cred.Write()
}

func (v *WindowsCredManagerVault) Get(targetKey string) (string, error) {
	if targetKey == "" {
		return "", ErrEmptyTargetOrValue
	}
	cred, err := wincred.GetGenericCredential(targetKey)
	if err != nil {
		if errors.Is(err, wincred.ErrElementNotFound) {
			return "", ErrSecretNotFound
		}
		return "", err
	}
	return string(cred.CredentialBlob), nil
}

func (v *WindowsCredManagerVault) Delete(targetKey string) error {
	if targetKey == "" {
		return ErrEmptyTargetOrValue
	}
	cred, err := wincred.GetGenericCredential(targetKey)
	if err != nil {
		if errors.Is(err, wincred.ErrElementNotFound) {
			return nil
		}
		return err
	}
	return cred.Delete()
}
