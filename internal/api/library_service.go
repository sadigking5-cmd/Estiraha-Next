package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sadigking5-cmd/Estiraha-Next/internal/backup"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/config"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/db"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/domain"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/duplicates"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/errorsbox"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/identify"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/library"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/movies"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/naming"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/organizer"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/player"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/providers"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/reports"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/review"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/scanner"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/secrets"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/series"
)

type FolderPicker interface {
	PickDirectory() (string, error)
	EmitEvent(name string, payload any)
}

type ProviderStatusView struct {
	ID                       string `json:"id"`
	DisplayName              string `json:"displayName"`
	Category                 string `json:"category"`
	IntegrationStatus        string `json:"integrationStatus"`
	RequiresAPIKey           bool   `json:"requiresApiKey"`
	HasKeyConfigured         bool   `json:"hasKeyConfigured"`
	CredentialManagerTarget  string `json:"credentialManagerTarget"`
	RequirePerRequestConsent bool   `json:"requirePerRequestConsent"`
	AttributionRequirement   string `json:"attributionRequirement"`
}

type NotificationView struct {
	ID               string `json:"id"`
	NotificationType string `json:"notificationType"`
	TitleAr          string `json:"titleAr"`
	TitleEn          string `json:"titleEn"`
	BodyAr           string `json:"bodyAr"`
	BodyEn           string `json:"bodyEn"`
	IsRead           bool   `json:"isRead"`
	CreatedAt        string `json:"createdAt"`
}

type AuditLogView struct {
	ID        string `json:"id"`
	Category  string `json:"category"`
	Severity  string `json:"severity"`
	Actor     string `json:"actor"`
	Summary   string `json:"summary"`
	CreatedAt string `json:"createdAt"`
}

type AIAssistRequest struct {
	Prompt             string `json:"prompt"`
	WorkID             string `json:"workId"`
	AllowCloudDeepSeek bool   `json:"allowCloudDeepSeek"`
}

type AIAssistResponse struct {
	Mode              string   `json:"mode"`
	AnswerAr          string   `json:"answerAr"`
	AnswerEn          string   `json:"answerEn"`
	Recommendations   []string `json:"recommendations"`
	PrivacyGuaranteed bool     `json:"privacyGuaranteed"`
}

type ScanAndAnalyzeResult struct {
	Scan           scanner.ScanSummary            `json:"scan"`
	Identification identify.IdentificationSummary `json:"identification"`
	Duplicates     duplicates.AnalysisSummary     `json:"duplicates"`
	IssueCount     int                            `json:"issueCount"`
}

type SystemOverview struct {
	Stats               reports.DashboardStats        `json:"stats"`
	Disks               []library.DiskView            `json:"disks"`
	Roots               []library.RootView            `json:"roots"`
	ContinueWatching    []player.ContinueWatchingItem `json:"continueWatching"`
	RecentNotifications []NotificationView            `json:"recentNotifications"`
	ScanRunning         bool                          `json:"scanRunning"`
	ActivePolicies      map[string]string             `json:"activePolicies"`
}

type LibraryService struct {
	cfg       config.Config
	dbEngine  *db.Engine
	sqlDB     *sql.DB
	vault     secrets.Vault
	picker    FolderPicker
	backupSvc *backup.Service
	libSvc    *library.Service
	scanSvc   *scanner.Service
	identSvc  *identify.Service
	movieSvc  *movies.Service
	seriesSvc *series.Service
	dupSvc    *duplicates.Service
	reviewSvc *review.Service
	orgSvc    *organizer.Service
	errBoxSvc *errorsbox.Service
	reportSvc *reports.Service
	playerSvc *player.Service
}

