package unit_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sadigking5-cmd/Estiraha-Next/internal/config"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/domain"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/fsops"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/imagecache"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/naming"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/providers"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/review"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/secrets"
)

func TestAllSupportedMediaTypesAndImmutableIDs(t *testing.T) {
	if got := len(domain.AllMediaTypes()); got != 12 {
		t.Fatalf("expected 12 supported media types, got %d", got)
	}
	id1 := domain.NewImmutableID()
	id2 := domain.NewImmutableID()
	if id1 == "" || id2 == "" || id1 == id2 {
		t.Fatalf("expected unique non-empty immutable IDs, got %q and %q", id1, id2)
	}
}

func TestConfigSafetyInvariantsRejectUnsafeMutations(t *testing.T) {
	base := config.Default()
	if err := base.ValidateSafetyInvariants(); err != nil {
		t.Fatalf("default config must pass safety invariants: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*config.Config)
	}{
		{"disable_review_approval", func(c *config.Config) { c.Safety.RequireExplicitReviewApproval = false }},
		{"enable_auto_move", func(c *config.Config) { c.Safety.AllowAutomaticMove = true }},
		{"enable_auto_delete", func(c *config.Config) { c.Safety.AllowAutomaticDelete = true }},
		{"enable_overwrite", func(c *config.Config) { c.Safety.AllowOverwriteExisting = true }},
		{"enable_watcher_write", func(c *config.Config) { c.Safety.WatcherReadOnlyMode = false }},
		{"enable_archive_extract", func(c *config.Config) { c.Safety.ArchiveAutoExtract = true }},
		{"enable_svg", func(c *config.Config) { c.ImageEngine.AllowSVG = true }},
		{"cache_in_user_library", func(c *config.Config) { c.ImageEngine.StoreTempOutsideUserLibs = false }},
		{"auto_connect_unknown_api", func(c *config.Config) { c.APIHub.AutoConnectUnknownProviders = true }},
		{"deepseek_without_consent", func(c *config.Config) {
			c.APIHub.DeepSeekEnabled = true
			c.APIHub.DeepSeekRequireRequestConsent = false
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Default()
			tc.mutate(&cfg)
			if err := cfg.ValidateSafetyInvariants(); err == nil {
				t.Fatalf("expected safety invariant violation for %s, got nil", tc.name)
			}
		})
	}
}

func TestMandatoryNamingRules(t *testing.T) {
	// 1. Movie rule: "The Dark Knight 2008" (no parentheses, no dots, no quality/codec/group)
	gotMovie, err := naming.FormatMovieName("The.Dark.Knight.(2008).2160p.UHD.BluRay.x265.HDR10.RARBG", 2008)
	if err != nil {
		t.Fatalf("unexpected movie naming error: %v", err)
	}
	if gotMovie != "The Dark Knight 2008" {
		t.Fatalf("expected 'The Dark Knight 2008', got %q", gotMovie)
	}

	// 2. Arabic Movie retains authentic Arabic title + year
	gotArabicMovie, err := naming.FormatMovieName("الناظر (2000) 1080p WEB-DL", 2000)
	if err != nil || gotArabicMovie != "الناظر 2000" {
		t.Fatalf("expected 'الناظر 2000', got %q (err=%v)", gotArabicMovie, err)
	}

	// 3. Series rule: NO production year in folder name + S01 / 01.mp4
	gotSeries, err := naming.FormatSeriesFolder("باب.الحارة.1080p")
	if err != nil || gotSeries != "باب الحارة" {
		t.Fatalf("expected 'باب الحارة', got %q (err=%v)", gotSeries, err)
	}
	seasonDir, _ := naming.FormatSeasonFolder(1)
	epFile, _ := naming.FormatEpisodeFile(1, ".MP4")
	if seasonDir != "S01" || epFile != "01.mp4" {
		t.Fatalf("expected S01/01.mp4, got %s/%s", seasonDir, epFile)
	}

	// 4. Franchise parent folder without year
	gotFranchise, err := naming.FormatFranchiseFolder("The.Lion.King")
	if err != nil || gotFranchise != "The Lion King" {
		t.Fatalf("expected 'The Lion King', got %q (err=%v)", gotFranchise, err)
	}

	// 5. Celebrity rule: الاسم بالعربية (English Name)
	gotCeleb, err := naming.FormatCelebrityFolder("أحمد زكي", "Ahmed Zaki")
	if err != nil || gotCeleb != "أحمد زكي (Ahmed Zaki)" {
		t.Fatalf("expected 'أحمد زكي (Ahmed Zaki)', got %q (err=%v)", gotCeleb, err)
	}

	// 6. Unknown Program rule: retain original name, forbid auto-seasons, route to Review
	unknownPlan, err := naming.PlanProgramNaming("برنامج نادر غير معروف حلقة خاصة", false)
	if err != nil {
		t.Fatalf("unexpected program naming error: %v", err)
	}
	if unknownPlan.AllowAutoSeasons || !unknownPlan.RouteToReviewQueue || unknownPlan.FolderName != "برنامج نادر غير معروف حلقة خاصة" {
		t.Fatalf("unexpected unknown program plan: %+v", unknownPlan)
	}
}

func TestReviewPermitAndNoOverwriteGuards(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "source.mkv")
	existingDst := filepath.Join(tmp, "existing.mkv")
	freeDst := filepath.Join(tmp, "new_target.mkv")

	_ = os.WriteFile(src, []byte("synthetic-bytes-1"), 0o600)
	_ = os.WriteFile(existingDst, []byte("synthetic-bytes-2"), 0o600)

	// Unapproved permit MUST be blocked
	unapproved := review.ExecutionPermit{
		BatchID:      "batch-1",
		ReviewItemID: "item-1",
		Decision:     review.DecisionPending,
	}
	if err := fsops.VerifySafeDestination(unapproved, src, freeDst); err == nil {
		t.Fatal("expected unapproved execution permit to be blocked")
	}

	// Approved permit MUST still block overwriting existing file
	approved := review.ExecutionPermit{
		BatchID:      "batch-1",
		ReviewItemID: "item-1",
		Decision:     review.DecisionApproved,
		ApprovedAt:   time.Now().UTC(),
	}
	if err := fsops.VerifySafeDestination(approved, src, existingDst); err != fsops.ErrOverwriteForbidden {
		t.Fatalf("expected ErrOverwriteForbidden, got %v", err)
	}

	// Approved permit + non-existent destination succeeds precheck
	if err := fsops.VerifySafeDestination(approved, src, freeDst); err != nil {
		t.Fatalf("expected valid precheck, got %v", err)
	}
}

