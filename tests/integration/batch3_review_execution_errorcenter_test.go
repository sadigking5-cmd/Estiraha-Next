package integration_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sadigking5-cmd/Estiraha-Next/internal/api"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/app"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/backup"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/config"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/db"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/errorsbox"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/library"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/organizer"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/review"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/scanner"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/secrets"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/tray"
)

func sha256OfFile(t *testing.T, p string) string {
	t.Helper()
	f, err := os.Open(p)
	if err != nil {
		t.Fatalf("open %s for sha256: %v", p, err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		t.Fatalf("read %s for sha256: %v", p, err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func snapshotAllRegularFiles(t *testing.T, roots ...string) map[string]int {
	t.Helper()
	countsByHash := map[string]int{}
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			countsByHash[sha256OfFile(t, p)]++
			return nil
		})
	}
	return countsByHash
}

func assertZeroFileLoss(t *testing.T, before, after map[string]int, stage string) {
	t.Helper()
	if len(before) != len(after) {
		t.Fatalf("[%s] zero-file-loss invariant failed: unique SHA-256 count before=%d after=%d", stage, len(before), len(after))
	}
	for h, wantCount := range before {
		if got := after[h]; got != wantCount {
			t.Fatalf("[%s] zero-file-loss invariant failed for hash %s: before=%d after=%d", stage, h[:12], wantCount, got)
		}
	}
}

func TestBatch3ReviewCenterExecutionManifestErrorCenterRecoveryAndRollbackAcceptance(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	tmp := t.TempDir()
	dbFile := filepath.Join(tmp, "batch3_acceptance.sqlite")
	backupDir := filepath.Join(tmp, "backups")

	driveAMovies := filepath.Join(tmp, "DriveA", "Movies")
	driveBArchive := filepath.Join(tmp, "DriveB", "MoviesMirror")
	for _, d := range []string{driveAMovies, driveBArchive} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	engine, err := db.Open(ctx, dbFile)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer engine.Close()

	ver, err := engine.Migrate(ctx, backupDir)
	if err != nil || ver < 6 {
		t.Fatalf("expected Migration 0006 (ver>=6), got ver=%d err=%v", ver, err)
	}

	vault := secrets.NewDefaultVault()
	libSvc := api.NewLibraryService(config.Default(), engine, vault, backupDir, nil)
	defer libSvc.Close()

	// Verify all 6 official managed Error Center categories are registered in canonical order
	errCats := libSvc.ListErrorFolderCategories()
	if len(errCats) != 6 || errCats[0].Code != errorsbox.CategoryEmptyFolders || errCats[5].Code != errorsbox.CategoryConflictsReview {
		t.Fatalf("expected 6 official Error Center categories, got %+v", errCats)
	}

	trayTracker := tray.NewStateTracker()
	pocSvc := api.NewPoCService(config.Default(), engine, vault, trayTracker, app.DetectWebView2(false), nil)

	// Criterion 12: Verify Arabic RTL (sidebar=right) & English LTR (sidebar=left) contract synchronization
	diagAr, err := pocSvc.SetActiveLocale("ar")
	if err != nil || diagAr.Direction != "rtl" || diagAr.SidebarPlacement != "right" {
		t.Fatalf("Criterion 12 failed for Arabic RTL: got dir=%q sidebar=%q err=%v", diagAr.Direction, diagAr.SidebarPlacement, err)
	}
	diagEn, err := pocSvc.SetActiveLocale("en")
	if err != nil || diagEn.Direction != "ltr" || diagEn.SidebarPlacement != "left" {
		t.Fatalf("Criterion 12 failed for English LTR: got dir=%q sidebar=%q err=%v", diagEn.Direction, diagEn.SidebarPlacement, err)
	}
	_, _ = pocSvc.SetActiveLocale("ar")

	// Synthesize real playable FFmpeg test videos in isolated temp dir
	seed1080 := filepath.Join(tmp, "seed_1080p.mp4")
	seed720 := filepath.Join(tmp, "seed_720p.mp4")
	generateTestVideoWithFFmpeg(t, seed1080, 2, 1920, 1080, "-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac")
	generateTestVideoWithFFmpeg(t, seed720, 2, 1280, 720, "-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac")

	jpegHeader := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01, 0x01, 0x00}
	appleDouble := append([]byte{0x00, 0x05, 0x16, 0x07, 0x00, 0x02, 0x00, 0x00}, bytes.Repeat([]byte("A"), 4088)...)

	// Movie A (Approved Bundle: Video + AR/EN Subtitles + Poster + Protected .shorts)
	m1Dir := filepath.Join(driveAMovies, "Gladiator.2000.1080p.Raw")
	remuxFeatureClip(t, seed1080, filepath.Join(m1Dir, "Gladiator.2000.1080p.BluRay.mp4"))
	mustWriteFile(t, filepath.Join(m1Dir, "Gladiator.2000.ar.srt"), []byte("1\n00:00:01,000 --> 00:00:03,000\nالمصارع\n"))
	mustWriteFile(t, filepath.Join(m1Dir, "Gladiator.2000.en.srt"), []byte("1\n00:00:01,000 --> 00:00:03,000\nAre you not entertained?\n"))
	mustWriteFile(t, filepath.Join(m1Dir, "poster.jpg"), append(append([]byte{}, jpegHeader...), []byte("GLADIATOR_GENUINE_POSTER_2000")...))
	shortsOrigPath := filepath.Join(m1Dir, "Gladiator.2000.shorts")
	mustWriteFile(t, shortsOrigPath, []byte("GLADIATOR_PROTECTED_SHORTS_CLIP_PAYLOAD"))
	shortsOrigHash := sha256OfFile(t, shortsOrigPath)

	// Cross-disk copy of Gladiator (2000) on DriveB (Criterion 10: Cross-disk copies preserved!)
	// Note: placed in an unorganized subfolder on DriveB AND kept unapproved in CinemaVaultB so CinemaVaultA organizes independently
	crossDiskPath := filepath.Join(driveBArchive, "MirrorRaw", "Gladiator.2000.720p.Mirror.mp4")
	remuxFeatureClip(t, seed720, crossDiskPath)
	crossDiskHash := sha256OfFile(t, crossDiskPath)

	// Movie B (Will be REJECTED by user -> Criterion 2: Rejected operations remain completely unchanged!)
	m2Dir := filepath.Join(driveAMovies, "Memento.2000.Raw")
	m2VidPath := filepath.Join(m2Dir, "Memento.2000.1080p.mp4")
	remuxFeatureClip(t, seed1080, m2VidPath)
	m2SubPath := filepath.Join(m2Dir, "Memento.2000.en.srt")
	mustWriteFile(t, m2SubPath, []byte("1\n00:00:01,000 --> 00:00:03,000\nRemember Sammy Jankis\n"))

	// Movie C (Will remain PENDING / UNAPPROVED -> Criterion 1: Unapproved operations NEVER execute!)
	m3Dir := filepath.Join(driveAMovies, "Heat.1995.Raw")
	m3VidPath := filepath.Join(m3Dir, "Heat.1995.720p.mp4")
	remuxFeatureClip(t, seed720, m3VidPath)

	// Managed Error Folder candidates across official categories (Criterion 6 & Error Center)
	emptyPreDir := filepath.Join(driveAMovies, "Empty_Source_Folder_Legacy")
	if err := os.MkdirAll(emptyPreDir, 0o755); err != nil {
		t.Fatal(err)
	}
	zeroBytePath := filepath.Join(driveAMovies, "broken_zero_file.mkv")
	mustWriteFile(t, zeroBytePath, []byte{})

	orphanSubDir := filepath.Join(driveAMovies, "LooseSubsOnly")
	orphanSubPath := filepath.Join(orphanSubDir, "unlinked_orphan.srt")
	mustWriteFile(t, orphanSubPath, []byte("1\n00:00:01,000 --> 00:00:02,000\nOrphan subtitle\n"))

	applePath := filepath.Join(m1Dir, "._Gladiator.2000.mp4")
	mustWriteFile(t, applePath, appleDouble)
	promoPath := filepath.Join(m1Dir, "WWW.YTS.MX.jpg")
	mustWriteFile(t, promoPath, append(append([]byte{}, jpegHeader...), []byte("YTS_PROMO_GLADIATOR")...))

	samplePath := filepath.Join(m1Dir, "Gladiator-sample.mp4")
	generateTestVideoWithFFmpeg(t, samplePath, 3, 640, 360, "-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac")

	// Unlinked `.shorts` at root (Criterion 9: `.shorts` NEVER moved to Samples or Errors!)
	looseShortsPath := filepath.Join(driveAMovies, "standalone_protected_clip.shorts")
	mustWriteFile(t, looseShortsPath, []byte("STANDALONE_SHORTS_MUST_NEVER_ENTER_ERRORS"))

	baselineSnapshot := snapshotAllRegularFiles(t, driveAMovies, driveBArchive)

	if _, err := libSvc.AddLibraryRoot(library.AddRootInput{
		FullPath:        driveAMovies,
		ContentCategory: "movies",
		DisplayName:     "الأفلام الرئيسية",
		DiskLabel:       "CinemaVaultA",
	}); err != nil {
		t.Fatalf("add root A: %v", err)
	}
	if _, err := libSvc.AddLibraryRoot(library.AddRootInput{
		FullPath:        driveBArchive,
		ContentCategory: "movies",
		DisplayName:     "مرآة الأرشيف",
		DiskLabel:       "CinemaVaultB",
	}); err != nil {
		t.Fatalf("add root B: %v", err)
	}

	scanRes, err := libSvc.RunManualScanAndAnalyze(scanner.ScanOptions{ComputeHash: true})
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	if scanRes.Duplicates.CrossDiskKeptGroups < 1 {
		t.Fatalf("Criterion 10: expected cross-disk duplicate group to be marked Keep, got %+v", scanRes.Duplicates)
	}

	batch, err := libSvc.GenerateOrganizationPlan("دفعة اختبار القبول الشامل — Batch 3")
	if err != nil {
		t.Fatalf("GenerateOrganizationPlan: %v", err)
	}

	// CRITERION 1A: Attempting to execute a batch with ZERO approved items MUST fail and change nothing
	if _, err := libSvc.ExecuteApprovedBatch(batch.ID); !errors.Is(err, organizer.ErrNoApprovedItems) {
		t.Fatalf("Criterion 1: expected ErrNoApprovedItems when 0 items approved, got %v", err)
	}
	assertZeroFileLoss(t, baselineSnapshot, snapshotAllRegularFiles(t, driveAMovies, driveBArchive), "after-blocked-unapproved-exec")

	items, err := libSvc.ListReviewBatchItems(batch.ID)
	if err != nil || len(items) == 0 {
		t.Fatalf("ListReviewBatchItems: len=%d err=%v", len(items), err)
	}

	var mementoIDs []string
	var gladiatorAndErrorIDs []string
	var heatIDs []string

	for _, it := range items {
		if it.ProtectedShortsFile && it.ActionKind == "move_to_errors" {
			t.Fatalf("Criterion 9 violation: .shorts item %s scheduled for move_to_errors", it.OldName)
		}

		if strings.Contains(it.OldRelPath, "Memento.2000") {
			mementoIDs = append(mementoIDs, it.ID)
		} else if strings.Contains(it.OldRelPath, "Heat.1995") {
			heatIDs = append(heatIDs, it.ID)
		} else if it.DiskLabel == "CinemaVaultA" && !it.ConflictDetected && (strings.Contains(it.OldRelPath, "Gladiator") || it.ActionKind == "move_to_errors") {
			gladiatorAndErrorIDs = append(gladiatorAndErrorIDs, it.ID)
		}
	}

	if len(mementoIDs) == 0 || len(heatIDs) == 0 || len(gladiatorAndErrorIDs) < 6 {
		t.Fatalf("unexpected fixture categorization: memento=%d heat=%d gladiator+err=%d", len(mementoIDs), len(heatIDs), len(gladiatorAndErrorIDs))
	}

	det, err := libSvc.GetReviewItemDetails(gladiatorAndErrorIDs[0])
	if err != nil || det.Item.OldFullPath == "" || det.Item.EffectiveFullPath == "" {
		t.Fatalf("GetReviewItemDetails failed: %+v err=%v", det, err)
	}

	// Verify `.shorts` extension stripping via manual edit is strictly rejected
	for _, it := range items {
		if it.ProtectedShortsFile && it.ActionKind != "needs_manual_review" {
			if _, sErr := libSvc.DecideReviewItem(review.ItemDecisionInput{
				ItemID:         it.ID,
				Decision:       "approved",
				UserEditedName: "Gladiator (2000).mp4",
			}); sErr == nil {
				t.Fatal("Criterion 9: expected stripping .shorts extension in manual edit to be rejected")
			}
		}
	}

	// CRITERION 2: Bulk Reject Memento items; leave Heat items PENDING (unapproved); Bulk Approve Gladiator + Errors
	rejRes, err := libSvc.BulkDecideReviewItems(review.BulkDecisionInput{
		BatchID:  batch.ID,
		ItemIDs:  mementoIDs,
		Decision: "rejected",
	})
	if err != nil || rejRes.UpdatedCount != len(mementoIDs) {
		t.Fatalf("Bulk reject failed: %+v err=%v", rejRes, err)
	}

	appRes, err := libSvc.BulkDecideReviewItems(review.BulkDecisionInput{
		BatchID:  batch.ID,
		ItemIDs:  gladiatorAndErrorIDs,
		Decision: "approved",
	})
	if err != nil || appRes.UpdatedCount != len(gladiatorAndErrorIDs) {
		t.Fatalf("Bulk approve failed: %+v err=%v", appRes, err)
	}

	// CRITERION 4: Build pre-execution manifest summary & verify it matches actual execution 100%
	previewManifest, err := libSvc.GetExecutionManifestSummary(batch.ID)
	if err != nil {
		t.Fatalf("GetExecutionManifestSummary: %v", err)
	}
	if previewManifest.TotalApprovedOperations != len(gladiatorAndErrorIDs) {
		t.Fatalf("expected %d approved steps in manifest, got %d", len(gladiatorAndErrorIDs), previewManifest.TotalApprovedOperations)
	}
	if previewManifest.SubtitlesToAssociate != 2 || previewManifest.PostersArtworkToAssociate != 1 || previewManifest.ShortsToPreserve != 2 {
		t.Fatalf("unexpected companion counters in manifest preview: subs=%d posters=%d shorts=%d",
			previewManifest.SubtitlesToAssociate, previewManifest.PostersArtworkToAssociate, previewManifest.ShortsToPreserve)
	}
	if previewManifest.EmptyFoldersToMoveErrors < 1 || previewManifest.FoldersToCreate < 1 || previewManifest.UnchangedFilesCount < 3 {
		t.Fatalf("unexpected folder/unchanged counters in manifest preview: %+v", previewManifest)
	}

	// CRITERION 5: Existing files are NEVER overwritten (collision guard check)
	heatItemID := heatIDs[0]
	blockerRel := "Movies/ExistingCollisionTarget/Heat (1995).mp4"
	blockerFull := filepath.Join(tmp, "DriveA", filepath.FromSlash(blockerRel))
	mustWriteFile(t, blockerFull, []byte("PRE_EXISTING_USER_FILE_DO_NOT_OVERWRITE"))
	blockerHash := sha256OfFile(t, blockerFull)

	if _, colErr := libSvc.DecideReviewItem(review.ItemDecisionInput{
		ItemID:            heatItemID,
		Decision:          "approved",
		UserEditedRelPath: blockerRel,
	}); colErr == nil {
		t.Fatal("Criterion 5: expected collision guard to block approving onto existing file")
	}
	_, _ = libSvc.DecideReviewItem(review.ItemDecisionInput{
		ItemID:            heatItemID,
		Decision:          "pending",
		UserEditedRelPath: "Movies/Heat (1995)/Heat (1995).mp4",
	})
	baselineSnapshot = snapshotAllRegularFiles(t, driveAMovies, driveBArchive)

	// CRITERION 8: Simulate mid-batch interruption + Pause/Resume + Crash Recovery!
	bSvc := backup.NewService(engine.SQL(), backupDir)
	directOrg := organizer.NewService(engine.SQL(), bSvc)

	var hookTriggered atomic.Bool
	directOrg.SetStepHook(func(stepIdx int, _ review.ManifestStepSpec) error {
		if stepIdx == 3 && !hookTriggered.Swap(true) {
			return errors.New("simulated sudden power/disk interruption at step 4")
		}
		return nil
	})

	partialExec, intErr := directOrg.ExecuteApprovedBatch(ctx, batch.ID)
	if intErr == nil {
		t.Fatal("Criterion 8: expected simulated interruption error on step 4")
	}
	if partialExec.ExecutedCount != 3 {
		t.Fatalf("Criterion 8: expected 3 steps completed before step 4 interruption, got %d", partialExec.ExecutedCount)
	}
	assertZeroFileLoss(t, baselineSnapshot, snapshotAllRegularFiles(t, driveAMovies, driveBArchive), "during-mid-batch-interruption")

	progAfterInt, _ := libSvc.GetBatchExecutionProgress(batch.ID)
	if progAfterInt.InterruptedEntries != 1 {
		t.Fatalf("Criterion 8: expected 1 interrupted journal entry, got %d", progAfterInt.InterruptedEntries)
	}

	recRes, err := libSvc.RecoverInterruptedBatch(batch.ID)
	if err != nil || recRes.RequeuedCount != 1 {
		t.Fatalf("Criterion 8: RecoverInterruptedBatch failed: %+v err=%v", recRes, err)
	}
	directOrg.SetStepHook(nil)

	// Test Pause & Resume on the remaining approved items
	var pausedOnce atomic.Bool
	directOrg.SetStepHook(func(stepIdx int, _ review.ManifestStepSpec) error {
		if stepIdx == 0 && !pausedOnce.Swap(true) {
			directOrg.PauseExecution()
			go func() {
				time.Sleep(80 * time.Millisecond)
				directOrg.ResumeExecution()
			}()
		}
		return nil
	})

	remManifest, err := libSvc.GetExecutionManifestSummary(batch.ID)
	if err != nil {
		t.Fatalf("GetExecutionManifestSummary remaining: %v", err)
	}
	execRes, err := directOrg.ExecuteApprovedBatch(ctx, batch.ID)
	if err != nil {
		t.Fatalf("Criterion 3 & 8: resume execution failed: %v", err)
	}
	directOrg.SetStepHook(nil)

	// CRITERION 4: Verify executed count across initial + resumed run equals exact approved manifest count!
	totalExecuted := partialExec.ExecutedCount + execRes.ExecutedCount
	if totalExecuted != previewManifest.TotalApprovedOperations {
		t.Fatalf("Criterion 4: preview approved=%d, actual executed=%d (remManifest=%d)",
			previewManifest.TotalApprovedOperations, totalExecuted, remManifest.TotalApprovedOperations)
	}
	if execRes.ManifestSHA256 != remManifest.ManifestSHA256 || execRes.PreBatchBackupID == "" {
		t.Fatalf("Criterion 4: manifest hash or pre-batch backup missing: %+v", execRes)
	}

	// Verify Criterion 1, 2, 5, 9, 10: Unapproved (Heat), Rejected (Memento), Blocker, Loose .shorts, Cross-Disk copy UNCHANGED!
	for _, untouchedPath := range []string{m2VidPath, m2SubPath, m3VidPath, looseShortsPath, blockerFull, crossDiskPath} {
		if _, err := os.Stat(untouchedPath); err != nil {
			t.Fatalf("Criterion 1/2/5/9/10 violation: expected untouched file %s to remain in original place: %v", untouchedPath, err)
		}
	}
	if sha256OfFile(t, blockerFull) != blockerHash {
		t.Fatal("Criterion 5 violation: pre-existing file hash altered!")
	}
	if sha256OfFile(t, crossDiskPath) != crossDiskHash {
		t.Fatal("Criterion 10 violation: cross-disk movie copy altered!")
	}

	// Verify Criterion 3 & 9: Gladiator bundle (Video + 2 Subtitles + Poster + Protected .shorts) moved cleanly
	// Official Estiraha Next movie naming law: `<CleanTitle> <Year>/<CleanTitle> <Year>.<ext>` (no parentheses!)
	gladCanonicalDir := filepath.Join(driveAMovies, "Gladiator 2000")
	for _, expectedName := range []string{
		"Gladiator 2000.mp4",
		"Gladiator 2000.ar.srt",
		"Gladiator 2000.en.srt",
		"poster.jpg",
		"Gladiator 2000.shorts",
	} {
		p := filepath.Join(gladCanonicalDir, expectedName)
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("Criterion 3/9: expected organized bundle file %s: %v", p, err)
		}
	}
	if sha256OfFile(t, filepath.Join(gladCanonicalDir, "Gladiator 2000.shorts")) != shortsOrigHash {
		t.Fatal("Criterion 9: Gladiator .shorts SHA-256 mismatch after move!")
	}

	assertZeroFileLoss(t, baselineSnapshot, snapshotAllRegularFiles(t, driveAMovies, driveBArchive), "after-approved-execution")

	// CRITERION 6: Error Center Inspection, Individual Restore, Selected Restore, and Complete Batch Restore!
	errEntries, err := libSvc.ListErrorFolderEntries("all")
	if err != nil || len(errEntries) < 5 {
		t.Fatalf("Criterion 6: expected >= 5 quarantined error entries, got %d err=%v", len(errEntries), err)
	}
	inspected, err := libSvc.InspectErrorFolderEntry(errEntries[0].ID)
	if err != nil || inspected.OriginalFullPath == "" || inspected.CategoryFolderAr == "" {
		t.Fatalf("Criterion 6: InspectErrorFolderEntry failed: %+v err=%v", inspected, err)
	}

	var orphanEntryID string
	var selectedPair []string
	for _, ev := range errEntries {
		if ev.EntryStatus != "active" {
			continue
		}
		if ev.ItemName == "unlinked_orphan.srt" {
			orphanEntryID = ev.ID
		} else if len(selectedPair) < 2 && ev.EntryKind == "file" {
			selectedPair = append(selectedPair, ev.ID)
		}
	}
	if orphanEntryID == "" || len(selectedPair) < 2 {
		t.Fatalf("Criterion 6: missing expected error entries for restore tests: orphan=%q pair=%v", orphanEntryID, selectedPair)
	}

	if _, err := libSvc.RestoreErrorFolderEntry(orphanEntryID); err != nil {
		t.Fatalf("Criterion 6A: individual error restore failed: %v", err)
	}
	if _, err := os.Stat(orphanSubPath); err != nil {
		t.Fatalf("Criterion 6A: expected %s restored to original location: %v", orphanSubPath, err)
	}

	selSum, err := libSvc.RestoreSelectedErrorFolderEntries(selectedPair)
	if err != nil || selSum.RestoredCount != 2 {
		t.Fatalf("Criterion 6B: RestoreSelectedErrorFolderEntries failed: %+v err=%v", selSum, err)
	}

	allActiveAfterSel, _ := libSvc.ListErrorFolderEntries("all")
	var targetErrBatchID string
	for _, ev := range allActiveAfterSel {
		if ev.EntryStatus == "active" && ev.BatchID != "" {
			targetErrBatchID = ev.BatchID
			break
		}
	}
	if targetErrBatchID != "" {
		batchSum, err := libSvc.RestoreErrorFolderBatch(targetErrBatchID)
		if err != nil || batchSum.RestoredCount < 1 {
			t.Fatalf("Criterion 6C: RestoreErrorFolderBatch failed: %+v err=%v", batchSum, err)
		}
	}
	assertZeroFileLoss(t, baselineSnapshot, snapshotAllRegularFiles(t, driveAMovies, driveBArchive), "after-error-center-restores")

	// CRITERION 7: Full Verified Rollback of Batch restores exact original paths & SHA-256 hashes!
	rbRes, err := libSvc.RollbackBatch(batch.ID)
	if err != nil || rbRes.BlockedCount != 0 || rbRes.RestoredCount < 5 {
		t.Fatalf("Criterion 7: RollbackBatch failed: %+v err=%v", rbRes, err)
	}

	if _, err := os.Stat(shortsOrigPath); err != nil {
		t.Fatalf("Criterion 7: expected %s restored by rollback: %v", shortsOrigPath, err)
	}
	if sha256OfFile(t, shortsOrigPath) != shortsOrigHash {
		t.Fatal("Criterion 7: SHA-256 mismatch on restored .shorts after rollback!")
	}
	if _, err := os.Stat(filepath.Join(m1Dir, "Gladiator.2000.1080p.BluRay.mp4")); err != nil {
		t.Fatalf("Criterion 7: expected Gladiator video restored to original path: %v", err)
	}
	if _, err := os.Stat(emptyPreDir); err != nil {
		t.Fatalf("Criterion 7: expected Empty_Source_Folder_Legacy restored: %v", err)
	}

	assertZeroFileLoss(t, baselineSnapshot, snapshotAllRegularFiles(t, driveAMovies, driveBArchive), "after-complete-rollback")
}