func NewLibraryService(cfg config.Config, dbEngine *db.Engine, vault secrets.Vault, backupDir string, picker FolderPicker) *LibraryService {
	sqlDB := dbEngine.SQL()
	bSvc := backup.NewService(sqlDB, backupDir)
	lSvc := library.NewService(sqlDB)
	scSvc := scanner.NewService(sqlDB, lSvc)
	idSvc := identify.NewService(sqlDB, vault)
	mvSvc := movies.NewService(sqlDB)
	srSvc := series.NewService(sqlDB)
	dpSvc := duplicates.NewService(sqlDB)
	rvSvc := review.NewService(sqlDB)
	ogSvc := organizer.NewService(sqlDB, bSvc)
	ebSvc := errorsbox.NewService(sqlDB)
	rpSvc := reports.NewService(sqlDB, srSvc)
	plSvc := player.NewService(sqlDB)

	return &LibraryService{
		cfg:       cfg,
		dbEngine:  dbEngine,
		sqlDB:     sqlDB,
		vault:     vault,
		picker:    picker,
		backupSvc: bSvc,
		libSvc:    lSvc,
		scanSvc:   scSvc,
		identSvc:  idSvc,
		movieSvc:  mvSvc,
		seriesSvc: srSvc,
		dupSvc:    dpSvc,
		reviewSvc: rvSvc,
		orgSvc:    ogSvc,
		errBoxSvc: ebSvc,
		reportSvc: rpSvc,
		playerSvc: plSvc,
	}
}

func (s *LibraryService) Close() error {
	return s.playerSvc.Close()
}

func (s *LibraryService) GetSystemOverview() (SystemOverview, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	disks, roots, _ := s.libSvc.RefreshConnectivity(ctx)
	stats, err := s.reportSvc.GetDashboardStats(ctx)
	if err != nil {
		return SystemOverview{}, err
	}
	cw, _ := s.playerSvc.ListContinueWatching(ctx)
	notifs, _ := s.ListNotifications()
	policies := s.loadSettingsMap(ctx)

	return SystemOverview{
		Stats:               stats,
		Disks:               disks,
		Roots:               roots,
		ContinueWatching:    cw,
		RecentNotifications: notifs,
		ScanRunning:         s.scanSvc.IsRunning(),
		ActivePolicies:      policies,
	}, nil
}

func (s *LibraryService) PickFolderDialog() (string, error) {
	if s.picker == nil {
		return "", errors.New("native folder dialog requires desktop window session")
	}
	return s.picker.PickDirectory()
}

func (s *LibraryService) AddLibraryRoot(in library.AddRootInput) (library.RootView, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return s.libSvc.AddRoot(ctx, in)
}

func (s *LibraryService) RemoveLibraryRoot(rootID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return s.libSvc.RemoveRoot(ctx, rootID)
}

func (s *LibraryService) ListDisksAndRoots() (map[string]any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	disks, roots, err := s.libSvc.RefreshConnectivity(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"disks": disks,
		"roots": roots,
	}, nil
}

func (s *LibraryService) RunManualScanAndAnalyze(opts scanner.ScanOptions) (ScanAndAnalyzeResult, error) {
	ctx := context.Background()
	scanRes, err := s.scanSvc.RunManualScan(ctx, opts, func(prog scanner.ScanSummary) {
		if s.picker != nil {
			s.picker.EmitEvent("estiraha:scan-progress", prog)
		}
	})
	if err != nil {
		return ScanAndAnalyzeResult{Scan: scanRes}, err
	}

	identRes, err := s.identSvc.IdentifyLibrary(ctx)
	if err != nil {
		return ScanAndAnalyzeResult{Scan: scanRes}, err
	}
	dupRes, _ := s.dupSvc.AnalyzeDuplicates(ctx)
	issues, _ := s.reportSvc.GenerateHealthReport(ctx)

	out := ScanAndAnalyzeResult{
		Scan:           scanRes,
		Identification: identRes,
		Duplicates:     dupRes,
		IssueCount:     len(issues),
	}
	if s.picker != nil {
		s.picker.EmitEvent("estiraha:scan-completed", out)
	}
	return out, nil
}

func (s *LibraryService) CancelActiveScan() bool {
	return s.scanSvc.CancelActiveScan()
}

func (s *LibraryService) ListMovies(filter movies.MovieFilter) ([]map[string]any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cards, err := s.movieSvc.ListMovies(ctx, filter)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(cards))
	for _, c := range cards {
		raw, _ := json.Marshal(c)
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		m["posterUrl"] = s.playerSvc.ArtworkURLForFile(c.PosterFileID)
		out = append(out, m)
	}
	return out, nil
}