func TestAPIHubGovernanceAndPrivacyGuards(t *testing.T) {
	// ElCinema must remain deferred
	elcinema := providers.ProviderDescriptor{
		ID:            "elcinema",
		Mode:          providers.ModeDeferredLegalReview,
		TermsReviewed: false,
	}
	if err := providers.ValidateOutboundRequest(elcinema, true, "الناظر", false); err != providers.ErrProviderDeferredLegal {
		t.Fatalf("expected ErrProviderDeferredLegal for elcinema, got %v", err)
	}

	// OpenSubtitles search allowed, auto-download blocked
	opensub := providers.ProviderDescriptor{
		ID:            "opensubtitles",
		Mode:          providers.ModeSearchOnly,
		TermsReviewed: true,
	}
	if err := providers.ValidateOutboundRequest(opensub, false, "The Dark Knight", false); err != nil {
		t.Fatalf("expected OpenSubtitles search to pass, got %v", err)
	}
	if err := providers.ValidateOutboundRequest(opensub, true, "The Dark Knight", true); err != providers.ErrSubtitleDownloadBlocked {
		t.Fatalf("expected OpenSubtitles download to be blocked, got %v", err)
	}

	// DeepSeek requires per-request consent and blocks Windows path leaks
	deepseek := providers.ProviderDescriptor{
		ID:                       "deepseek",
		Mode:                     providers.ModeEnabled,
		TermsReviewed:            true,
		RequirePerRequestConsent: true,
	}
	if err := providers.ValidateOutboundRequest(deepseek, false, "أفلام أحمد زكي", false); err != providers.ErrMissingPerRequestConsent {
		t.Fatalf("expected per-request consent error, got %v", err)
	}
	if err := providers.ValidateOutboundRequest(deepseek, true, `D:\Movies\The Dark Knight 2008\movie.mkv`, false); err != providers.ErrPrivacyLeakDetected {
		t.Fatalf("expected Windows path privacy leak error, got %v", err)
	}
}

func TestSmartImageEngineSecurityAndIsolation(t *testing.T) {
	// SVG must be blocked both by MIME and header bytes
	svgAttempt := imagecache.CandidateImage{
		DeclaredMIME: "image/svg+xml",
		HeaderBytes:  []byte("<svg xmlns='http://www.w3.org/2000/svg'></svg>"),
		ByteSize:     100,
		Width:        200,
		Height:       300,
	}
	if err := imagecache.ValidateImage(svgAttempt, 10*1024*1024, 8192); err != imagecache.ErrForbiddenSVG {
		t.Fatalf("expected ErrForbiddenSVG, got %v", err)
	}

	// Valid PNG magic header passes
	validPNG := imagecache.CandidateImage{
		DeclaredMIME: "image/png",
		HeaderBytes:  []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0, 0, 0, 13},
		ByteSize:     204800,
		Width:        1000,
		Height:       1500,
	}
	if err := imagecache.ValidateImage(validPNG, 15*1024*1024, 8192); err != nil {
		t.Fatalf("expected valid PNG to pass, got %v", err)
	}

	// User local asset cannot be evicted/overwritten
	if err := imagecache.AssertNotUserOwnedAsset(true); err != imagecache.ErrProtectedUserLocalAsset {
		t.Fatalf("expected ErrProtectedUserLocalAsset, got %v", err)
	}

	// Cache directory inside user library root must be rejected
	if err := imagecache.VerifyCacheDirIsolated("/media/disk1/Movies/.cache", []string{"/media/disk1/Movies"}); err != imagecache.ErrCacheInsideUserLibrary {
		t.Fatalf("expected ErrCacheInsideUserLibrary, got %v", err)
	}
}

func TestSecretsVaultContract(t *testing.T) {
	v := secrets.NewDefaultVault()
	if err := v.Set("EstirahaNext/Providers/TMDb", "test-ephemeral-token"); err != nil {
		t.Fatalf("set secret failed: %v", err)
	}
	got, err := v.Get("EstirahaNext/Providers/TMDb")
	if err != nil || got != "test-ephemeral-token" {
		t.Fatalf("get secret mismatch: got=%q err=%v", got, err)
	}
	_ = v.Delete("EstirahaNext/Providers/TMDb")
	if _, err := v.Get("EstirahaNext/Providers/TMDb"); err != secrets.ErrSecretNotFound {
		t.Fatalf("expected ErrSecretNotFound after delete, got %v", err)
	}
}
