package integration_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sadigking5-cmd/Estiraha-Next/internal/api"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/config"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/db"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/errorsbox"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/library"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/movies"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/review"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/scanner"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/secrets"
)

func remuxFeatureClip(t *testing.T, seedPath, outPath string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", outPath, err)
	}
	cmd := exec.Command("ffmpeg", "-y", "-hide_banner", "-loglevel", "error", "-stream_loop", "59", "-i", seedPath, "-c", "copy", outPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("remuxFeatureClip %s failed: %v (%s)", outPath, err, string(out))
	}
}

func TestBatch2MovieBundlingSubtitlesPostersShortsMultiVersionEmptyFoldersAndOrganizerRollback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	tmp := t.TempDir()
	dbFile := filepath.Join(tmp, "batch2_bundling.sqlite")
	backupDir := filepath.Join(tmp, "backups")

	diskARoot := filepath.Join(tmp, "DriveA", "Movies")
	diskBRoot := filepath.Join(tmp, "DriveB", "Archive")

	for _, d := range []string{diskARoot, diskBRoot} {
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
	if err != nil || ver != 5 {
		t.Fatalf("expected Migration 0005 (ver=5), got ver=%d err=%v", ver, err)
	}

	vault := secrets.NewDefaultVault()
	libSvc := api.NewLibraryService(config.Default(), engine, vault, backupDir, nil)
	defer libSvc.Close()

	seed720 := filepath.Join(tmp, "seed_720p_2s.mp4")
	seed1080 := filepath.Join(tmp, "seed_1080p_2s.mp4")
	generateTestVideoWithFFmpeg(t, seed720, 2, 1280, 720, "-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac")
	generateTestVideoWithFFmpeg(t, seed1080, 2, 1920, 1080, "-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac")

	jpegHeader := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01, 0x01, 0x00}
	appleDouble := append([]byte{0x00, 0x05, 0x16, 0x07, 0x00, 0x02, 0x00, 0x00}, bytes.Repeat([]byte("R"), 4088)...)

	// 1. `15 Minutes (2001)`
	f15Dir := filepath.Join(diskARoot, "15 Minutes (2001)")
	remuxFeatureClip(t, seed1080, filepath.Join(f15Dir, "15.Minutes.2001.1080p.BluRay.mp4"))
	mustWriteFile(t, filepath.Join(f15Dir, "._15.Minutes.2001.srt"), appleDouble)
	mustWriteFile(t, filepath.Join(f15Dir, "15.Minutes.2001.en.srt"), []byte("1\n00:00:01,000 --> 00:00:03,000\nEnglish subtitle for 15 Minutes\n"))
	mustWriteFile(t, filepath.Join(f15Dir, "15.Minutes.2001.ar.srt"), []byte("1\n00:00:01,000 --> 00:00:03,000\nترجمة عربية لفيلم 15 دقيقة\n"))
	mustWriteFile(t, filepath.Join(f15Dir, "._folder.jpg"), appleDouble)
	mustWriteFile(t, filepath.Join(f15Dir, "folder.jpg"), append(append([]byte{}, jpegHeader...), []byte("POSTER_15_MINUTES")...))
	mustWriteFile(t, filepath.Join(f15Dir, "15.Minutes.2001.shorts"), []byte("PROTECTED_SHORTS_15_MINUTES"))

	// 2. `21 Grams (2003)`
	f21Dir := filepath.Join(diskARoot, "21 Grams (2003)")
	remuxFeatureClip(t, seed1080, filepath.Join(f21Dir, "21.Grams.2003.1080p.BrRip.mp4"))
	mustWriteFile(t, filepath.Join(f21Dir, "Subs", "English.srt"), []byte("1\n00:00:01,000 --> 00:00:03,000\n21 Grams standard English\n"))
	mustWriteFile(t, filepath.Join(f21Dir, "Subs", "English.SDH.srt"), []byte("1\n00:00:01,000 --> 00:00:03,000\n[sighs] 21 Grams SDH English\n"))
	mustWriteFile(t, filepath.Join(f21Dir, "Subs", "Spanish.srt"), []byte("1\n00:00:01,000 --> 00:00:03,000\n21 Gramos Español\n"))
	mustWriteFile(t, filepath.Join(f21Dir, "WWW.YIFY-TORRENTS.COM.jpg"), append(append([]byte{}, jpegHeader...), []byte("YIFY_PROMO_BANNER_21G")...))
	mustWriteFile(t, filepath.Join(f21Dir, "poster.jpg"), append(append([]byte{}, jpegHeader...), []byte("GENUINE_POSTER_21_GRAMS_HIGHER_SIZE_1234567890")...))
	mustWriteFile(t, filepath.Join(f21Dir, "backdrop.jpg"), append(append([]byte{}, jpegHeader...), []byte("GENUINE_BACKDROP_21_GRAMS")...))

	// 3. `[ www.UsaBit.com ] - 3000 Miles To Graceland (2001)`
	f3000Dir := filepath.Join(diskARoot, "[ www.UsaBit.com ] - 3000 Miles To Graceland (2001)")
	remuxFeatureClip(t, seed720, filepath.Join(f3000Dir, "3000.Miles.to.Graceland.2001.720p.WEB-DL.mp4"))
	mustWriteFile(t, filepath.Join(f3000Dir, "مجلد جديد", "Arabic.srt"), []byte("1\n00:00:01,000 --> 00:00:03,000\nترجمة 3000 ميل إلى غريسلاند\n"))
	mustWriteFile(t, filepath.Join(f3000Dir, "Torrent downloaded from Demonoid.ph.txt"), []byte("Torrent downloaded from Demonoid.ph"))

	// 4. `Atonement (2007)`
	fAtoneDir := filepath.Join(diskARoot, "Atonement (2007)")
	remuxFeatureClip(t, seed1080, filepath.Join(fAtoneDir, "Atonement.2007.1080p.mp4"))
	mustWriteFile(t, filepath.Join(fAtoneDir, "Subs", "Estonian.sub"), []byte("VOBSUB_BITMAP_PAYLOAD_ATONEMENT_ET"))
	mustWriteFile(t, filepath.Join(fAtoneDir, "Subs", "Estonian.idx"), []byte("# VobSub index file, v7\nid: et, index: 0\n"))
	mustWriteFile(t, filepath.Join(fAtoneDir, "Subs", "Atonement.2007.ar.ass"), []byte("[Script Info]\nTitle: Atonement Arabic\n[Events]\nDialogue: 0,0:00:01.00,0:00:03.00,Default,,0,0,0,,تكفير\n"))

	// 5. `Australia (2008)`
	fAusDir := filepath.Join(diskARoot, "Australia (2008)")
	remuxFeatureClip(t, seed720, filepath.Join(fAusDir, "Australia.2008.720p.mp4"))
	mustWriteFile(t, filepath.Join(fAusDir, "WWW.YTS.MX.jpg"), append(append([]byte{}, jpegHeader...), []byte("PROMO_ONLY_BANNER_AUSTRALIA")...))
	mustWriteFile(t, filepath.Join(fAusDir, "Australia.2008.en.srt"), []byte("1\n00:00:01,000 --> 00:00:03,000\nWelcome to Australia\n"))

	// 6. `Bandidas (2006)`
	fBandDir := filepath.Join(diskARoot, "Bandidas (2006)")
	remuxFeatureClip(t, seed720, filepath.Join(fBandDir, "Bandidas.2006.720p.mp4"))
	mustWriteFile(t, filepath.Join(fBandDir, "._Bandidas.srt"), appleDouble)
	mustWriteFile(t, filepath.Join(fBandDir, "Bandidas.ar.srt"), []byte("1\n00:00:01,000 --> 00:00:03,000\nبانديداس عربي\n"))
	mustWriteFile(t, filepath.Join(fBandDir, "Bandidas.fr.srt"), []byte("1\n00:00:01,000 --> 00:00:03,000\nBandidas Français\n"))
	mustWriteFile(t, filepath.Join(fBandDir, "cover.jpg"), append(append([]byte{}, jpegHeader...), []byte("COVER_BANDIDAS")...))
	mustWriteFile(t, filepath.Join(fBandDir, "Thumbs.db"), []byte("THUMBS_DB_CACHE_BANDIDAS"))
	mustWriteFile(t, filepath.Join(fBandDir, "desktop.ini"), []byte("[.ShellClassInfo]\r\n"))

	// 7. `Basic (2003)`
	fBasicDir := filepath.Join(diskARoot, "Basic (2003)")
	remuxFeatureClip(t, seed1080, filepath.Join(fBasicDir, "Basic (2003) 1080p.mp4"))
	mustWriteFile(t, filepath.Join(fBasicDir, ".@__thumb", "s800Basic (2003).mp4"), []byte("QNAP_PROXY_THUMB"))
	mustWriteFile(t, filepath.Join(fBasicDir, "Other", "ENG_Basic_2003.srt"), []byte("1\n00:00:01,000 --> 00:00:03,000\nBasic English Subtitle\n"))
	mustWriteFile(t, filepath.Join(fBasicDir, "director_analysis_notes.txt"), []byte("Important user notes about John McTiernan's Basic (2003) timeline structure."))

	// 8. `CJ7 (2008)`
	fCJ7Dir := filepath.Join(diskARoot, "CJ7 (2008)")
	remuxFeatureClip(t, seed1080, filepath.Join(fCJ7Dir, "CJ7.2008.1080p.BluRay.mp4"))
	mustWriteFile(t, filepath.Join(fCJ7Dir, "sub", "Chinese.srt"), []byte("1\n00:00:01,000 --> 00:00:03,000\n长江七号\n"))
	mustWriteFile(t, filepath.Join(fCJ7Dir, "sub", "Arabic.srt"), []byte("1\n00:00:01,000 --> 00:00:03,000\nسي جي 7\n"))
	mustWriteFile(t, filepath.Join(fCJ7Dir, "CJ7.2008.shorts"), []byte("PROTECTED_SHORTS_CJ7"))

	// 9. `Fish Tank (2009)`
	fFishDir := filepath.Join(diskARoot, "Fish Tank (2009)")
	remuxFeatureClip(t, seed720, filepath.Join(fFishDir, "Fish.Tank.2009.720p.mp4"))
	generateTestVideoWithFFmpeg(t, filepath.Join(fFishDir, "Fish.Tank.2009.sample.mp4"), 2, 640, 360, "-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac")
	mustWriteFile(t, filepath.Join(fFishDir, "Fish.Tank.2009.en.srt"), []byte("1\n00:00:01,000 --> 00:00:03,000\nFish Tank genuine subtitle\n"))
	mustWriteFile(t, filepath.Join(fFishDir, "Inception.2010.en.srt"), []byte("1\n00:00:01,000 --> 00:00:03,000\nMismatched foreign movie subtitle!\n"))

	// 10. `Harsh Times (2005)`
	remuxFeatureClip(t, seed1080, filepath.Join(diskARoot, "Harsh.Times.2005.1080p.mp4"))
	mustWriteFile(t, filepath.Join(diskARoot, "Harsh.Times.2005.1080p.ar.srt"), []byte("1\n00:00:01,000 --> 00:00:03,000\nأوقات قاسية\n"))
	mustWriteFile(t, filepath.Join(diskARoot, "Harsh.Times.2005.1080p.shorts"), []byte("PROTECTED_LOOSE_SHORTS_HARSH_TIMES"))
	mustWriteFile(t, filepath.Join(diskARoot, "zero_corrupt_sub.srt"), []byte{})
	mustWriteFile(t, filepath.Join(diskARoot, "Totally.Unlinked.Orphan.1995.srt"), []byte("1\n00:00:01,000 --> 00:00:02,000\nOrphan root subtitle\n"))
	mustWriteFile(t, filepath.Join(diskARoot, "mystery_unmatched_clip.shorts"), []byte("UNCERTAIN_UNMATCHED_SHORTS_NEVER_SAMPLE_OR_ERRORS"))

	// 11. `Gone in 60 Seconds (2000)`
	fGone720Dir := filepath.Join(diskARoot, "gone in 60 seconds 2000")
	fGone1080Dir := filepath.Join(diskARoot, "Gone in Sixty Seconds (2000) 1080p")
	fGoneDiskBDir := filepath.Join(diskBRoot, "Gone.in.60.Seconds.2000.720p.Mirror")
	remuxFeatureClip(t, seed720, filepath.Join(fGone720Dir, "gone.in.60.seconds.2000.720p.x264.mp4"))
	remuxFeatureClip(t, seed1080, filepath.Join(fGone1080Dir, "Gone.in.Sixty.Seconds.2000.1080p.BluRay.x264.mp4"))
	mustWriteFile(t, filepath.Join(fGone1080Dir, "Gone.in.Sixty.Seconds.2000.en.srt"), []byte("1\n00:00:01,000 --> 00:00:03,000\nEleanor GT500\n"))
	remuxFeatureClip(t, seed720, filepath.Join(fGoneDiskBDir, "Gone.in.60.Seconds.2000.720p.BluRay.mp4"))

	// 12. Pre-existing completely empty folder `Capote (2005)` on DiskA
	preEmptyDir := filepath.Join(diskARoot, "Capote (2005)")
	if err := os.MkdirAll(preEmptyDir, 0o755); err != nil {
		t.Fatal(err)
	}

	snapDiskABefore := snapshotDirSHA256(t, diskARoot)
	snapDiskBBefore := snapshotDirSHA256(t, diskBRoot)

	if _, err := libSvc.AddLibraryRoot(library.AddRootInput{
		FullPath:        diskARoot,
		ContentCategory: "movies",
		DisplayName:     "Primary Movies DiskA",
		DiskLabel:       "DiskA",
	}); err != nil {
		t.Fatalf("AddLibraryRoot DiskA: %v", err)
	}
	if _, err := libSvc.AddLibraryRoot(library.AddRootInput{
		FullPath:        diskBRoot,
		ContentCategory: "movies",
		DisplayName:     "Mirror Movies DiskB",
		DiskLabel:       "DiskB",
	}); err != nil {
		t.Fatalf("AddLibraryRoot DiskB: %v", err)
	}

	scanRes, err := libSvc.RunManualScanAndAnalyze(scanner.ScanOptions{ComputeHash: true})
	if err != nil {
		t.Fatalf("RunManualScanAndAnalyze: %v", err)
	}
	if scanRes.Scan.ThumbDirsSkipped < 1 {
		t.Fatalf("expected QNAP .@__thumb dir to be skipped, got %d", scanRes.Scan.ThumbDirsSkipped)
	}

	assertSnapshotsIdentical(t, snapDiskABefore, snapshotDirSHA256(t, diskARoot))
	assertSnapshotsIdentical(t, snapDiskBBefore, snapshotDirSHA256(t, diskBRoot))

	allMovies, err := libSvc.ListMovies(movies.MovieFilter{Limit: 100})
	if err != nil {
		t.Fatalf("ListMovies: %v", err)
	}
	if len(allMovies) != 11 {
		var gotNames []string
		for _, m := range allMovies {
			gotNames = append(gotNames, fmt.Sprint(m["canonicalFolderName"]))
		}
		t.Fatalf("expected exactly 11 canonical movies, got %d: %v", len(allMovies), gotNames)
	}

	movieByCanon := map[string]map[string]any{}
	for _, m := range allMovies {
		movieByCanon[fmt.Sprint(m["canonicalFolderName"])] = m
	}

	expectedCanonicalFolders := []string{
		"15 Minutes 2001",
		"21 Grams 2003",
		"3000 Miles to Graceland 2001",
		"Atonement 2007",
		"Australia 2008",
		"Bandidas 2006",
		"Basic 2003",
		"CJ7 2008",
		"Fish Tank 2009",
		"Harsh Times 2005",
		"Gone in 60 Seconds 2000",
	}
	for _, wantName := range expectedCanonicalFolders {
		if _, ok := movieByCanon[wantName]; !ok {
			var keys []string
			for k := range movieByCanon {
				keys = append(keys, k)
			}
			t.Fatalf("missing expected canonical movie %q; got: %v", wantName, keys)
		}
	}

	var poster21Name string
	_ = engine.SQL().QueryRowContext(ctx, `
		SELECT mf.filename FROM works w JOIN media_files mf ON mf.id = w.poster_rel_path
		WHERE w.canonical_folder_name = '21 Grams 2003'
	`).Scan(&poster21Name)
	if poster21Name != "poster.jpg" {
		t.Fatalf("expected 21 Grams primary poster 'poster.jpg', got %q", poster21Name)
	}

	var poster15Name string
	_ = engine.SQL().QueryRowContext(ctx, `
		SELECT mf.filename FROM works w JOIN media_files mf ON mf.id = w.poster_rel_path
		WHERE w.canonical_folder_name = '15 Minutes 2001'
	`).Scan(&poster15Name)
	if poster15Name != "folder.jpg" {
		t.Fatalf("expected 15 Minutes primary poster 'folder.jpg', got %q", poster15Name)
	}

	ausCard := movieByCanon["Australia 2008"]
	if fmt.Sprint(ausCard["posterFileId"]) != "" || fmt.Sprint(ausCard["posterUrl"]) != "" {
		t.Fatalf("Australia 2008 only had YTS promo banner; expected empty posterFileId & posterUrl, got id=%v url=%v", ausCard["posterFileId"], ausCard["posterUrl"])
	}

	goneCard := movieByCanon["Gone in 60 Seconds 2000"]
	if fmt.Sprint(goneCard["versionCount"]) != "3" {
		t.Fatalf("expected 3 grouped versions for Gone in 60 Seconds 2000, got %v", goneCard["versionCount"])
	}
	dups, err := libSvc.ListDuplicateGroups()
	if err != nil || len(dups) != 1 {
		t.Fatalf("expected 1 duplicate group for Gone in 60 Seconds 2000, got %d (err=%v)", len(dups), err)
	}
	if !dups[0].CrossDiskKeep {
		t.Fatalf("expected CrossDiskKeep=true when DiskB holds a mirror copy")
	}
	best1080 := false
	altSameDisk := false
	crossDiskKeep := false
	for _, mem := range dups[0].Members {
		if mem.IsSelectedBest && mem.ResolutionLabel == "1080p" {
			best1080 = true
		}
		if mem.ProposedAction == "propose_move_to_errors" {
			altSameDisk = true
		}
		if mem.ProposedAction == "keep_other_disk" {
			crossDiskKeep = true
		}
	}
	if !best1080 || !altSameDisk || !crossDiskKeep {
		t.Fatalf("multi-version scoring mismatch: best1080=%v altSameDisk=%v crossDisk=%v", best1080, altSameDisk, crossDiskKeep)
	}

	planBatch, err := libSvc.GenerateOrganizationPlan("Batch 2 Integration Verification Plan")
	if err != nil || planBatch.TotalItems == 0 {
		t.Fatalf("GenerateOrganizationPlan failed: total=%d err=%v", planBatch.TotalItems, err)
	}

	planItems, err := libSvc.ListReviewBatchItems(planBatch.ID)
	if err != nil {
		t.Fatalf("ListReviewBatchItems: %v", err)
	}

	shortsBundled := 0
	mysteryShortsReview := false
	foundMismatchedInceptionReview := false
	foundEmptyCapoteDirMove := false
	foundEvacuatedDirMove := false

	for _, it := range planItems {
		if strings.HasSuffix(strings.ToLower(it.OldName), ".shorts") {
			if it.ActionKind == "move_to_errors" || it.ErrorCategoryCode != "" {
				t.Fatalf("CRITICAL RULE VIOLATION: .shorts file %q routed to move_to_errors (%s)", it.OldName, it.ErrorCategoryCode)
			}
			if it.OldName == "mystery_unmatched_clip.shorts" {
				if it.ActionKind != "needs_manual_review" {
					t.Fatalf("expected unmatched .shorts to require manual review, got %q", it.ActionKind)
				}
				mysteryShortsReview = true
			} else {
				if !it.ProtectedShortsFile || !strings.HasSuffix(it.ProposedName, ".shorts") {
					t.Fatalf("expected matched .shorts %q to keep .shorts extension & ProtectedShortsFile=true, got proposed=%q", it.OldName, it.ProposedName)
				}
				shortsBundled++
			}
		}
		if it.OldName == "Inception.2010.en.srt" {
			if it.ActionKind != "move_to_errors" || it.ErrorCategoryCode != string(errorsbox.CategoryConflictsReview) || !it.ConflictDetected {
				t.Fatalf("expected foreign mismatched Inception.2010.en.srt inside Fish Tank folder to route to conflicts_needing_review with conflict=true, got action=%q cat=%q conflict=%v", it.ActionKind, it.ErrorCategoryCode, it.ConflictDetected)
			}
			foundMismatchedInceptionReview = true
		}
		if it.ItemFileKind == "directory" && it.OldName == "Capote (2005)" {
			if it.ActionKind != "move_to_errors" || it.ErrorCategoryCode != string(errorsbox.CategoryEmptyFolders) {
				t.Fatalf("expected pre-existing empty folder Capote (2005) -> move_to_errors/empty_folders, got %q/%q", it.ActionKind, it.ErrorCategoryCode)
			}
			foundEmptyCapoteDirMove = true
		}
		if it.ItemFileKind == "directory" && it.OldName == "15 Minutes (2001)" {
			foundEvacuatedDirMove = true
		}
		if it.ItemFileKind == "directory" && it.OldName == "Basic (2003)" {
			t.Fatalf("SAFETY VIOLATION: Basic (2003) contains user's director_analysis_notes.txt and MUST NOT be planned as an empty folder!")
		}
	}

	if shortsBundled != 3 || !mysteryShortsReview {
		t.Fatalf("expected 3 bundled .shorts and mystery_unmatched_clip.shorts in manual review, got bundled=%d mystery=%v", shortsBundled, mysteryShortsReview)
	}
	if !foundMismatchedInceptionReview {
		t.Fatal("expected mismatched Inception.2010.en.srt to be caught as conflicts_needing_review")
	}
	if !foundEmptyCapoteDirMove || !foundEvacuatedDirMove {
		t.Fatalf("expected both pre-existing empty Capote (2005) and evacuated 15 Minutes (2001) folder items: capote=%v evacuated=%v", foundEmptyCapoteDirMove, foundEvacuatedDirMove)
	}

	afterApprove, err := libSvc.ApproveAllSafeReviewItems(planBatch.ID)
	if err != nil || afterApprove.ApprovedItems == 0 {
		t.Fatalf("ApproveAllSafeReviewItems failed: approved=%d err=%v", afterApprove.ApprovedItems, err)
	}

	for _, it := range planItems {
		if it.OldName == "Inception.2010.en.srt" || it.OldName == "Totally.Unlinked.Orphan.1995.srt" {
			if _, dErr := libSvc.DecideReviewItem(review.ItemDecisionInput{
				ItemID:   it.ID,
				Decision: "approved",
			}); dErr != nil {
				t.Fatalf("explicitly approve conflict item %s: %v", it.OldName, dErr)
			}
		}
	}

	execRes, err := libSvc.ExecuteApprovedBatch(planBatch.ID)
	if err != nil {
		t.Fatalf("ExecuteApprovedBatch failed: %v (errs=%v)", err, execRes.Errors)
	}
	if len(execRes.Errors) > 0 || execRes.AbortedCount > 0 {
		t.Fatalf("expected 0 execution errors and 0 aborted items, got aborted=%d errors=%v", execRes.AbortedCount, execRes.Errors)
	}
	if execRes.PreBatchBackupID == "" {
		t.Fatal("expected automatic verified PreBatchBackupID before execution")
	}

	mustExistOrganized := []string{
		filepath.Join(diskARoot, "15 Minutes 2001", "15 Minutes 2001.mp4"),
		filepath.Join(diskARoot, "15 Minutes 2001", "15 Minutes 2001.en.srt"),
		filepath.Join(diskARoot, "15 Minutes 2001", "15 Minutes 2001.ar.srt"),
		filepath.Join(diskARoot, "15 Minutes 2001", "poster.jpg"),
		filepath.Join(diskARoot, "15 Minutes 2001", "15 Minutes 2001.shorts"),

		filepath.Join(diskARoot, "21 Grams 2003", "21 Grams 2003.mp4"),
		filepath.Join(diskARoot, "21 Grams 2003", "21 Grams 2003.en.srt"),
		filepath.Join(diskARoot, "21 Grams 2003", "21 Grams 2003.en.2.srt"),
		filepath.Join(diskARoot, "21 Grams 2003", "21 Grams 2003.es.srt"),
		filepath.Join(diskARoot, "21 Grams 2003", "poster.jpg"),
		filepath.Join(diskARoot, "21 Grams 2003", "21 Grams 2003 - artwork-2.jpg"),

		filepath.Join(diskARoot, "3000 Miles to Graceland 2001", "3000 Miles to Graceland 2001.mp4"),
		filepath.Join(diskARoot, "3000 Miles to Graceland 2001", "3000 Miles to Graceland 2001.ar.srt"),

		filepath.Join(diskARoot, "Atonement 2007", "Atonement 2007.mp4"),
		filepath.Join(diskARoot, "Atonement 2007", "Atonement 2007.et.sub"),
		filepath.Join(diskARoot, "Atonement 2007", "Atonement 2007.et.idx"),
		filepath.Join(diskARoot, "Atonement 2007", "Atonement 2007.ar.ass"),

		filepath.Join(diskARoot, "Australia 2008", "Australia 2008.mp4"),
		filepath.Join(diskARoot, "Australia 2008", "Australia 2008.en.srt"),

		filepath.Join(diskARoot, "Bandidas 2006", "Bandidas 2006.mp4"),
		filepath.Join(diskARoot, "Bandidas 2006", "Bandidas 2006.ar.srt"),
		filepath.Join(diskARoot, "Bandidas 2006", "Bandidas 2006.fr.srt"),
		filepath.Join(diskARoot, "Bandidas 2006", "poster.jpg"),

		filepath.Join(diskARoot, "Basic 2003", "Basic 2003.mp4"),
		filepath.Join(diskARoot, "Basic 2003", "Basic 2003.en.srt"),
		filepath.Join(diskARoot, "Basic (2003)", "director_analysis_notes.txt"),

		filepath.Join(diskARoot, "CJ7 2008", "CJ7 2008.mp4"),
		filepath.Join(diskARoot, "CJ7 2008", "CJ7 2008.zh.srt"),
		filepath.Join(diskARoot, "CJ7 2008", "CJ7 2008.ar.srt"),
		filepath.Join(diskARoot, "CJ7 2008", "CJ7 2008.shorts"),

		filepath.Join(diskARoot, "Fish Tank 2009", "Fish Tank 2009.mp4"),
		filepath.Join(diskARoot, "Fish Tank 2009", "Fish Tank 2009.en.srt"),

		filepath.Join(diskARoot, "Harsh Times 2005", "Harsh Times 2005.mp4"),
		filepath.Join(diskARoot, "Harsh Times 2005", "Harsh Times 2005.ar.srt"),
		filepath.Join(diskARoot, "Harsh Times 2005", "Harsh Times 2005.shorts"),
		filepath.Join(diskARoot, "mystery_unmatched_clip.shorts"),

		filepath.Join(diskARoot, "Gone in 60 Seconds 2000", "Gone in 60 Seconds 2000.mp4"),
		filepath.Join(diskARoot, "Gone in 60 Seconds 2000", "Gone in 60 Seconds 2000 [720p].mp4"),
		filepath.Join(diskARoot, "Gone in 60 Seconds 2000", "Gone in 60 Seconds 2000.en.srt"),
		filepath.Join(diskBRoot, "Gone in 60 Seconds 2000", "Gone in 60 Seconds 2000.mp4"),
	}

	for _, p := range mustExistOrganized {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected organized path %s to exist: %v", p, err)
		}
	}

	if _, err := os.Stat(filepath.Join(diskARoot, "Australia 2008", "poster.jpg")); !os.IsNotExist(err) {
		t.Fatal("Australia 2008 must NOT have poster.jpg created from YTS promo banner")
	}

	for _, shouldBeGoneFromRoot := range []string{
		filepath.Join(diskARoot, "15 Minutes (2001)"),
		filepath.Join(diskARoot, "21 Grams (2003)"),
		filepath.Join(diskARoot, "[ www.UsaBit.com ] - 3000 Miles To Graceland (2001)"),
		filepath.Join(diskARoot, "Atonement (2007)"),
		filepath.Join(diskARoot, "Australia (2008)"),
		filepath.Join(diskARoot, "Bandidas (2006)"),
		filepath.Join(diskARoot, "CJ7 (2008)"),
		filepath.Join(diskARoot, "Fish Tank (2009)"),
		filepath.Join(diskARoot, "Gone in Sixty Seconds (2000) 1080p"),
		filepath.Join(diskARoot, "gone in 60 seconds 2000"),
		filepath.Join(diskARoot, "Capote (2005)"),
	} {
		if _, err := os.Stat(shouldBeGoneFromRoot); !os.IsNotExist(err) {
			t.Fatalf("expected evacuated/empty folder %s to be relocated to الأخطاء/مجلدات فارغة, still exists at root!", shouldBeGoneFromRoot)
		}
	}

	errEntries, err := libSvc.ListErrorFolderEntries("all")
	if err != nil || len(errEntries) == 0 {
		t.Fatalf("expected active entries in الأخطاء/, got %d (err=%v)", len(errEntries), err)
	}
	catCounts := map[errorsbox.CategoryCode]int{}
	for _, e := range errEntries {
		if e.EntryStatus == "active" {
			catCounts[e.CategoryCode]++
		}
		if strings.HasSuffix(strings.ToLower(e.ItemName), ".shorts") {
			t.Fatalf("CRITICAL VIOLATION: found .shorts inside الأخطاء/: %+v", e)
		}
	}
	for _, mustHaveCat := range []errorsbox.CategoryCode{
		errorsbox.CategoryEmptyFolders,
		errorsbox.CategoryZeroByteFiles,
		errorsbox.CategoryUnlinkedSubtitles,
		errorsbox.CategoryAuxiliaryUnknown,
		errorsbox.CategorySamplesShortClips,
		errorsbox.CategoryConflictsReview,
	} {
		if catCounts[mustHaveCat] < 1 {
			t.Fatalf("expected at least 1 active entry in category %s, got counts=%v", mustHaveCat, catCounts)
		}
	}

	rbRes, err := libSvc.RollbackBatch(planBatch.ID)
	if err != nil || rbRes.BlockedCount > 0 || len(rbRes.Errors) > 0 {
		t.Fatalf("RollbackBatch failed: restored=%d blocked=%d err=%v errs=%v", rbRes.RestoredCount, rbRes.BlockedCount, err, rbRes.Errors)
	}

	assertSnapshotsIdentical(t, snapDiskABefore, snapshotDirSHA256(t, diskARoot))
	assertSnapshotsIdentical(t, snapDiskBBefore, snapshotDirSHA256(t, diskBRoot))

	if st, err := os.Stat(preEmptyDir); err != nil || !st.IsDir() {
		t.Fatalf("expected Capote (2005) empty folder to be restored on rollback: %v", err)
	}
}