func (s *LibraryService) GetMovieDetail(workID string) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	d, err := s.movieSvc.GetMovieDetail(ctx, workID)
	if err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(d)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	m["posterUrl"] = s.playerSvc.ArtworkURLForFile(d.PosterFileID)
	return m, nil
}

func (s *LibraryService) UpdateMovieMetadata(in movies.UpdateMovieMetadataInput) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	d, err := s.movieSvc.UpdateMovieMetadata(ctx, in)
	if err != nil {
		return nil, err
	}
	_, _ = s.reportSvc.GenerateHealthReport(ctx)
	raw, _ := json.Marshal(d)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	m["posterUrl"] = s.playerSvc.ArtworkURLForFile(d.PosterFileID)
	return m, nil
}

func (s *LibraryService) ListSeries(filter series.SeriesFilter) ([]map[string]any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cards, err := s.seriesSvc.ListSeries(ctx, filter)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(cards))
	for _, c := range cards {
		raw, _ := json.Marshal(c)
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		m["posterUrl"] = s.playerSvc.ArtworkURLForFile(c.PosterFileID)
		out = append(out, m)
	}
	return out, nil
}

func (s *LibraryService) GetSeriesDetail(workID string) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	d, err := s.seriesSvc.GetSeriesDetail(ctx, workID)
	if err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(d)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	m["posterUrl"] = s.playerSvc.ArtworkURLForFile(d.PosterFileID)
	return m, nil
}

func (s *LibraryService) UpdateSeriesMetadata(in series.UpdateSeriesMetadataInput) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	d, err := s.seriesSvc.UpdateSeriesMetadata(ctx, in)
	if err != nil {
		return nil, err
	}
	_, _ = s.reportSvc.GenerateHealthReport(ctx)
	raw, _ := json.Marshal(d)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	m["posterUrl"] = s.playerSvc.ArtworkURLForFile(d.PosterFileID)
	return m, nil
}

func (s *LibraryService) SetSeasonExpectedEpisodes(seasonID string, expectedCount int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.seriesSvc.SetSeasonExpectedEpisodes(ctx, seasonID, expectedCount); err != nil {
		return err
	}
	_, _ = s.reportSvc.GenerateHealthReport(ctx)
	return nil
}

func (s *LibraryService) PreviewNamingRules(title string, year int, isSeries bool, seasonNum int, epNum int) (map[string]any, error) {
	if isSeries {
		sf, snf, files, err := naming.PlanEpisodeBundle(title, max(1, seasonNum), max(1, epNum), []naming.CompanionFilePlan{
			{OriginalFilename: "raw.ep.mp4", Extension: ".mp4", FileKind: "video"},
			{OriginalFilename: "raw.ep.srt", Extension: ".srt", FileKind: "subtitle"},
			{OriginalFilename: "raw.ep.shorts", Extension: ".shorts", FileKind: "shorts"},
		}, false)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"seriesFolder": sf,
			"seasonFolder": snf,
			"posterFile":   sf + "/poster.jpg",
			"files":        files,
		}, nil
	}
	folder, files, err := naming.PlanMovieBundle(title, year, []naming.CompanionFilePlan{
		{OriginalFilename: "raw.movie.mkv", Extension: ".mkv", FileKind: "video"},
		{OriginalFilename: "raw.movie.srt", Extension: ".srt", FileKind: "subtitle"},
		{OriginalFilename: "raw.movie.jpg", Extension: ".jpg", FileKind: "image"},
		{OriginalFilename: "raw.movie.shorts", Extension: ".shorts", FileKind: "shorts"},
	}, false)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"movieFolder": folder,
		"files":       files,
	}, nil
}

func (s *LibraryService) ListDuplicateGroups() ([]duplicates.DuplicateGroupView, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return s.dupSvc.ListDuplicateGroups(ctx)
}

func (s *LibraryService) GenerateOrganizationPlan(customTitle string) (review.ReviewBatchView, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return s.reviewSvc.GenerateOrganizationPlan(ctx, customTitle)
}

func (s *LibraryService) ListReviewBatches() ([]review.ReviewBatchView, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return s.reviewSvc.ListBatches(ctx)
}

