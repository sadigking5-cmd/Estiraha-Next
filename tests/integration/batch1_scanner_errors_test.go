package integration_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/sadigking5-cmd/Estiraha-Next/internal/db"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/errorsbox"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/identify"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/library"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/review"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/scanner"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/secrets"
)

type fakeUnavailableProber struct{}

func (fakeUnavailableProber) ProbeVideo(_ context.Context, _ string) scanner.MediaProbeResult {
	return scanner.MediaProbeResult{
		ProbeStatus: scanner.ProbeStatusUnavailableNeedsReview,
		ProbeError:  "simulated missing ffprobe binary",
	}
}

func generateTestVideoWithFFmpeg(t *testing.T, outPath string, durationSec int, width, height int, extraArgs ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", outPath, err)
	}
	ffmpegBin, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Fatalf("ffmpeg required in test environment to synthesize genuine video containers: %v", err)
	}
	args := []string{
		"-y", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc=size=" + itoa(width) + "x" + itoa(height) + ":rate=15",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=22050",
		"-t", itoa(durationSec),
	}
	args = append(args, extraArgs...)
	args = append(args, outPath)
	cmd := exec.Command(ffmpegBin, args...)
	out, runErr := cmd.CombinedOutput()
	if runErr != nil {
		t.Fatalf("ffmpeg generate %s failed: %v (%s)", outPath, runErr, string(out))
	}
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

