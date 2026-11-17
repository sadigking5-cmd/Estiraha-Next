// Package fsops provides crash-safe, journaled filesystem operations gated by the Review Center.
// Phase 0 Foundation establishes strict pre-flight safety guards:
//   - Requires valid review.ExecutionPermit
//   - Never overwrites existing files
//   - Never deletes files automatically
package fsops

import (
	"errors"
	"os"

	"github.com/sadigking5-cmd/Estiraha-Next/internal/review"
)

var (
	ErrOverwriteForbidden = errors.New("fsops safety violation: target path already exists; overwrite is never permitted")
	ErrSourceMissing      = errors.New("fsops safety error: source file does not exist")
)

// VerifySafeDestination ensures permit validity, source existence, and zero collision at destination.
func VerifySafeDestination(permit review.ExecutionPermit, sourcePath, destPath string) error {
	if err := review.ValidatePermit(permit); err != nil {
		return err
	}
	if _, err := os.Stat(sourcePath); err != nil {
		if os.IsNotExist(err) {
			return ErrSourceMissing
		}
		return err
	}
	if _, err := os.Stat(destPath); err == nil {
		return ErrOverwriteForbidden
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}