func (s *LibraryService) ListReviewBatchItems(batchID string) ([]review.ReviewItemView, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return s.reviewSvc.ListBatchItems(ctx, batchID)
}

func (s *LibraryService) DecideReviewItem(in review.ItemDecisionInput) (review.ReviewItemView, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return s.reviewSvc.DecideItem(ctx, in)
}

func (s *LibraryService) ApproveAllSafeReviewItems(batchID string) (review.ReviewBatchView, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return s.reviewSvc.ApproveAllSafeItems(ctx, batchID)
}

func (s *LibraryService) ExecuteApprovedBatch(batchID string) (organizer.ExecutionResult, error) {
	if s.scanSvc.IsRunning() {
		return organizer.ExecutionResult{}, errors.New("safety lock: cannot execute file organization while a library scan is running")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	return s.orgSvc.ExecuteApprovedBatch(ctx, batchID)
}

func (s *LibraryService) RollbackBatch(batchID string) (organizer.RollbackResult, error) {
	if s.scanSvc.IsRunning() {
		return organizer.RollbackResult{}, errors.New("safety lock: cannot rollback files while a library scan is running")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	return s.orgSvc.RollbackBatch(ctx, batchID)
}

func (s *LibraryService) ListOperationJournal(batchID string) ([]organizer.JournalEntryView, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return s.orgSvc.ListJournalEntries(ctx, batchID)
}

func (s *LibraryService) ListErrorFolderCategories() []errorsbox.CategorySpec {
	return errorsbox.AllCategories()
}

func (s *LibraryService) ListErrorFolderEntries(categoryFilter string) ([]errorsbox.EntryView, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cat := errorsbox.CategoryCode(strings.TrimSpace(categoryFilter))
	if cat == "all" {
		cat = ""
	}
	return s.errBoxSvc.ListEntries(ctx, cat, "")
}

func (s *LibraryService) RestoreErrorFolderEntry(entryID string) (errorsbox.EntryView, error) {
	if s.scanSvc.IsRunning() {
		return errorsbox.EntryView{}, errors.New("safety lock: cannot restore files while a library scan is running")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return s.errBoxSvc.RestoreEntry(ctx, strings.TrimSpace(entryID))
}

func (s *LibraryService) RefreshAndListHealthIssues(kindFilter string) ([]reports.HealthIssueView, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, _, _ = s.libSvc.RefreshConnectivity(ctx)
	if _, err := s.reportSvc.GenerateHealthReport(ctx); err != nil {
		return nil, err
	}
	return s.reportSvc.ListHealthIssues(ctx, kindFilter)
}

func (s *LibraryService) StartPlaybackSession(workID, episodeID, explicitFileID string) (player.PlaybackSession, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return s.playerSvc.BuildPlaybackSession(ctx, workID, episodeID, explicitFileID)
}

func (s *LibraryService) SavePlaybackProgress(in player.ProgressUpdateInput) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return s.playerSvc.SavePlaybackProgress(ctx, in)
}

func (s *LibraryService) LaunchExternalFallbackPlayer(fileID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.playerSvc.LaunchInSystemPlayer(ctx, fileID)
}

func (s *LibraryService) AskSmartAssistant(req AIAssistRequest) (AIAssistResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	q := strings.TrimSpace(req.Prompt)
	stats, _ := s.reportSvc.GetDashboardStats(ctx)

	if req.AllowCloudDeepSeek {
		dsKey, kErr := s.vault.Get("EstirahaNext/Providers/DeepSeek")
		if kErr == nil && strings.TrimSpace(dsKey) != "" {
			desc := providers.ProviderDescriptor{
				ID:                       "deepseek",
				Mode:                     providers.ModeEnabled,
				TermsReviewed:            true,
				RequirePerRequestConsent: true,
			}
			sanitizedPrompt := fmt.Sprintf("Library stats: %d movies, %d series. User question: %s", stats.TotalMovies, stats.TotalSeries, q)
			if vErr := providers.ValidateOutboundRequest(desc, true, sanitizedPrompt, false); vErr == nil {
				if ans, cErr := callDeepSeekChat(ctx, strings.TrimSpace(dsKey), sanitizedPrompt); cErr == nil && ans != "" {
					return AIAssistResponse{
						Mode:              "deepseek_cloud_consented",
						AnswerAr:          ans,
						AnswerEn:          ans,
						Recommendations:   s.localTopRecommendations(ctx),
						PrivacyGuaranteed: true,
					}, nil
				}
			}
		}
	}

	recs := s.localTopRecommendations(ctx)
	ansAr := fmt.Sprintf(
		"تحليل محلي فوري (بدون إنترنت): تحتوي مكتبتك حالياً على %d فيلم، و%d مسلسل (%d حلقة متوفرة)، و%d ملف .shorts محمي. يوجد %d عنصر يحتاج مراجعة تعريف، و%d نسخة احتياطية موثقة.",
		stats.TotalMovies, stats.TotalSeries, stats.TotalEpisodesPresent, stats.ProtectedShortsCount, stats.NeedsReviewCount, stats.VerifiedBackupsCount,
	)
	ansEn := fmt.Sprintf(
		"Offline Local Intelligence: Your library currently holds %d movie(s), %d series (%d present episodes), and %d protected .shorts file(s). %d item(s) await identification review, with %d verified backup(s).",
		stats.TotalMovies, stats.TotalSeries, stats.TotalEpisodesPresent, stats.ProtectedShortsCount, stats.NeedsReviewCount, stats.VerifiedBackupsCount,
	)
	if q != "" {
		ansAr += fmt.Sprintf(" بخصوص سؤالك «%s»: يمكنك استخدام التصفية الذكية أو توليد خطة تنظيم للمعاينة قبل أي تعديل.", q)
		ansEn += fmt.Sprintf(" Regarding \"%s\": use FTS5 instant search or generate a non-destructive Organization Plan in the Smart Organizer.", q)
	}

	return AIAssistResponse{
		Mode:              "local_offline",
		AnswerAr:          ansAr,
		AnswerEn:          ansEn,
		Recommendations:   recs,
		PrivacyGuaranteed: true,
	}, nil
}

func (s *LibraryService) LookupTMDbCandidates(query string, mediaType string, year int) ([]identify.TMDbCandidate, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return s.identSvc.LookupTMDbOptional(ctx, query, mediaType, year)
}

func (s *LibraryService) ListProviders() ([]ProviderStatusView, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, err := s.sqlDB.QueryContext(ctx, `
		SELECT id, display_name, provider_category, integration_status, requires_api_key,
		       credential_manager_target, require_per_request_consent, attribution_requirement
		FROM data_providers ORDER BY priority_rank ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProviderStatusView{}
	for rows.Next() {
		var p ProviderStatusView
		var reqKey, reqConsent int
		if err := rows.Scan(&p.ID, &p.DisplayName, &p.Category, &p.IntegrationStatus, &reqKey, &p.CredentialManagerTarget, &reqConsent, &p.AttributionRequirement); err == nil {
			p.RequiresAPIKey = reqKey == 1
			p.RequirePerRequestConsent = reqConsent == 1
			if p.CredentialManagerTarget != "" {
				if val, gErr := s.vault.Get(p.CredentialManagerTarget); gErr == nil && strings.TrimSpace(val) != "" {
					p.HasKeyConfigured = true
				}
			}
			out = append(out, p)
		}
	}
	return out, rows.Err()
}

func (s *LibraryService) SaveProviderAPIKey(providerID, apiKey string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var target, status string
	if err := s.sqlDB.QueryRowContext(ctx, `SELECT credential_manager_target, integration_status FROM data_providers WHERE id = ?`, providerID).Scan(&target, &status); err != nil {
		return err
	}
	if status == "deferred_legal_review" {
		return errors.New("provider is deferred pending legal review and cannot be activated")
	}
	trimmed := strings.TrimSpace(apiKey)
	if trimmed == "" {
		_ = s.vault.Delete(target)
		_, _ = s.sqlDB.ExecContext(ctx, `UPDATE data_providers SET integration_status = 'disabled' WHERE id = ?`, providerID)
		return nil
	}
	if err := s.vault.Set(target, trimmed); err != nil {
		return err
	}
	newState := "enabled"
	if providerID == "opensubtitles" {
		newState = "search_only"
	}
	_, _ = s.sqlDB.ExecContext(ctx, `UPDATE data_providers SET integration_status = ?, terms_reviewed = 1 WHERE id = ?`, newState, providerID)
	_, _ = s.sqlDB.ExecContext(ctx, `
		INSERT INTO audit_logs (id, event_category, severity, actor, summary, details_json)
		VALUES (?, 'security', 'info', 'user', ?, '{}')
	`, domain.NewImmutableID(), fmt.Sprintf("Stored API key for %s in Windows Credential Manager (never in DB)", providerID))
	return nil
}

func (s *LibraryService) CreateVerifiedBackup() (backup.BackupRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return s.backupSvc.CreateVerifiedBackup(ctx, "manual_user")
}

func (s *LibraryService) VerifyBackup(backupID string) (backup.BackupRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return s.backupSvc.VerifyBackupRecord(ctx, backupID)
}

func (s *LibraryService) ListBackups() ([]backup.BackupRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return s.backupSvc.ListBackups(ctx)
}

func (s *LibraryService) ListNotifications() ([]NotificationView, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, err := s.sqlDB.QueryContext(ctx, `
		SELECT id, notification_type, title_ar, title_en, body_ar, body_en, is_read, created_at
		FROM notifications ORDER BY created_at DESC LIMIT 30
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []NotificationView{}
	for rows.Next() {
		var n NotificationView
		var rInt int
		if err := rows.Scan(&n.ID, &n.NotificationType, &n.TitleAr, &n.TitleEn, &n.BodyAr, &n.BodyEn, &rInt, &n.CreatedAt); err == nil {
			n.IsRead = rInt == 1
			out = append(out, n)
		}
	}
	return out, rows.Err()
}

func (s *LibraryService) MarkNotificationsRead() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.sqlDB.ExecContext(ctx, `UPDATE notifications SET is_read = 1 WHERE is_read = 0`)
	return err
}

func (s *LibraryService) ListAuditLogs() ([]AuditLogView, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, err := s.sqlDB.QueryContext(ctx, `
		SELECT id, event_category, severity, actor, summary, created_at
		FROM audit_logs ORDER BY created_at DESC LIMIT 100
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditLogView{}
	for rows.Next() {
		var a AuditLogView
		if err := rows.Scan(&a.ID, &a.Category, &a.Severity, &a.Actor, &a.Summary, &a.CreatedAt); err == nil {
			out = append(out, a)
		}
	}
	return out, rows.Err()
}

func (s *LibraryService) loadSettingsMap(ctx context.Context) map[string]string {
	out := map[string]string{}
	rows, err := s.sqlDB.QueryContext(ctx, `SELECT setting_key, setting_value_json FROM app_settings`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err == nil {
			out[k] = strings.Trim(v, `"`)
		}
	}
	return out
}

func (s *LibraryService) localTopRecommendations(ctx context.Context) []string {
	rows, err := s.sqlDB.QueryContext(ctx, `
		SELECT w.canonical_folder_name
		FROM works w
		LEFT JOIN watch_states ws ON ws.work_id = w.id AND ws.episode_id IS NULL
		WHERE COALESCE(ws.watch_status, 'unwatched') = 'unwatched'
		ORDER BY w.confidence_score DESC, w.updated_at DESC
		LIMIT 5
	`)
	if err != nil {
		return []string{}
	}
	defer rows.Close()
	var recs []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil && name != "" {
			recs = append(recs, name)
		}
	}
	return recs
}

func callDeepSeekChat(ctx context.Context, apiKey, sanitizedPrompt string) (string, error) {
	bodyMap := map[string]any{
		"model": "deepseek-chat",
		"messages": []map[string]string{
			{"role": "system", "content": "You are Estiraha Next's concise cinema library assistant. Answer in Arabic and English."},
			{"role": "user", "content": sanitizedPrompt},
		},
	}
	raw, _ := json.Marshal(bodyMap)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.deepseek.com/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("deepseek status %d", resp.StatusCode)
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil || len(parsed.Choices) == 0 {
		return "", errors.New("empty deepseek response")
	}
	return parsed.Choices[0].Message.Content, nil
}