func makeValidZipBytes(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

func snapshotDirSHA256(t *testing.T, rootDir string) map[string]string {
	t.Helper()
	snap := map[string]string{}
	err := filepath.WalkDir(rootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, rErr := filepath.Rel(rootDir, path)
		if rErr != nil {
			return rErr
		}
		f, oErr := os.Open(path)
		if oErr != nil {
			return oErr
		}
		h := sha256.New()
		_, cErr := io.Copy(h, f)
		f.Close()
		if cErr != nil {
			return cErr
		}
		snap[filepath.ToSlash(rel)] = hex.EncodeToString(h.Sum(nil))
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot %s: %v", rootDir, err)
	}
	return snap
}

func assertSnapshotsIdentical(t *testing.T, before, after map[string]string) {
	t.Helper()
	if len(before) != len(after) {
		t.Fatalf("file count changed during scan/identify! before=%d after=%d", len(before), len(after))
	}
	for rel, bHash := range before {
		aHash, ok := after[rel]
		if !ok {
			t.Fatalf("file %q vanished or moved without approval!", rel)
		}
		if aHash != bHash {
			t.Fatalf("SHA-256 changed on %q: before=%s after=%s", rel, bHash, aHash)
		}
	}
}

// TestBatch1ScannerIntelligenceFFProbeAndRuleGuards verifies:
// 1. .@__thumb exclusion
// 2. AppleDouble ._* protection (never steals poster or subtitle slot!)
// 3. 0 B files detection (`zero_byte_files`)
// 4. Thumbs.db & desktop.ini detection (`auxiliary_or_unknown`)
// 5. Tiny MP4 clips & Sample MKV via genuine ffprobe stream inspection
// 6. Rule 3 & Rule 4: Small size ALONE never guesses sample; missing/failed ffprobe marks `conflicts_needing_review`
// 7. .rmvb, .rm, .divx, .asf, .vob video format recognition
// 8. .shorts strict protection (never classified as sample or junk)
// 9. Arabic folder & movie names
// 10. Rule 6: Meaningful .txt, .zip, and .ico files are NOT treated as junk automatically
// 11. Multilingual subtitles preserved & UUIDs preserved on Migration 0004 upgrade
// 12. 100% SHA-256 verification proving ZERO files moved, overwritten, or deleted during scan/identify
func TestBatch1ScannerIntelligenceFFProbeAndRuleGuards(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	tmp := t.TempDir()
	dbFile := filepath.Join(tmp, "batch1_scanner.sqlite")
	backupDir := filepath.Join(tmp, "backups")
	libRoot := filepath.Join(tmp, "مكتبة_الأفلام_التجريبية")

	engine, err := db.Open(ctx, dbFile)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer engine.Close()

	ver, err := engine.Migrate(ctx, backupDir)
	if err != nil || ver < 4 {
		t.Fatalf("expected Migration >= 0004 (ver>=4), got ver=%d err=%v", ver, err)
	}

	libSvc := library.NewService(engine.SQL())
	scanSvc := scanner.NewService(engine.SQL(), libSvc)
	identSvc := identify.NewService(engine.SQL(), secrets.NewDefaultVault())
	errBoxSvc := errorsbox.NewService(engine.SQL())

	// Pre-create official `الأخطاء/` structure inside library root with decoy files to prove scanner skips it!
	_, catPaths, err := errBoxSvc.EnsureErrorStructure(libRoot)
	if err != nil {
		t.Fatalf("EnsureErrorStructure: %v", err)
	}
	mustWriteFile(t, filepath.Join(catPaths[errorsbox.CategorySamplesShortClips], "quarantined_old_sample_2005.mp4"), []byte("QUARANTINED_DO_NOT_RESCAN"))
	mustWriteFile(t, filepath.Join(catPaths[errorsbox.CategoryUnlinkedSubtitles], "orphan_old.srt"), []byte("1\n00:00:01,000 --> 00:00:02,000\nOrphan\n"))

	// 1) `.@__thumb` inside `Basic (2003)` + genuine feature MP4 (probed > 90s via `-stream_loop` or real container)
	// Generate a 2s clip and loop-mux to 120s (fast <0.2s remux!) so ffprobe sees a genuine 120s feature film!
	baseSeedClip := filepath.Join(tmp, "seed_2s.mp4")
	generateTestVideoWithFFmpeg(t, baseSeedClip, 2, 640, 360, "-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac")

	basicFeature := filepath.Join(libRoot, "Basic (2003)", "Basic (2003) 1080p.mp4")
	if err := os.MkdirAll(filepath.Dir(basicFeature), 0o755); err != nil {
		t.Fatal(err)
	}
	muxCmd := exec.Command("ffmpeg", "-y", "-hide_banner", "-loglevel", "error", "-stream_loop", "59", "-i", baseSeedClip, "-c", "copy", basicFeature)
	if out, mErr := muxCmd.CombinedOutput(); mErr != nil {
		t.Fatalf("remux 120s feature failed: %v (%s)", mErr, string(out))
	}
	// Hidden QNAP `.@__thumb` proxy MP4s that previously turned into 5 fake movies!
	for _, thumbName := range []string{"a100Basic (2003).mp4", "adefaultBasic (2003).mp4", "defaultBasic (2003).mp4", "s100Basic (2003).mp4", "s800Basic (2003).mp4"} {
		mustWriteFile(t, filepath.Join(libRoot, "Basic (2003)", ".@__thumb", thumbName), []byte("QNAP_THUMB_PROXY_VIDEO"))
	}

	// 2) AppleDouble `._*` trap (`Bandidas (2006)` & `15 Minutes (2001)`) + multilingual subtitles + OS artifacts (`Thumbs.db`, `desktop.ini`)
	bandidasVideo := filepath.Join(libRoot, "Bandidas (2006)", "Bandidas.2006.720p.mp4")
	if err := os.MkdirAll(filepath.Dir(bandidasVideo), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, mErr := exec.Command("ffmpeg", "-y", "-hide_banner", "-loglevel", "error", "-stream_loop", "59", "-i", baseSeedClip, "-c", "copy", bandidasVideo).CombinedOutput(); mErr != nil {
		t.Fatalf("bandidas video: %v (%s)", mErr, string(out))
	}
	// Fake 4KB macOS AppleDouble resource forks sorting BEFORE real files
	appleDoubleFork := append([]byte{0x00, 0x05, 0x16, 0x07, 0x00, 0x02, 0x00, 0x00}, bytes.Repeat([]byte("M"), 4088)...)
	mustWriteFile(t, filepath.Join(libRoot, "Bandidas (2006)", "._Bandidas.srt"), appleDoubleFork)
	mustWriteFile(t, filepath.Join(libRoot, "Bandidas (2006)", "Bandidas.ar.srt"), []byte("1\n00:00:01,000 --> 00:00:03,000\nترجمة عربية حقيقية\n"))
	mustWriteFile(t, filepath.Join(libRoot, "Bandidas (2006)", "Bandidas.en.srt"), []byte("1\n00:00:01,000 --> 00:00:03,000\nGenuine English Subtitle\n"))
	mustWriteFile(t, filepath.Join(libRoot, "Bandidas (2006)", "._folder.jpg"), appleDoubleFork)
	mustWriteFile(t, filepath.Join(libRoot, "Bandidas (2006)", "folder.jpg"), []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01})
	mustWriteFile(t, filepath.Join(libRoot, "Bandidas (2006)", "Thumbs.db"), []byte("WINDOWS_THUMBS_DB_CACHE"))
	mustWriteFile(t, filepath.Join(libRoot, "Bandidas (2006)", "desktop.ini"), []byte("[.ShellClassInfo]\r\nIconResource=movie.ico,0\r\n"))

	// 3) Zero-byte (`0 B`) files (`Crash.srt` 0B + `www.UsaBit.com.txt` 0B)
	mustWriteFile(t, filepath.Join(libRoot, "Crash (2004)", "Crash.2004.720p.YIFY.srt"), []byte{})
	mustWriteFile(t, filepath.Join(libRoot, "Crash (2004)", "www.UsaBit.com.txt"), []byte{})

	// 4) Tiny MP4 short clip (2 seconds) & Sample MKV (3 seconds) probed by live ffprobe
	tinyShortMP4 := filepath.Join(libRoot, "Loose_Clips", "tiny_clip_2008.mp4")
	generateTestVideoWithFFmpeg(t, tinyShortMP4, 2, 320, 180, "-c:v", "libx264", "-preset", "ultrafast", "-an")

	fishTankDir := filepath.Join(libRoot, "Fish Tank (2009) 720p")
	fishTankMain := filepath.Join(fishTankDir, "Fish.Tank.2009.720p.mkv")
	if err := os.MkdirAll(fishTankDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, mErr := exec.Command("ffmpeg", "-y", "-hide_banner", "-loglevel", "error", "-stream_loop", "59", "-i", baseSeedClip, "-c", "copy", fishTankMain).CombinedOutput(); mErr != nil {
		t.Fatalf("fishtank main mkv: %v (%s)", mErr, string(out))
	}
	fishTankSample := filepath.Join(fishTankDir, "Fish.Tank.2009.720p_sample.mkv")
	generateTestVideoWithFFmpeg(t, fishTankSample, 3, 640, 360, "-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac")

	// 5) Legacy video containers: `.rmvb`, `.rm`, `.divx`, `.asf`, `.vob`
	batmanDir := filepath.Join(libRoot, "BATMAN 2005-08-12")
	generateTestVideoWithFFmpeg(t, filepath.Join(batmanDir, "Batman.Begins.2005.rmvb"), 2, 352, 288, "-f", "rm", "-c:v", "rv20", "-b:v", "300k", "-an")
	// Loop or make 100s ASF/VOB/RM/DIVX or check recognition: note 2s small clip without sample keyword IS <90s & <50MB if probed_ok unless >90s.
	// Let's generate a 95-second RMVB so it's recognized as a full movie (not a 2s micro-clip)!
	generateTestVideoWithFFmpeg(t, filepath.Join(batmanDir, "The.Dark.Knight.2008.rmvb"), 95, 352, 288, "-f", "rm", "-c:v", "rv20", "-b:v", "200k", "-an")
	mustWriteFile(t, filepath.Join(batmanDir, "Classic.1999.rm"), bytes.Repeat([]byte("RMF_LEGACY_STREAM_"), 2048))
	mustWriteFile(t, filepath.Join(batmanDir, "Classic.2001.divx"), bytes.Repeat([]byte("DIVX_LEGACY_STREAM_"), 2048))
	mustWriteFile(t, filepath.Join(batmanDir, "Classic.2002.asf"), bytes.Repeat([]byte("ASF_LEGACY_STREAM_"), 2048))
	mustWriteFile(t, filepath.Join(batmanDir, "Classic.2003.vob"), bytes.Repeat([]byte("VOB_LEGACY_STREAM_"), 2048))

	// 6) Arabic folder name + Arabic movie + `.shorts` companion + Conservative `.txt`, `.zip`, `.ico` vs torrent `.txt`
	arabicDir := filepath.Join(libRoot, "أفلام عربية كلاسيكية", "الرسالة 1976")
	arabicMovie := filepath.Join(arabicDir, "الرسالة 1976.mp4")
	if err := os.MkdirAll(arabicDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, mErr := exec.Command("ffmpeg", "-y", "-hide_banner", "-loglevel", "error", "-stream_loop", "59", "-i", baseSeedClip, "-c", "copy", arabicMovie).CombinedOutput(); mErr != nil {
		t.Fatalf("arabic movie: %v (%s)", mErr, string(out))
	}
	mustWriteFile(t, filepath.Join(arabicDir, "الرسالة 1976.shorts"), []byte("PROTECTED_ARABIC_MOVIE_SHORTS_METADATA"))
	mustWriteFile(t, filepath.Join(arabicDir, "ملاحظات_المخرج_والقصة.txt"), []byte("هذا ملف نصي مهم كتبه المستخدم حول تاريخ تصوير فيلم الرسالة ولا يجوز اعتباره مخلفات تورنت أبداً."))
	mustWriteFile(t, filepath.Join(arabicDir, "Torrent downloaded from Demonoid.ph.txt"), []byte("Torrent downloaded from Demonoid.ph"))
	mustWriteFile(t, filepath.Join(arabicDir, "subtitles_pack.zip"), makeValidZipBytes(t, map[string]string{"Arabic.srt": "1\n00:00:01,000 --> 00:00:02,000\nمرحباً\n"}))
	mustWriteFile(t, filepath.Join(arabicDir, "custom_folder_icon.ico"), bytes.Repeat([]byte{0x00, 0x00, 0x01, 0x00}, 256))

	// Take complete SHA-256 snapshot BEFORE scanning to prove zero mutation/deletion/move during Batch 1 scan & identify
	snapBefore := snapshotDirSHA256(t, libRoot)

	rootView, err := libSvc.AddRoot(ctx, library.AddRootInput{
		FullPath:        libRoot,
		ContentCategory: "movies",
		DisplayName:     "مكتبة اختبار الدفعة الأولى",
		DiskLabel:       "Batch1TestDisk",
	})
	if err != nil {
		t.Fatalf("AddRoot failed: %v", err)
	}

	scanSum, err := scanSvc.RunManualScan(ctx, scanner.ScanOptions{ComputeHash: true}, nil)
	if err != nil {
		t.Fatalf("RunManualScan failed: %v", err)
	}

	// Verify `.@__thumb` and `الأخطاء` directories were skipped completely!
	if scanSum.ThumbDirsSkipped < 1 {
		t.Fatalf("expected .@__thumb directory to be skipped, got ThumbDirsSkipped=%d", scanSum.ThumbDirsSkipped)
	}
	if scanSum.ErrorDirsSkipped < 1 {
		t.Fatalf("expected الأخطاء directory to be skipped, got ErrorDirsSkipped=%d", scanSum.ErrorDirsSkipped)
	}

	var thumbIndexedCount int
	_ = engine.SQL().QueryRowContext(ctx, `SELECT COUNT(*) FROM media_files WHERE relative_path LIKE '%.@__thumb%'`).Scan(&thumbIndexedCount)
	if thumbIndexedCount != 0 {
		t.Fatalf("expected 0 files indexed from .@__thumb, got %d", thumbIndexedCount)
	}

	var errorDirIndexedCount int
	_ = engine.SQL().QueryRowContext(ctx, `SELECT COUNT(*) FROM media_files WHERE relative_path LIKE '%الأخطاء%'`).Scan(&errorDirIndexedCount)
	if errorDirIndexedCount != 0 {
		t.Fatalf("expected 0 files indexed from managed الأخطاء folder, got %d", errorDirIndexedCount)
	}

	// Verify AppleDouble `._*` classification
	var adKind, adCat string
	var adFlag int
	if err := engine.SQL().QueryRowContext(ctx, `
		SELECT file_kind, anomaly_category, is_apple_double FROM media_files WHERE filename = '._Bandidas.srt'
	`).Scan(&adKind, &adCat, &adFlag); err != nil {
		t.Fatalf("query ._Bandidas.srt: %v", err)
	}
	if adKind != "unknown_extra" || adCat != scanner.AnomalyAuxiliaryOrUnknown || adFlag != 1 {
		t.Fatalf("expected ._Bandidas.srt to be unknown_extra + auxiliary_or_unknown + is_apple_double=1, got kind=%q cat=%q flag=%d", adKind, adCat, adFlag)
	}

	// Verify 0 B detection
	var zeroCat, zeroIntegrity string
	if err := engine.SQL().QueryRowContext(ctx, `
		SELECT anomaly_category, integrity_status FROM media_files WHERE filename = 'Crash.2004.720p.YIFY.srt'
	`).Scan(&zeroCat, &zeroIntegrity); err != nil {
		t.Fatalf("query 0B srt: %v", err)
	}
	if zeroCat != scanner.AnomalyZeroByteFiles || zeroIntegrity != "corrupted" {
		t.Fatalf("expected 0B file to be zero_byte_files + corrupted, got cat=%q integrity=%q", zeroCat, zeroIntegrity)
	}

	// Verify Thumbs.db & desktop.ini indexed as auxiliary_or_unknown system artifacts
	for _, sysFile := range []string{"Thumbs.db", "desktop.ini"} {
		var sCat string
		var sysFlag int
		if err := engine.SQL().QueryRowContext(ctx, `
			SELECT anomaly_category, is_system_artifact FROM media_files WHERE filename = ?
		`, sysFile).Scan(&sCat, &sysFlag); err != nil {
			t.Fatalf("expected %s to be indexed, got err: %v", sysFile, err)
		}
		if sCat != scanner.AnomalyAuxiliaryOrUnknown || sysFlag != 1 {
			t.Fatalf("expected %s -> auxiliary_or_unknown + sysFlag=1, got cat=%q flag=%d", sysFile, sCat, sysFlag)
		}
	}

	// Verify live ffprobe detected tiny 2s MP4 clip & 3s Sample MKV as `samples_and_short_clips`
	for _, sampleFile := range []string{"tiny_clip_2008.mp4", "Fish.Tank.2009.720p_sample.mkv"} {
		var pStatus, aCat string
		var dur float64
		var sampleFlag int
		if err := engine.SQL().QueryRowContext(ctx, `
			SELECT probe_status, duration_seconds, anomaly_category, is_sample_clip FROM media_files WHERE filename = ?
		`, sampleFile).Scan(&pStatus, &dur, &aCat, &sampleFlag); err != nil {
			t.Fatalf("query sample %s: %v", sampleFile, err)
		}
		if pStatus != scanner.ProbeStatusOK || dur <= 0 || dur > 5 || aCat != scanner.AnomalySamplesAndShortClips || sampleFlag != 1 {
			t.Fatalf("expected %s probed_ok short sample, got status=%q dur=%.2f cat=%q sampleFlag=%d", sampleFile, pStatus, dur, aCat, sampleFlag)
		}
	}

	// Verify `.rmvb`, `.rm`, `.divx`, `.asf`, `.vob` are all recognized as `file_kind = 'video'`
	for _, vidName := range []string{"The.Dark.Knight.2008.rmvb", "Classic.1999.rm", "Classic.2001.divx", "Classic.2002.asf", "Classic.2003.vob"} {
		var fKind string
		if err := engine.SQL().QueryRowContext(ctx, `SELECT file_kind FROM media_files WHERE filename = ?`, vidName).Scan(&fKind); err != nil || fKind != "video" {
			t.Fatalf("expected legacy video format %s to have file_kind='video', got %q (err=%v)", vidName, fKind, err)
		}
	}
	// And verify the corrupted/non-container fake bytes `.divx` failed ffprobe and landed in `conflicts_needing_review` (Rule 4: never guess!)
	var divxProbe, divxAnomaly string
	_ = engine.SQL().QueryRowContext(ctx, `SELECT probe_status, anomaly_category FROM media_files WHERE filename = 'Classic.2001.divx'`).Scan(&divxProbe, &divxAnomaly)
	if divxProbe != scanner.ProbeStatusFailedNeedsReview || divxAnomaly != scanner.AnomalyConflictsNeedingReview {
		t.Fatalf("Rule 4 violation: expected unparseable video container to mark probe_failed_needs_review + conflicts_needing_review, got probe=%q cat=%q", divxProbe, divxAnomaly)
	}

	// Verify `.shorts` protection (Rule 5): NEVER classified as sample or anomaly!
	var shortsKind, shortsCat, shortsKeep string
	var shortsSample int
	if err := engine.SQL().QueryRowContext(ctx, `
		SELECT file_kind, anomaly_category, keep_decision, is_sample_clip FROM media_files WHERE filename = 'الرسالة 1976.shorts'
	`).Scan(&shortsKind, &shortsCat, &shortsKeep, &shortsSample); err != nil {
		t.Fatalf("query .shorts: %v", err)
	}
	if shortsKind != "shorts" || shortsCat != "" || shortsKeep != "keep" || shortsSample != 0 {
		t.Fatalf("Rule 5 violation on .shorts: kind=%q cat=%q keep=%q sample=%d", shortsKind, shortsCat, shortsKeep, shortsSample)
	}

	// Verify Rule 6: user `.txt`, `.zip`, and `.ico` are NOT auto-flagged as junk, whereas confirmed torrent promo `.txt` IS flagged!
	for _, cleanExtra := range []string{"ملاحظات_المخرج_والقصة.txt", "subtitles_pack.zip", "custom_folder_icon.ico"} {
		var cCat string
		var sysFlag int
		if err := engine.SQL().QueryRowContext(ctx, `
			SELECT anomaly_category, is_system_artifact FROM media_files WHERE filename = ?
		`, cleanExtra).Scan(&cCat, &sysFlag); err != nil {
			t.Fatalf("query %s: %v", cleanExtra, err)
		}
		if cCat != "" || sysFlag != 0 {
			t.Fatalf("Rule 6 violation: %s was wrongly auto-flagged as junk (cat=%q sysFlag=%d)", cleanExtra, cCat, sysFlag)
		}
	}
	var promoCat string
	_ = engine.SQL().QueryRowContext(ctx, `SELECT anomaly_category FROM media_files WHERE filename = 'Torrent downloaded from Demonoid.ph.txt'`).Scan(&promoCat)
	if promoCat != scanner.AnomalyAuxiliaryOrUnknown {
		t.Fatalf("expected confirmed Demonoid torrent promo txt to be auxiliary_or_unknown, got %q", promoCat)
	}

	// Run IdentifyLibrary and verify:
	// - Samples (`tiny_clip_2008.mp4`, `Fish.Tank..._sample.mkv`) were NOT promoted to movies!
	// - Real `The.Dark.Knight.2008.rmvb` WAS identified as `The Dark Knight 2008`!
	// - Arabic movie `الرسالة 1976` WAS identified!
	// - In `Bandidas (2006)`, real `folder.jpg` became poster (NOT `._folder.jpg`), and BOTH `.ar.srt` & `.en.srt` were linked while `._Bandidas.srt` was ignored!
	_, err = identSvc.IdentifyLibrary(ctx)
	if err != nil {
		t.Fatalf("IdentifyLibrary: %v", err)
	}

	var sampleWorkID string
	_ = engine.SQL().QueryRowContext(ctx, `SELECT COALESCE(work_id, '') FROM media_files WHERE filename = 'Fish.Tank.2009.720p_sample.mkv'`).Scan(&sampleWorkID)
	if sampleWorkID != "" {
		t.Fatalf("sample MKV must NOT be promoted to a movie work, got work_id=%q", sampleWorkID)
	}

	var rmvbCanon string
	if err := engine.SQL().QueryRowContext(ctx, `
		SELECT w.canonical_folder_name FROM works w
		JOIN media_files mf ON mf.work_id = w.id
		WHERE mf.filename = 'The.Dark.Knight.2008.rmvb'
	`).Scan(&rmvbCanon); err != nil || rmvbCanon != "The Dark Knight 2008" {
		t.Fatalf("expected RMVB movie 'The Dark Knight 2008' to be identified, got %q (err=%v)", rmvbCanon, err)
	}

	var arCanon string
	if err := engine.SQL().QueryRowContext(ctx, `
		SELECT w.canonical_folder_name FROM works w
		JOIN media_files mf ON mf.work_id = w.id
		WHERE mf.filename = 'الرسالة 1976.mp4'
	`).Scan(&arCanon); err != nil || arCanon != "الرسالة 1976" {
		t.Fatalf("expected Arabic movie 'الرسالة 1976' to be identified, got %q (err=%v)", arCanon, err)
	}

	var bandidasPosterName string
	if err := engine.SQL().QueryRowContext(ctx, `
		SELECT mf.filename FROM works w
		JOIN media_files mf ON mf.id = w.poster_rel_path
		WHERE w.canonical_folder_name = 'Bandidas 2006'
	`).Scan(&bandidasPosterName); err != nil || bandidasPosterName != "folder.jpg" {
		t.Fatalf("expected real 'folder.jpg' (not ._folder.jpg) to be linked as Bandidas poster, got %q (err=%v)", bandidasPosterName, err)
	}

	var bandidasSubCount int
	_ = engine.SQL().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM subtitle_tracks st
		JOIN works w ON w.id = st.work_id
		WHERE w.canonical_folder_name = 'Bandidas 2006'
	`).Scan(&bandidasSubCount)
	if bandidasSubCount != 2 {
		t.Fatalf("Rule 7: expected both Arabic and English subtitles linked to Bandidas 2006 (excluding ._Bandidas.srt), got %d", bandidasSubCount)
	}

	// Verify Rule 3 & Rule 4 explicitly when ffprobe is UNAVAILABLE:
	// Small 2MB MP4 without ffprobe MUST NOT be guessed as sample; it MUST be marked `probe_unavailable_needs_review` + `conflicts_needing_review`!
	noProbeRoot := filepath.Join(tmp, "NoFFProbe_Root")
	mustWriteFile(t, filepath.Join(noProbeRoot, "SmallMysteryClip 2010.mp4"), bytes.Repeat([]byte("MP4DATA"), 64*1024)) // ~448 KB
	rNoProbe, err := libSvc.AddRoot(ctx, library.AddRootInput{FullPath: noProbeRoot, ContentCategory: "movies", DisplayName: "NoProbe"})
	if err != nil {
		t.Fatalf("add root no-probe: %v", err)
	}
	scanSvc.SetMediaProber(fakeUnavailableProber{})
	if _, err := scanSvc.RunManualScan(ctx, scanner.ScanOptions{RootIDs: []string{rNoProbe.ID}}, nil); err != nil {
		t.Fatalf("scan without ffprobe: %v", err)
	}
	var mysteryProbe, mysteryCat string
	var mysterySample int
	if err := engine.SQL().QueryRowContext(ctx, `
		SELECT probe_status, anomaly_category, is_sample_clip FROM media_files WHERE filename = 'SmallMysteryClip 2010.mp4'
	`).Scan(&mysteryProbe, &mysteryCat, &mysterySample); err != nil {
		t.Fatalf("query SmallMysteryClip: %v", err)
	}
	if mysteryProbe != scanner.ProbeStatusUnavailableNeedsReview || mysteryCat != scanner.AnomalyConflictsNeedingReview || mysterySample != 0 {
		t.Fatalf("Rules 3 & 4 violation: without ffprobe, small MP4 must be needs_review (NOT guessed as sample), got probe=%q cat=%q sample=%d", mysteryProbe, mysteryCat, mysterySample)
	}

	// Verify re-scanning preserves existing file UUIDs (Rule 8) and still skips `الأخطاء`!
	var uuidBefore string
	_ = engine.SQL().QueryRowContext(ctx, `SELECT id FROM media_files WHERE filename = 'الرسالة 1976.mp4'`).Scan(&uuidBefore)
	scanSvc.SetMediaProber(scanner.NewFFProbeAnalyzer(""))
	if _, err := scanSvc.RunManualScan(ctx, scanner.ScanOptions{RootIDs: []string{rootView.ID}, ComputeHash: true}, nil); err != nil {
		t.Fatalf("second manual scan: %v", err)
	}
	var uuidAfter string
	_ = engine.SQL().QueryRowContext(ctx, `SELECT id FROM media_files WHERE filename = 'الرسالة 1976.mp4'`).Scan(&uuidAfter)
	if uuidBefore == "" || uuidBefore != uuidAfter {
		t.Fatalf("Rule 8 violation: file UUID changed across rescan! before=%q after=%q", uuidBefore, uuidAfter)
	}

	// Verify 100% SHA-256 non-mutation, non-move, non-deletion across the entire library fixture!
	snapAfter := snapshotDirSHA256(t, libRoot)
	assertSnapshotsIdentical(t, snapBefore, snapAfter)
}

// TestBatch1ManagedErrorFolderCategoriesCollisionPreventionRestoreAndNoRescan tests:
// 1. Creation of `الأخطاء/` with all 6 official Arabic category folders
// 2. Collision-free naming when moving identically named files (`WWW.YIFY-TORRENTS.COM.jpg` & `Thumbs.db`) from multiple movie folders
// 3. Rejection of unapproved moves and rejection of `.shorts` quarantine
// 4. Moving empty folder into `الأخطاء/مجلدات فارغة/` safely
// 5. SHA-256 bit-for-bit verification before/after quarantine and after 1-click Restore
// 6. Scanner completely ignores everything inside `الأخطاء/` on subsequent scans
func TestBatch1ManagedErrorFolderCategoriesCollisionPreventionRestoreAndNoRescan(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	tmp := t.TempDir()
	dbFile := filepath.Join(tmp, "batch1_errorsbox.sqlite")
	backupDir := filepath.Join(tmp, "backups")
	libRoot := filepath.Join(tmp, "TestLibrary_ErrorsBox")

	engine, err := db.Open(ctx, dbFile)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer engine.Close()

	if _, err := engine.Migrate(ctx, backupDir); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	libSvc := library.NewService(engine.SQL())
	scanSvc := scanner.NewService(engine.SQL(), libSvc)
	errBox := errorsbox.NewService(engine.SQL())

	// Populate isolated temp fixture with colliding junk files in 3 movie folders + an empty folder + a protected .shorts file
	mustWriteFile(t, filepath.Join(libRoot, "Blow (2001)", "WWW.YIFY-TORRENTS.COM.jpg"), []byte("YIFY_BANNER_BLOW_BYTES_111"))
	mustWriteFile(t, filepath.Join(libRoot, "Catch Me If You Can (2002)", "WWW.YIFY-TORRENTS.COM.jpg"), []byte("YIFY_BANNER_CATCH_BYTES_222"))
	mustWriteFile(t, filepath.Join(libRoot, "Crash (2004)", "WWW.YIFY-TORRENTS.COM.jpg"), []byte("YIFY_BANNER_CRASH_BYTES_333"))
	mustWriteFile(t, filepath.Join(libRoot, "Crash (2004)", "empty_sub.srt"), []byte{})
	mustWriteFile(t, filepath.Join(libRoot, "ProtectedMovie 2020", "ProtectedMovie 2020.shorts"), []byte("DO_NOT_QUARANTINE_SHORTS"))
	emptyFolderFull := filepath.Join(libRoot, "Capote (2005)")
	if err := os.MkdirAll(emptyFolderFull, 0o755); err != nil {
		t.Fatal(err)
	}

	rootView, err := libSvc.AddRoot(ctx, library.AddRootInput{
		FullPath:        libRoot,
		ContentCategory: "movies",
		DisplayName:     "ErrorsBox Test Root",
	})
	if err != nil {
		t.Fatalf("AddRoot: %v", err)
	}

	if _, err := scanSvc.RunManualScan(ctx, scanner.ScanOptions{ComputeHash: true}, nil); err != nil {
		t.Fatalf("initial scan: %v", err)
	}

	// Verify all 6 official Arabic category folders are created cleanly
	errRootPath, catMap, err := errBox.EnsureErrorStructure(libRoot)
	if err != nil {
		t.Fatalf("EnsureErrorStructure: %v", err)
	}
	if filepath.Base(errRootPath) != "الأخطاء" || len(catMap) != 6 {
		t.Fatalf("expected الأخطاء with 6 categories, got base=%q count=%d", filepath.Base(errRootPath), len(catMap))
	}
	expectedFolders := []string{
		"مجلدات فارغة",
		"ملفات صفرية",
		"ترجمات غير مرتبطة",
		"ملفات مساعدة وغير معروفة",
		"عينات ومقاطع قصيرة",
		"تعارضات تحتاج مراجعة",
	}
	entries, _ := os.ReadDir(errRootPath)
	gotFolders := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			gotFolders = append(gotFolders, e.Name())
		}
	}
	sort.Strings(expectedFolders)
	sort.Strings(gotFolders)
	if strings.Join(expectedFolders, "|") != strings.Join(gotFolders, "|") {
		t.Fatalf("category folders mismatch:\ngot:  %v\nwant: %v", gotFolders, expectedFolders)
	}

	permit := review.ExecutionPermit{
		BatchID:      "",
		ReviewItemID: "approved-item-test",
		Decision:     review.DecisionApproved,
		ApprovedAt:   time.Now().UTC(),
	}
	// Note: review.ValidatePermit requires non-empty BatchID & ReviewItemID
	validPermit := review.ExecutionPermit{
		BatchID:      "batch-test-001",
		ReviewItemID: "item-test-001",
		Decision:     review.DecisionApproved,
		ApprovedAt:   time.Now().UTC(),
	}
	// Insert dummy review_batches row so foreign key on batch_id is satisfied!
	_, _ = engine.SQL().ExecContext(ctx, `
		INSERT INTO review_batches (id, title, batch_type, batch_status)
		VALUES ('batch-test-001', 'Test Error Folder Batch', 'move_extras_to_errors', 'approved')
	`)

	// Verify unapproved permit is strictly rejected
	badPermit := permit // empty BatchID
	if _, err := errBox.MoveApprovedItem(ctx, errorsbox.MoveToErrorsRequest{
		Permit:          badPermit,
		DiskID:          rootView.DiskID,
		RootID:          rootView.ID,
		MountPath:       rootView.MountPath,
		RootRelPath:     rootView.RelativeRootPath,
		Category:        errorsbox.CategoryAuxiliaryUnknown,
		OriginalRelPath: rootView.RelativeRootPath + "/Blow (2001)/WWW.YIFY-TORRENTS.COM.jpg",
	}); err == nil {
		t.Fatal("expected MoveApprovedItem without valid BatchID permit to fail!")
	}

	// Verify `.shorts` cannot be quarantined into `الأخطاء/`
	if _, err := errBox.MoveApprovedItem(ctx, errorsbox.MoveToErrorsRequest{
		Permit:          validPermit,
		DiskID:          rootView.DiskID,
		RootID:          rootView.ID,
		MountPath:       rootView.MountPath,
		RootRelPath:     rootView.RelativeRootPath,
		Category:        errorsbox.CategorySamplesShortClips,
		OriginalRelPath: rootView.RelativeRootPath + "/ProtectedMovie 2020/ProtectedMovie 2020.shorts",
	}); err == nil {
		t.Fatal("SAFETY VIOLATION: .shorts file was allowed into الأخطاء!")
	}

	// Move 3 identically named `WWW.YIFY-TORRENTS.COM.jpg` files and verify ZERO collision / ZERO overwrite + exact SHA-256 preservation!
	folders := []string{"Blow (2001)", "Catch Me If You Can (2002)", "Crash (2004)"}
	movedEntries := make([]errorsbox.EntryView, 0, 3)
	origHashes := map[string]string{}

	for _, fld := range folders {
		relPath := rootView.RelativeRootPath + "/" + fld + "/WWW.YIFY-TORRENTS.COM.jpg"
		fullP := library.ResolveFullPath(rootView.MountPath, relPath)
		rawB, _ := os.ReadFile(fullP)
		sum := sha256.Sum256(rawB)
		origHashes[fld] = hex.EncodeToString(sum[:])

		var fID string
		_ = engine.SQL().QueryRowContext(ctx, `SELECT id FROM media_files WHERE disk_id = ? AND relative_path = ?`, rootView.DiskID, relPath).Scan(&fID)

		ev, mErr := errBox.MoveApprovedItem(ctx, errorsbox.MoveToErrorsRequest{
			Permit:          validPermit,
			DiskID:          rootView.DiskID,
			RootID:          rootView.ID,
			MountPath:       rootView.MountPath,
			RootRelPath:     rootView.RelativeRootPath,
			FileID:          fID,
			EntryKind:       "file",
			Category:        errorsbox.CategoryAuxiliaryUnknown,
			OriginalRelPath: relPath,
			ReasonAr:        "صورة دعائية لموقع تورنت",
			ReasonEn:        "Torrent tracker promotional image",
		})
		if mErr != nil {
			t.Fatalf("MoveApprovedItem (%s) failed: %v", fld, mErr)
		}
		if ev.SHA256Hash != origHashes[fld] {
			t.Fatalf("SHA-256 mismatch on %s: got %s want %s", fld, ev.SHA256Hash, origHashes[fld])
		}
		movedEntries = append(movedEntries, ev)
	}

	// Ensure all 3 error_rel_path destinations are unique and physically exist with distinct content!
	seenDest := map[string]bool{}
	for idx, ev := range movedEntries {
		if seenDest[ev.ErrorRelPath] {
			t.Fatalf("collision detected on ErrorRelPath: %s", ev.ErrorRelPath)
		}
		seenDest[ev.ErrorRelPath] = true
		dstFull := library.ResolveFullPath(rootView.MountPath, ev.ErrorRelPath)
		rawB, rErr := os.ReadFile(dstFull)
		if rErr != nil {
			t.Fatalf("read moved file %s: %v", dstFull, rErr)
		}
		sum := sha256.Sum256(rawB)
		if hex.EncodeToString(sum[:]) != origHashes[folders[idx]] {
			t.Fatalf("physical bytes overwritten or corrupted at %s!", dstFull)
		}
	}

	// Move empty directory `Capote (2005)` into `الأخطاء/مجلدات فارغة/`
	emptyEv, err := errBox.MoveApprovedItem(ctx, errorsbox.MoveToErrorsRequest{
		Permit:          validPermit,
		DiskID:          rootView.DiskID,
		RootID:          rootView.ID,
		MountPath:       rootView.MountPath,
		RootRelPath:     rootView.RelativeRootPath,
		EntryKind:       "directory",
		Category:        errorsbox.CategoryEmptyFolders,
		OriginalRelPath: rootView.RelativeRootPath + "/Capote (2005)",
		ReasonAr:        "مجلد فارغ بعد التنظيم",
		ReasonEn:        "Empty folder",
	})
	if err != nil {
		t.Fatalf("move empty folder failed: %v", err)
	}
	if _, stErr := os.Stat(emptyFolderFull); !os.IsNotExist(stErr) {
		t.Fatalf("expected original empty folder %s to be relocated", emptyFolderFull)
	}

	// Re-run Manual Scan and verify ZERO items inside `الأخطاء/` are scanned back as new present files!
	rescanSum, err := scanSvc.RunManualScan(ctx, scanner.ScanOptions{ComputeHash: true}, nil)
	if err != nil {
		t.Fatalf("rescan after quarantine: %v", err)
	}
	if rescanSum.NewFiles != 0 {
		t.Fatalf("expected 0 new files when rescanning root containing populated الأخطاء/, got %d", rescanSum.NewFiles)
	}

	// Restore one quarantined file + the empty directory and verify bit-for-bit restoration!
	restoredFile, err := errBox.RestoreEntry(ctx, movedEntries[1].ID)
	if err != nil {
		t.Fatalf("RestoreEntry file failed: %v", err)
	}
	restoredFull := library.ResolveFullPath(rootView.MountPath, restoredFile.OriginalRelPath)
	rawRestored, err := os.ReadFile(restoredFull)
	if err != nil {
		t.Fatalf("read restored file: %v", err)
	}
	sumRestored := sha256.Sum256(rawRestored)
	if hex.EncodeToString(sumRestored[:]) != origHashes["Catch Me If You Can (2002)"] {
		t.Fatalf("restored file SHA-256 mismatch!")
	}

	if _, err := errBox.RestoreEntry(ctx, emptyEv.ID); err != nil {
		t.Fatalf("RestoreEntry directory failed: %v", err)
	}
	if st, err := os.Stat(emptyFolderFull); err != nil || !st.IsDir() {
		t.Fatalf("expected restored empty directory at %s", emptyFolderFull)
	}
}
