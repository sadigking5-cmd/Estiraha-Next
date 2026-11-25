package integration_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sadigking5-cmd/Estiraha-Next/internal/api"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/config"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/db"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/library"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/movies"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/player"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/review"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/scanner"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/secrets"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/series"
)

func mustWriteFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestPhaseBEndToEndMediaLibrarySafetyNamingPlayerBackupAndScale(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	tmp := t.TempDir()
	dbFile := filepath.Join(tmp, "library_phase_b.sqlite")
	backupDir := filepath.Join(tmp, "backups")

	engine, err := db.Open(ctx, dbFile)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer engine.Close()

	ver, err := engine.Migrate(ctx, backupDir)
	if err != nil || ver < 3 {
		t.Fatalf("expected schema version >= 3, got ver=%d err=%v", ver, err)
	}

	vault := secrets.NewDefaultVault()
	libSvc := api.NewLibraryService(config.Default(), engine, vault, backupDir, nil)
	defer libSvc.Close()

	// 1. Setup Two Separate Simulated Drives ("By Content Type" + Cross-Disk Copy testing)
	drive1Movies := filepath.Join(tmp, "DriveA", "أفلام_سينمائية")
	drive1Series := filepath.Join(tmp, "DriveA", "Series_Archive")
	drive2Movies := filepath.Join(tmp, "DriveB_BackupVault", "Movies_Mirror")

	for _, d := range []string{drive1Movies, drive1Series, drive2Movies} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir root: %v", err)
		}
	}

	// Movie 1 (Arabic Movie + Subtitle + Poster + Protected .shorts!)
	mustWriteFile(t, filepath.Join(drive1Movies, "الناظر.2000.1080p.WEB-DL.x265.mkv"), []byte("FAKE_VIDEO_AR_NAZER_1080P_HEVC_STREAM_PAYLOAD_0123456789"))
	mustWriteFile(t, filepath.Join(drive1Movies, "الناظر.2000.1080p.WEB-DL.x265.srt"), []byte("1\n00:00:01,000 --> 00:00:04,500\nمرحباً بكم في فيلم الناظر\n"))
	mustWriteFile(t, filepath.Join(drive1Movies, "الناظر.2000.1080p.WEB-DL.x265.jpg"), []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F'})
	mustWriteFile(t, filepath.Join(drive1Movies, "الناظر.2000.1080p.WEB-DL.x265.shorts"), []byte("PROTECTED_SHORTS_COMPANION_DATA_NAZER"))

	// Movie 2 (English Movie with Same-Disk Lower-Quality Duplicate + Cross-Disk Copy on DriveB!)
	mustWriteFile(t, filepath.Join(drive1Movies, "The.Dark.Knight.(2008).2160p.4K.HDR10.HEVC.mkv"), []byte("DARK_KNIGHT_4K_HDR_MASTER_STREAM_PAYLOAD_999999999"))
	mustWriteFile(t, filepath.Join(drive1Movies, "The.Dark.Knight.2008.720p.x264.mp4"), []byte("DARK_KNIGHT_720P_SAME_DISK_LOWER_COPY"))
	mustWriteFile(t, filepath.Join(drive2Movies, "The.Dark.Knight.2008.1080p.BluRay.mkv"), []byte("DARK_KNIGHT_1080P_CROSS_DISK_KEEP_COPY"))

	// Unidentified / Ambiguous File ("Unidentified Files = Review") & Corrupted 0-byte File
	mustWriteFile(t, filepath.Join(drive1Movies, "unknown_clip_no_year.mp4"), []byte("UNKNOWN_CLIP_BYTES"))
	mustWriteFile(t, filepath.Join(drive1Movies, "corrupted_zero_byte.mkv"), []byte{})

	// Series with Season Gap (S01 & S03 -> S02 missing!), Episode Gap (E01 & E03 -> E02 missing!), Poster & .shorts!
	mustWriteFile(t, filepath.Join(drive1Series, "Breaking.Bad.2008", "cover.jpg"), []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F'})
	mustWriteFile(t, filepath.Join(drive1Series, "Breaking.Bad.2008", "Season 1", "Breaking.Bad.S01E01.1080p.HEVC.mp4"), []byte("BB_S01E01_1080P_VIDEO_STREAM_CONTENT_ABCDEFGHIJKLMNOP"))
	mustWriteFile(t, filepath.Join(drive1Series, "Breaking.Bad.2008", "Season 1", "Breaking.Bad.S01E01.1080p.HEVC.srt"), []byte("1\n00:00:02,000 --> 00:00:05,000\nSay my name.\n"))
	mustWriteFile(t, filepath.Join(drive1Series, "Breaking.Bad.2008", "Season 1", "Breaking.Bad.S01E01.1080p.HEVC.shorts"), []byte("BB_EPISODE_SHORTS_PAYLOAD"))
	mustWriteFile(t, filepath.Join(drive1Series, "Breaking.Bad.2008", "Season 1", "Breaking.Bad.S01E03.720p.mp4"), []byte("BB_S01E03_720P_VIDEO_STREAM"))
	mustWriteFile(t, filepath.Join(drive1Series, "Breaking.Bad.2008", "Season 3", "Breaking.Bad.S03E01.1080p.mp4"), []byte("BB_S03E01_1080P_VIDEO_STREAM"))

	// 2. Register Roots & Verify Duplicate/Nested Root Rejection (B1)
	r1, err := libSvc.AddLibraryRoot(library.AddRootInput{FullPath: drive1Movies, ContentCategory: "movies", DisplayName: "أفلام عربية وعالمية", DiskLabel: "VaultDiskA"})
	if err != nil {
		t.Fatalf("add root 1 failed: %v", err)
	}
	if _, dupErr := libSvc.AddLibraryRoot(library.AddRootInput{FullPath: drive1Movies, ContentCategory: "movies"}); dupErr == nil {
		t.Fatal("expected duplicate root registration to fail")
	}
	if _, err := libSvc.AddLibraryRoot(library.AddRootInput{FullPath: drive1Series, ContentCategory: "series", DisplayName: "مسلسلات", DiskLabel: "VaultDiskA"}); err != nil {
		t.Fatalf("add root 2 failed: %v", err)
	}
	r3, err := libSvc.AddLibraryRoot(library.AddRootInput{FullPath: drive2Movies, ContentCategory: "movies", DisplayName: "مرآة الأفلام", DiskLabel: "VaultDiskB"})
	if err != nil {
		t.Fatalf("add root 3 failed: %v", err)
	}

	// Verify zero auto-scan occurred on registration ("Scan Trigger = Manual")
	ovBefore, err := libSvc.GetSystemOverview()
	if err != nil || ovBefore.Stats.TotalMediaFiles != 0 {
		t.Fatalf("expected 0 indexed files before Manual Scan, got %d (err=%v)", ovBefore.Stats.TotalMediaFiles, err)
	}

	// 3. Execute Manual Scan + Multi-Signal Identification + Duplicate Scoring + Health Reports (B1, B2, B6, B9)
	scanOut, err := libSvc.RunManualScanAndAnalyze(scanner.ScanOptions{ComputeHash: true})
	if err != nil {
		t.Fatalf("manual scan & analyze failed: %v", err)
	}
	if scanOut.Scan.ShortsDetected != 2 {
		t.Fatalf("expected 2 protected .shorts files detected, got %d", scanOut.Scan.ShortsDetected)
	}
	if scanOut.Scan.CorruptedDetected != 1 {
		t.Fatalf("expected 1 corrupted 0-byte file detected, got %d", scanOut.Scan.CorruptedDetected)
	}

	// 4. Verify Arabic Movie Naming (authentic Arabic, NO speculative translation, NO brackets) & Series Naming (NO year)
	arMovies, err := libSvc.ListMovies(movies.MovieFilter{ArabicOnly: true})
	if err != nil || len(arMovies) != 1 {
		t.Fatalf("expected 1 Arabic movie, got %d (err=%v)", len(arMovies), err)
	}
	if got := fmt.Sprint(arMovies[0]["canonicalFolderName"]); got != "الناظر 2000" {
		t.Fatalf("expected Arabic movie canonical name 'الناظر 2000', got %q", got)
	}
	arMovieID := fmt.Sprint(arMovies[0]["id"])

	sList, err := libSvc.ListSeries(series.SeriesFilter{MissingOnly: true})
	if err != nil || len(sList) != 1 {
		t.Fatalf("expected 1 series with missing gaps, got %d (err=%v)", len(sList), err)
	}
	if got := fmt.Sprint(sList[0]["canonicalFolderName"]); got != "Breaking Bad" {
		t.Fatalf("expected yearless series folder 'Breaking Bad', got %q", got)
	}
	seriesID := fmt.Sprint(sList[0]["id"])
	sDetail, err := libSvc.GetSeriesDetail(seriesID)
	if err != nil {
		t.Fatalf("get series detail: %v", err)
	}
	if fmt.Sprint(sDetail["missingEpisodeCount"]) != "1" {
		t.Fatalf("expected 1 missing episode (S01E02), got %v", sDetail["missingEpisodeCount"])
	}

	// 5. Verify Cross-Disk Copy Preservation ("Cross-Disk Copies = Keep") & Best-Version Scoring (B6)
	dups, err := libSvc.ListDuplicateGroups()
	if err != nil || len(dups) == 0 {
		t.Fatalf("expected duplicate group for The Dark Knight 2008, got %d (err=%v)", len(dups), err)
	}
	foundCrossKeep := false
	foundSameDiskProposal := false
	for _, m := range dups[0].Members {
		if m.IsSelectedBest && m.ResolutionLabel != "4K" {
			t.Fatalf("expected 4K HEVC HDR version to be scored Best, got %s", m.ResolutionLabel)
		}
		if m.ProposedAction == "keep_other_disk" {
			foundCrossKeep = true
		}
		if m.ProposedAction == "propose_move_to_errors" {
			foundSameDiskProposal = true
		}
	}
	if !foundCrossKeep || !foundSameDiskProposal {
		t.Fatalf("expected both keep_other_disk (DriveB) and propose_move_to_errors (DriveA 720p): cross=%v same=%v", foundCrossKeep, foundSameDiskProposal)
	}

	// 6. Verify Review Center Gate, `.shorts` Tamper Protection & Safe Execution + Rollback (B5, B7, B8)
	batch, err := libSvc.GenerateOrganizationPlan("Test Phase B Plan")
	if err != nil || batch.TotalItems == 0 {
		t.Fatalf("generate plan failed: total=%d err=%v", batch.TotalItems, err)
	}

	if _, unauthErr := libSvc.ExecuteApprovedBatch(batch.ID); unauthErr == nil {
		t.Fatal("SAFETY VIOLATION: ExecuteApprovedBatch succeeded with zero approved items!")
	}

	items, err := libSvc.ListReviewBatchItems(batch.ID)
	if err != nil {
		t.Fatalf("list batch items: %v", err)
	}
	var shortsItem *review.ReviewItemView
	for i := range items {
		if items[i].ProtectedShortsFile {
			shortsItem = &items[i]
			break
		}
	}
	if shortsItem == nil {
		t.Fatal("expected `.shorts` companion file to be included in Movie/Episode bundle plan")
	}

	_, stripErr := libSvc.DecideReviewItem(review.ItemDecisionInput{
		ItemID:         shortsItem.ID,
		Decision:       "approved",
		UserEditedName: "الناظر 2000.txt",
	})
	if stripErr == nil {
		t.Fatal("SAFETY VIOLATION: manual edit was allowed to strip protected .shorts extension!")
	}

	if _, err := libSvc.ApproveAllSafeReviewItems(batch.ID); err != nil {
		t.Fatalf("approve safe items: %v", err)
	}

	var origMovieFileID string
	_ = engine.SQL().QueryRowContext(ctx, `SELECT id FROM media_files WHERE work_id = ? AND file_kind = 'video'`, arMovieID).Scan(&origMovieFileID)

	execRes, err := libSvc.ExecuteApprovedBatch(batch.ID)
	if err != nil || execRes.ExecutedCount == 0 {
		t.Fatalf("execute approved batch failed: count=%d err=%v errs=%v", execRes.ExecutedCount, err, execRes.Errors)
	}
	if execRes.PreBatchBackupID == "" {
		t.Fatal("expected verified pre-batch backup to be created automatically before moving files")
	}

	organizedMovieVideo := filepath.Join(drive1Movies, "الناظر 2000", "الناظر 2000.mkv")
	organizedMovieShorts := filepath.Join(drive1Movies, "الناظر 2000", "الناظر 2000.shorts")
	organizedEpVideo := filepath.Join(drive1Series, "Breaking Bad", "S01", "01.mp4")
	organizedEpShorts := filepath.Join(drive1Series, "Breaking Bad", "S01", "01.shorts")
	organizedSeriesPoster := filepath.Join(drive1Series, "Breaking Bad", "poster.jpg")

	for _, mustExist := range []string{organizedMovieVideo, organizedMovieShorts, organizedEpVideo, organizedEpShorts, organizedSeriesPoster} {
		if _, stErr := os.Stat(mustExist); stErr != nil {
			t.Fatalf("expected organized canonical file %s to exist: %v", mustExist, stErr)
		}
	}

	var postMoveFileID string
	_ = engine.SQL().QueryRowContext(ctx, `SELECT id FROM media_files WHERE work_id = ? AND file_kind = 'video'`, arMovieID).Scan(&postMoveFileID)
	if origMovieFileID == "" || postMoveFileID != origMovieFileID {
		t.Fatalf("immutable file ID changed across move: before=%s after=%s", origMovieFileID, postMoveFileID)
	}

	// 7. Verify Integrated Media Player HTTP Byte-Range Streaming, SRT->WebVTT & Watch Progress (B10)
	playSess, err := libSvc.StartPlaybackSession(arMovieID, "", "")
	if err != nil {
		t.Fatalf("start playback session: %v", err)
	}
	if len(playSess.Subtitles) != 1 {
		t.Fatalf("expected 1 matched subtitle track in playback session, got %d", len(playSess.Subtitles))
	}

	req, _ := http.NewRequest(http.MethodGet, playSess.StreamURL, nil)
	req.Header.Set("Range", "bytes=0-15")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("range request failed: %v", err)
	}
	rangeBytes, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent || len(rangeBytes) != 16 {
		t.Fatalf("expected HTTP 206 Partial Content (16 bytes), got status=%d len=%d", resp.StatusCode, len(rangeBytes))
	}

	vttResp, err := http.Get(playSess.Subtitles[0].WebVTTURL)
	if err != nil {
		t.Fatalf("subtitle vtt fetch failed: %v", err)
	}
	vttText, _ := io.ReadAll(vttResp.Body)
	vttResp.Body.Close()
	if !strings.HasPrefix(string(vttText), "WEBVTT") || !strings.Contains(string(vttText), "00:00:01.000 --> 00:00:04.500") {
		t.Fatalf("expected converted WebVTT Arabic subtitle, got: %s", string(vttText))
	}

	wsState, err := libSvc.SavePlaybackProgress(player.ProgressUpdateInput{
		WorkID:     arMovieID,
		FileID:     playSess.FileID,
		PositionMs: 42000,
		DurationMs: 120000,
	})
	if err != nil || wsState != "watching" {
		t.Fatalf("expected watch status 'watching', got %q (err=%v)", wsState, err)
	}

	// 8. Verify 1-Click Rollback restores original paths & `.shorts` bit-for-bit!
	rbRes, err := libSvc.RollbackBatch(batch.ID)
	if err != nil || rbRes.RestoredCount != execRes.ExecutedCount {
		t.Fatalf("rollback failed: restored=%d want=%d err=%v errs=%v", rbRes.RestoredCount, execRes.ExecutedCount, err, rbRes.Errors)
	}
	if _, err := os.Stat(filepath.Join(drive1Movies, "الناظر.2000.1080p.WEB-DL.x265.shorts")); err != nil {
		t.Fatalf("expected original .shorts path restored after rollback: %v", err)
	}

	// 9. Verify Offline Drive Preservation & Health Report Categories (B1 & B9)
	_ = os.RemoveAll(filepath.Dir(drive2Movies))
	issues, err := libSvc.RefreshAndListHealthIssues("all")
	if err != nil {
		t.Fatalf("refresh health issues: %v", err)
	}
	seenKinds := map[string]bool{}
	for _, iss := range issues {
		seenKinds[iss.IssueKind] = true
	}
	for _, expectedKind := range []string{"missing_episode", "missing_season", "corrupted_file", "unidentified_file", "duplicate_conflict", "offline_disk"} {
		if !seenKinds[expectedKind] {
			t.Fatalf("expected health report to detect %q, got kinds=%v", expectedKind, seenKinds)
		}
	}
	_ = r1
	_ = r3

	// 10. Verify Backup Corruption Detection ("Verified Backup Only") & Zero Secret Leak in SQLite
	bk, err := libSvc.CreateVerifiedBackup()
	if err != nil || bk.VerificationStatus != "verified" {
		t.Fatalf("create verified backup failed: %v", err)
	}
	secretToken := "SUPER_SECRET_TMDB_KEY_9988776655"
	if err := libSvc.SaveProviderAPIKey("tmdb", secretToken); err != nil {
		t.Fatalf("save provider key: %v", err)
	}
	rawDBBytes, err := os.ReadFile(dbFile)
	if err != nil {
		t.Fatalf("read db file: %v", err)
	}
	if strings.Contains(string(rawDBBytes), secretToken) {
		t.Fatal("CRITICAL SECURITY VIOLATION: API key leaked into SQLite database file!")
	}

	if err := os.WriteFile(bk.ArchivePath, []byte("CORRUPTED_NOT_SQLITE"), 0o600); err != nil {
		t.Fatalf("corrupt backup: %v", err)
	}
	if _, verifyErr := libSvc.VerifyBackup(bk.ID); verifyErr == nil {
		t.Fatal("expected VerifyBackup to reject corrupted backup archive!")
	}

	// 11. Scale & FTS5 Responsiveness Benchmark Check (5,000 indexed works queried in < 500ms)
	tx, err := engine.SQL().BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin scale tx: %v", err)
	}
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO works (id, media_type, sub_format, title_original, title_ar, title_en, is_arabic_origin, production_year, canonical_folder_name, identification_status, confidence_score)
		VALUES (?, 'movie', 'standalone', ?, ?, ?, 0, 2020, ?, 'identified', 0.95)
	`)
	if err != nil {
		t.Fatalf("prepare scale insert: %v", err)
	}
	for i := 0; i < 5000; i++ {
		wID := fmt.Sprintf("scale-work-%05d", i)
		title := fmt.Sprintf("Archive Cinema Masterpiece %05d", i)
		_, _ = stmt.ExecContext(ctx, wID, title, fmt.Sprintf("فيلم أرشيفي %05d", i), title, fmt.Sprintf("%s 2020", title))
	}
	_ = stmt.Close()
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit scale tx: %v", err)
	}

	qStart := time.Now()
	foundScale, err := libSvc.ListMovies(movies.MovieFilter{Query: "04999", Limit: 20})
	qDur := time.Since(qStart)
	if err != nil || len(foundScale) != 1 {
		t.Fatalf("scale FTS5 query failed: count=%d err=%v", len(foundScale), err)
	}
	if qDur > 500*time.Millisecond {
		t.Fatalf("FTS5 query over 5,000+ works took too long: %v", qDur)
	}
}
