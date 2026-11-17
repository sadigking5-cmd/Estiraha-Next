// Package imagecache implements the Smart Image Engine contracts:
//   - Multi-tier caches (Poster, Backdrop, Celebrity, Series Artwork, Thumbnail)
//   - Memory + Disk LRU eviction (2 GiB initial customizable budget)
//   - Strict MIME, byte-size, and pixel-dimension validation (SVG strictly blocked)
//   - Absolute protection of user-owned local images (never deleted or overwritten)
//   - Zero temporary/cache pollution inside user media library directories
package imagecache

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
)

const DefaultDiskBudgetBytes int64 = 2 * 1024 * 1024 * 1024 // 2 GiB

var (
	ErrForbiddenSVG            = errors.New("image engine security: SVG images are strictly prohibited")
	ErrUnsupportedImageFormat  = errors.New("image engine security: only verified JPEG, PNG, and WebP images are accepted")
	ErrImageExceedsMaxBytes    = errors.New("image engine validation: image size exceeds configured maximum")
	ErrInvalidImageDimensions  = errors.New("image engine validation: image dimensions are invalid or exceed bounds")
	ErrProtectedUserLocalAsset = errors.New("image engine safety: user-owned local artwork cannot be deleted or replaced")
	ErrCacheInsideUserLibrary  = errors.New("image engine safety: cache/temp path must never reside inside user media library roots")
)

// CandidateImage describes an incoming image header & metadata prior to acceptance.
type CandidateImage struct {
	DeclaredMIME string
	HeaderBytes  []byte
	ByteSize     int64
	Width        int
	Height       int
}

// ValidateImage enforces format, magic-byte signature, size, and dimension policies.
func ValidateImage(img CandidateImage, maxBytes int64, maxDim int) error {
	mimeLower := strings.ToLower(strings.TrimSpace(img.DeclaredMIME))
	if strings.Contains(mimeLower, "svg") || bytes.Contains(bytes.ToLower(img.HeaderBytes), []byte("<svg")) {
		return ErrForbiddenSVG
	}
	if img.ByteSize <= 0 || img.ByteSize > maxBytes {
		return ErrImageExceedsMaxBytes
	}
	if img.Width <= 0 || img.Height <= 0 || img.Width > maxDim || img.Height > maxDim {
		return ErrInvalidImageDimensions
	}
	if !hasValidMagicSignature(img.HeaderBytes, mimeLower) {
		return ErrUnsupportedImageFormat
	}
	return nil
}

func hasValidMagicSignature(header []byte, mimeLower string) bool {
	if len(header) < 12 {
		return false
	}
	isJPEG := header[0] == 0xFF && header[1] == 0xD8 && header[2] == 0xFF
	isPNG := bytes.Equal(header[:8], []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A})
	isWebP := bytes.Equal(header[:4], []byte("RIFF")) && bytes.Equal(header[8:12], []byte("WEBP"))

	switch mimeLower {
	case "image/jpeg":
		return isJPEG
	case "image/png":
		return isPNG
	case "image/webp":
		return isWebP
	default:
		return false
	}
}

// AssertNotUserOwnedAsset blocks any attempt by cache eviction or artwork sync to overwrite/remove user local files.
func AssertNotUserOwnedAsset(isUserLocal bool) error {
	if isUserLocal {
		return ErrProtectedUserLocalAsset
	}
	return nil
}

// VerifyCacheDirIsolated ensures the cache directory is strictly outside all user library roots.
func VerifyCacheDirIsolated(cacheDir string, libraryRoots []string) error {
	cleanCache := strings.ToLower(filepath.Clean(cacheDir))
	for _, root := range libraryRoots {
		cleanRoot := strings.ToLower(filepath.Clean(root))
		if cleanRoot == "" || cleanRoot == "." {
			continue
		}
		if cleanCache == cleanRoot || strings.HasPrefix(cleanCache, cleanRoot+string(filepath.Separator)) {
			return ErrCacheInsideUserLibrary
		}
	}
	return nil
}
