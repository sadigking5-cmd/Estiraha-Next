// Package providers implements the API Hub architecture for external metadata,
// subtitles, tracking, and optional AI assistance with strict legal, privacy,
// rate-limit, confidence-scoring, and conflict-detection boundaries.
package providers

import (
	"context"
	"errors"
	"strings"
	"time"
)

var (
	ErrUnknownProviderForbidden = errors.New("api hub: automatic connection to unknown/unregistered providers is prohibited")
	ErrProviderDeferredLegal    = errors.New("api hub: provider is deferred pending permitted integration & legal verification")
	ErrTermsNotReviewed         = errors.New("api hub: provider terms, license, and attribution must be reviewed before activation")
	ErrMissingPerRequestConsent = errors.New("api hub: provider requires explicit per-request user consent")
	ErrSubtitleDownloadBlocked  = errors.New("api hub: OpenSubtitles is restricted to search-only mode; automatic download is prohibited")
	ErrPrivacyLeakDetected      = errors.New("api hub privacy guard: request contains local disk paths or restricted provider payload")
)

// IntegrationMode defines operational constraints per provider.
type IntegrationMode string

const (
	ModeDisabled            IntegrationMode = "disabled"
	ModeEnabled             IntegrationMode = "enabled"
	ModeSearchOnly          IntegrationMode = "search_only"
	ModeDeferredLegalReview IntegrationMode = "deferred_legal_review"
)

// ProviderDescriptor models a registered provider inside the API Hub.
type ProviderDescriptor struct {
	ID                       string
	DisplayName              string
	Mode                     IntegrationMode
	PriorityRank             int
	RateLimitPerMinute       int
	CredentialManagerTarget  string
	RequirePerRequestConsent bool
	TermsReviewed            bool
	AttributionRequirement   string
}

// MetadataClaim tracks provenance, confidence, and conflicts for every single field.
type MetadataClaim struct {
	ProviderID       string
	ExternalEntityID string
	FieldName        string
	ValueJSON        string
	ConfidenceScore  float64 // [0.0 .. 1.0]
	FetchedAt        time.Time
}

// Adapter is the isolated contract every external provider implementation must satisfy.
type Adapter interface {
	Descriptor() ProviderDescriptor
	HealthCheck(ctx context.Context) error
}

// ValidateOutboundRequest enforces API Hub governance before any external network call.
func ValidateOutboundRequest(desc ProviderDescriptor, userApprovedThisRequest bool, sanitizedQuery string, isSubtitleDownload bool) error {
	if desc.ID == "" {
		return ErrUnknownProviderForbidden
	}
	if desc.ID == "elcinema" || desc.Mode == ModeDeferredLegalReview {
		return ErrProviderDeferredLegal
	}
	if desc.Mode == ModeDisabled {
		return errors.New("api hub: provider is currently disabled")
	}
	if !desc.TermsReviewed {
		return ErrTermsNotReviewed
	}
	if desc.RequirePerRequestConsent && !userApprovedThisRequest {
		return ErrMissingPerRequestConsent
	}
	if desc.Mode == ModeSearchOnly && isSubtitleDownload {
		return ErrSubtitleDownloadBlocked
	}
	// Privacy guard especially critical for optional AI providers (e.g., DeepSeek):
	// Never transmit Windows drive paths (e.g. D:\ or \\Server\) or UNC paths.
	if looksLikeLocalDiskPath(sanitizedQuery) {
		return ErrPrivacyLeakDetected
	}
	return nil
}

func looksLikeLocalDiskPath(input string) bool {
	s := strings.TrimSpace(input)
	if strings.HasPrefix(s, `\\`) {
		return true
	}
	for i := 0; i+2 < len(s); i++ {
		ch := s[i]
		if ((ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z')) && s[i+1] == ':' && (s[i+2] == '\\' || s[i+2] == '/') {
			return true
		}
	}
	return false
}
