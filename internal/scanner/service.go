package scanner

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/sadigking5-cmd/Estiraha-Next/internal/domain"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/errorsbox"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/library"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/naming"
)

var (
	ErrScanAlreadyRunning = errors.New("scanner: a manual library scan is already running")
	resRegex4K            = regexp.MustCompile(`(?i)\b(2160p|4k|uhd)\b`)
	resRegex1080          = regexp.MustCompile(`(?i)\b(1080p|fhd)\b`)
	resRegex720           = regexp.MustCompile(`(?i)\b(720p|hd)\b`)
	resRegex480           = regexp.MustCompile(`(?i)\b(480p|sd|dvdrip)\b`)
	codecHEVC             = regexp.MustCompile(`(?i)\b(x265|h265|hevc)\b`)
	codecAV1              = regexp.MustCompile(`(?i)\b(av1)\b`)
	codecH264             = regexp.MustCompile(`(?i)\b(x264|h264|avc)\b`)
	hdrToken              = regexp.MustCompile(`(?i)\b(hdr10\+|hdr10|dolbyvision|dv|hlg)\b`)
)

type ScanOptions struct {
	RootIDs     []string `json:"rootIds"`
	ComputeHash bool     `json:"computeHash"`
	ProbeVideos *bool    `json:"probeVideos,omitempty"`
}

type ScanSummary struct {
	JobID                    string   `json:"jobId"`
	State                    string   `json:"state"`
	RootsScanned             int      `json:"rootsScanned"`
	RootsSkippedOff          int      `json:"rootsSkippedOffline"`
	FilesDiscovered          int      `json:"filesDiscovered"`
	NewFiles                 int      `json:"newFiles"`
	UpdatedFiles             int      `json:"updatedFiles"`
	MissingMarked            int      `json:"missingMarked"`
	CorruptedDetected        int      `json:"corruptedDetected"`
	ZeroByteDetected         int      `json:"zeroByteDetected"`
	AppleDoubleDetected      int      `json:"appleDoubleDetected"`
	SystemArtifactsDetected  int      `json:"systemArtifactsDetected"`
	SamplesDetected          int      `json:"samplesDetected"`
	ProbeNeedsReviewDetected int      `json:"probeNeedsReviewDetected"`
	ThumbDirsSkipped         int      `json:"thumbDirsSkipped"`
	ErrorDirsSkipped         int      `json:"errorDirsSkipped"`
	ShortsDetected           int      `json:"shortsDetected"`
	DurationMs               int64    `json:"durationMs"`
	Warnings                 []string `json:"warnings"`
	StartedAt                string   `json:"startedAt"`
	CompletedAt              string   `json:"completedAt"`
}

type ProgressCallback func(summary ScanSummary)

type Service struct {
	sqlDB   *sql.DB
	libSvc  *library.Service
	prober  MediaProber
	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	lastJob ScanSummary
}

func NewService(sqlDB *sql.DB, libSvc *library.Service) *Service {
	return &Service{
		sqlDB:  sqlDB,
		libSvc: libSvc,
		prober: NewFFProbeAnalyzer(""),
	}
}

// SetMediaProber overrides the default FFProbeAnalyzer (used for deterministic unit/integration testing
// of missing or failing ffprobe environments).
func (s *Service) SetMediaProber(p MediaProber) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prober = p
}

func (s *Service) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

func (s *Service) LastSummary() ScanSummary {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastJob
}

func (s *Service) CancelActiveScan() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running || s.cancel == nil {
		return false
	}
	s.cancel()
	return true
}

// ClassifyFileKind categorizes physical filenames accurately:
//   - AppleDouble (`._*`) files are ALWAYS classified as `unknown_extra` BEFORE checking file extensions
//     so `._Bandidas.srt` or `._folder.jpg` NEVER pollute subtitles, posters, or videos.
//   - `.shorts` is strictly recognized and protected as `"shorts"`.
//   - Video formats include `.rmvb`, `.rm`, `.divx`, `.asf`, and `.vob` alongside modern containers.
//   - OS shell artifacts (`Thumbs.db`, `desktop.ini`, `.DS_Store`) are indexed as `unknown_extra`
//     (rather than silently ignored) so folders containing only `Thumbs.db` are properly accounted for.
func ClassifyFileKind(filename string) (ext string, kind string, supported bool) {
	base := filepath.Base(filename)
	if IsAppleDoubleFile(base) {
		ext = strings.ToLower(filepath.Ext(base))
		if ext == "" {
			ext = "._appledouble"
		}
		return ext, "unknown_extra", true
	}

	if naming.IsShortsExtension(base) {
		return ".shorts", "shorts", true
	}

	lowerBase := strings.ToLower(base)
	if lowerBase == ".ds_store" {
		return ".ds_store", "unknown_extra", true
	}

	ext = strings.ToLower(filepath.Ext(base))
	switch ext {
	case ".mkv", ".mp4", ".avi", ".mov", ".wmv", ".m4v", ".webm", ".ts", ".flv",
		".rmvb", ".rm", ".divx", ".asf", ".vob":
		return ext, "video", true
	case ".srt", ".ass", ".ssa", ".vtt", ".sub", ".idx":
		return ext, "subtitle", true
	case ".jpg", ".jpeg", ".png", ".webp":
		return ext, "image", true
	case ".nfo":
		return ext, "nfo", true
	case ".zip":
		return ext, "archive_zip", true
	case ".rar":
		return ext, "archive_rar", true
	case ".mp3", ".aac", ".mka", ".flac", ".ac3":
		return ext, "audio_ext", true
	default:
		if lowerBase == "thumbs.db" || lowerBase == "desktop.ini" {
			return ext, "unknown_extra", true
		}
		if ext == "" {
			return "", "", false
		}
		return ext, "unknown_extra", true
	}
}

func (s *Service) RunManualScan(parentCtx context.Context, opts ScanOptions, onProgress ProgressCallback) (ScanSummary, error) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return ScanSummary{}, ErrScanAlreadyRunning
	}
	scanCtx, cancel := context.WithCancel(parentCtx)
	s.running = true
	s.cancel = cancel
	prober := s.prober
	jobID := domain.NewImmutableID()
	start := time.Now().UTC()
	summary := ScanSummary{
		JobID:     jobID,
		State:     "running",
		Warnings:  []string{},
		StartedAt: start.Format(time.RFC3339Nano),
	}
	s.lastJob = summary
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.running = false
		s.cancel = nil
		s.lastJob = summary
		s.mu.Unlock()
	}()

	_, _ = s.sqlDB.ExecContext(parentCtx, `
		INSERT INTO background_jobs (id, job_type, job_state, started_at, updated_at)
		VALUES (?, 'disk_scan', 'running', ?, ?)
	`, jobID, summary.StartedAt, summary.StartedAt)

	_, roots, err := s.libSvc.RefreshConnectivity(scanCtx)
	if err != nil {
		summary.State = "failed"
		return summary, err
	}

	filterSet := map[string]bool{}
	for _, id := range opts.RootIDs {
		if strings.TrimSpace(id) != "" {
			filterSet[id] = true
		}
	}

	probeEnabled := opts.ProbeVideos != nil && *opts.ProbeVideos

	for _, root := range roots {
		if len(filterSet) > 0 && !filterSet[root.ID] {
			continue
		}
		if !root.ScanEnabled {
			continue
		}
		if root.ContentCategory == "errors" || errorsbox.IsInsideManagedErrorPath(root.FullPath) {
			summary.ErrorDirsSkipped++
			continue
		}
		if !root.IsAccessible {
			summary.RootsSkippedOff++
			summary.Warnings = append(summary.Warnings, fmt.Sprintf("Skipped offline root: %s (%s)", root.DisplayName, root.FullPath))
			continue
		}

		// Pre-pass map of largest video size per parent folder to provide multi-signal context for sample vs main movie
		maxVideoInDir := map[string]int64{}
		_ = filepath.WalkDir(root.FullPath, func(path string, d fs.DirEntry, walkInErr error) error {
			if walkInErr != nil {
				return nil
			}
			if d.IsDir() {
				if shouldSkipDirectory(path, root.FullPath, d.Name(), nil) {
					return filepath.SkipDir
				}
				return nil
			}
			_, kind, ok := ClassifyFileKind(d.Name())
			if !ok || kind != "video" {
				return nil
			}
			if info, err := d.Info(); err == nil {
				dirKey := filepath.Clean(filepath.Dir(path))
				if info.Size() > maxVideoInDir[dirKey] {
					maxVideoInDir[dirKey] = info.Size()
				}
			}
			return nil
		})

		seenRelPaths := map[string]bool{}
		walkErr := filepath.WalkDir(root.FullPath, func(path string, d fs.DirEntry, walkInErr error) error {
			select {
			case <-scanCtx.Done():
				return scanCtx.Err()
			default:
			}
			if walkInErr != nil {
				summary.Warnings = append(summary.Warnings, fmt.Sprintf("Unreadable entry %s: %v", path, walkInErr))
				return nil
			}
			if d.IsDir() {
				if shouldSkipDirectory(path, root.FullPath, d.Name(), &summary) {
					return filepath.SkipDir
				}
				return nil
			}

			ext, kind, ok := ClassifyFileKind(d.Name())
			if !ok {
				return nil
			}

			info, infoErr := d.Info()
			if infoErr != nil {
				summary.Warnings = append(summary.Warnings, fmt.Sprintf("Stat failed %s: %v", path, infoErr))
				return nil
			}

			relToMount, relErr := computeRelToMount(root.MountPath, path)
			if relErr != nil {
				return nil
			}
			parentRel := filepath.ToSlash(filepath.Dir(filepath.FromSlash(relToMount)))
			if parentRel == "." {
				parentRel = ""
			}
			seenRelPaths[relToMount] = true
			depth := strings.Count(strings.TrimPrefix(relToMount, root.RelativeRootPath), "/")

			integrity := "healthy"
			var partHash, fullHash, archiveJSON sql.NullString

			if info.Size() == 0 {
				integrity = "corrupted"
				summary.CorruptedDetected++
				summary.ZeroByteDetected++
			} else {
				ph, fErr := computePartialFingerprint(path, info.Size())
				if fErr != nil {
					integrity = "unreadable"
					summary.CorruptedDetected++
				} else {
					partHash = sql.NullString{String: ph, Valid: true}
				}
				if opts.ComputeHash && integrity == "healthy" && info.Size() <= 256*1024*1024 {
					if fh, hErr := computeFullSHA256(path); hErr == nil {
						fullHash = sql.NullString{String: fh, Valid: true}
					}
				}
				if kind == "archive_zip" && integrity == "healthy" {
					if manifest, zErr := inspectZipReadOnly(path); zErr == nil {
						archiveJSON = sql.NullString{String: manifest, Valid: true}
					}
				}
			}

			if kind == "shorts" {
				summary.ShortsDetected++
			}

			probeRes := MediaProbeResult{ProbeStatus: ProbeStatusNotApplicable}
			var assessment AnomalyAssessment

			if kind == "video" {
				dirKey := filepath.Clean(filepath.Dir(path))
				largestInDir := maxVideoInDir[dirKey]
				hasLargerFeature := largestInDir > 0 && info.Size() > 0 && largestInDir >= info.Size()*4 && largestInDir >= 50*1024*1024

				shouldRunProbe := probeEnabled
				if opts.ProbeVideos == nil {
					// Default automatic probing: run ffprobe on all real videos when prober is configured
					shouldRunProbe = prober != nil && info.Size() > 0
				}

				if info.Size() == 0 {
					probeRes = MediaProbeResult{ProbeStatus: ProbeStatusNotApplicable}
				} else if shouldRunProbe && prober != nil {
					probeRes = prober.ProbeVideo(scanCtx, path)
				} else {
					probeRes = MediaProbeResult{ProbeStatus: ProbeStatusUnavailableNeedsReview}
				}

				assessment = AssessVideoAnomaly(d.Name(), parentRel, info.Size(), probeRes, hasLargerFeature)
				if assessment.IsSampleClip {
					summary.SamplesDetected++
				}
				if assessment.AnomalyCategory == AnomalyConflictsNeedingReview {
					summary.ProbeNeedsReviewDetected++
				}
			} else {
				assessment = AssessNonVideoAnomaly(path, d.Name(), info.Size())
				if assessment.IsAppleDouble {
					summary.AppleDoubleDetected++
				} else if assessment.IsSystemArtifact {
					summary.SystemArtifactsDetected++
				}
			}

			isNew, fileID, upErr := s.upsertMediaFile(
				scanCtx, root, relToMount, parentRel, d.Name(), ext, kind,
				info.Size(), info.ModTime().Unix(), depth, integrity,
				partHash, fullHash, archiveJSON, probeRes, assessment,
			)
			if upErr != nil {
				summary.Warnings = append(summary.Warnings, fmt.Sprintf("DB index error %s: %v", d.Name(), upErr))
				return nil
			}
			summary.FilesDiscovered++
			if isNew {
				summary.NewFiles++
			} else {
				summary.UpdatedFiles++
			}

			if kind == "video" {
				_ = s.upsertVideoVersion(scanCtx, fileID, d.Name(), ext, info.Size(), integrity, probeRes, assessment)
			}

			if onProgress != nil && summary.FilesDiscovered%25 == 0 {
				onProgress(summary)
			}
			return nil
		})

		if errors.Is(walkErr, context.Canceled) {
			summary.State = "cancelled"
			summary.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
			summary.DurationMs = time.Since(start).Milliseconds()
			_, _ = s.sqlDB.ExecContext(context.Background(), `
				UPDATE background_jobs SET job_state = 'cancelled', processed_units = ?, updated_at = ?, completed_at = ?
				WHERE id = ?
			`, summary.FilesDiscovered, summary.CompletedAt, summary.CompletedAt, jobID)
			return summary, context.Canceled
		}

		missingCount := s.markMissingForRoot(scanCtx, root.ID, seenRelPaths)
		summary.MissingMarked += missingCount
		summary.RootsScanned++

		nowStr := time.Now().UTC().Format(time.RFC3339Nano)
		_, _ = s.sqlDB.ExecContext(scanCtx, `UPDATE disks SET last_scan_at = ?, updated_at = ? WHERE id = ?`, nowStr, nowStr, root.DiskID)
	}

	summary.State = "completed"
	summary.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
	summary.DurationMs = time.Since(start).Milliseconds()

	_, _ = s.sqlDB.ExecContext(context.Background(), `
		UPDATE background_jobs SET job_state = 'completed', processed_units = ?, total_units = ?, updated_at = ?, completed_at = ?
		WHERE id = ?
	`, summary.FilesDiscovered, summary.FilesDiscovered, summary.CompletedAt, summary.CompletedAt, jobID)

	_, _ = s.sqlDB.ExecContext(context.Background(), `
		INSERT INTO notifications (id, notification_type, title_ar, title_en, body_ar, body_en)
		VALUES (?, 'new_files_detected', 'اكتمل الفحص اليدوي للمكتبة', 'Manual Library Scan Completed', ?, ?)
	`, domain.NewImmutableID(),
		fmt.Sprintf("تم فحص %d مسار واكتشاف %d ملف (%d جديد، %d مفقود).", summary.RootsScanned, summary.FilesDiscovered, summary.NewFiles, summary.MissingMarked),
		fmt.Sprintf("Scanned %d root(s), indexed %d file(s) (%d new, %d marked missing).", summary.RootsScanned, summary.FilesDiscovered, summary.NewFiles, summary.MissingMarked),
	)

	return summary, nil
}

func shouldSkipDirectory(fullPath, rootFullPath, dirName string, summary *ScanSummary) bool {
	if filepath.Clean(fullPath) == filepath.Clean(rootFullPath) {
		return false
	}
	if errorsbox.IsManagedErrorDirName(dirName) {
		if summary != nil {
			summary.ErrorDirsSkipped++
		}
		return true
	}
	if IsSkippedThumbnailDir(dirName) {
		if summary != nil {
			summary.ThumbDirsSkipped++
		}
		return true
	}
	base := strings.ToLower(strings.TrimSpace(dirName))
	if base == "$recycle.bin" || base == "system volume information" || base == ".estiraha_trash" {
		return true
	}
	return false
}

func (s *Service) upsertMediaFile(
	ctx context.Context,
	root library.RootView,
	relPath, parentRel, filename, ext, kind string,
	sizeBytes, mtimeUnix int64,
	depth int,
	integrity string,
	partHash, fullHash, archiveJSON sql.NullString,
	probe MediaProbeResult,
	assessment AnomalyAssessment,
) (bool, string, error) {
	nowStr := time.Now().UTC().Format(time.RFC3339Nano)
	probeStatus := probe.ProbeStatus
	if probeStatus == "" {
		probeStatus = ProbeStatusNotApplicable
	}
	keepDec := assessment.KeepDecision
	if keepDec == "" {
		keepDec = "undecided"
	}

	var existingID, existingKeep string
	err := s.sqlDB.QueryRowContext(ctx, `
		SELECT id, keep_decision FROM media_files WHERE disk_id = ? AND relative_path = ?
	`, root.DiskID, relPath).Scan(&existingID, &existingKeep)

	if errors.Is(err, sql.ErrNoRows) {
		newID := domain.NewImmutableID()
		_, insErr := s.sqlDB.ExecContext(ctx, `
			INSERT INTO media_files (
				id, disk_id, root_id, relative_path, parent_rel_path, filename, extension, file_kind,
				size_bytes, mtime_unix, partial_fingerprint, sha256_hash, nesting_depth,
				archive_entries_json, integrity_status, presence_status, keep_decision,
				duration_seconds, video_codec, video_width, video_height,
				has_video_stream, has_audio_stream, probe_status,
				anomaly_category, anomaly_reason, is_apple_double, is_sample_clip, is_system_artifact,
				discovered_at, verified_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'present', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`,
			newID, root.DiskID, root.ID, relPath, parentRel, filename, ext, kind,
			sizeBytes, mtimeUnix, partHash, fullHash, depth,
			archiveJSON, integrity, keepDec,
			probe.DurationSeconds, probe.VideoCodec, probe.Width, probe.Height,
			boolToSQLInt(probe.HasVideoStream), boolToSQLInt(probe.HasAudioStream), probeStatus,
			assessment.AnomalyCategory, assessment.AnomalyReason,
			boolToSQLInt(assessment.IsAppleDouble), boolToSQLInt(assessment.IsSampleClip), boolToSQLInt(assessment.IsSystemArtifact),
			nowStr, nowStr, nowStr,
		)
		return true, newID, insErr
	}
	if err != nil {
		return false, "", err
	}

	// Preserve existing user/cross-disk keep decisions while updating anomaly flags when undecided
	effectiveKeep := existingKeep
	if existingKeep == "" || existingKeep == "undecided" {
		effectiveKeep = keepDec
	}

	_, upErr := s.sqlDB.ExecContext(ctx, `
		UPDATE media_files
		SET root_id = ?, parent_rel_path = ?, filename = ?, extension = ?, file_kind = ?,
		    size_bytes = ?, mtime_unix = ?,
		    partial_fingerprint = COALESCE(?, partial_fingerprint),
		    sha256_hash = COALESCE(?, sha256_hash),
		    nesting_depth = ?,
		    archive_entries_json = COALESCE(?, archive_entries_json),
		    integrity_status = ?, presence_status = 'present', keep_decision = ?,
		    duration_seconds = ?, video_codec = ?, video_width = ?, video_height = ?,
		    has_video_stream = ?, has_audio_stream = ?, probe_status = ?,
		    anomaly_category = ?, anomaly_reason = ?,
		    is_apple_double = ?, is_sample_clip = ?, is_system_artifact = ?,
		    verified_at = ?, updated_at = ?
		WHERE id = ?
	`,
		root.ID, parentRel, filename, ext, kind,
		sizeBytes, mtimeUnix,
		partHash, fullHash, depth, archiveJSON,
		integrity, effectiveKeep,
		probe.DurationSeconds, probe.VideoCodec, probe.Width, probe.Height,
		boolToSQLInt(probe.HasVideoStream), boolToSQLInt(probe.HasAudioStream), probeStatus,
		assessment.AnomalyCategory, assessment.AnomalyReason,
		boolToSQLInt(assessment.IsAppleDouble), boolToSQLInt(assessment.IsSampleClip), boolToSQLInt(assessment.IsSystemArtifact),
		nowStr, nowStr, existingID,
	)
	return false, existingID, upErr
}

func (s *Service) upsertVideoVersion(
	ctx context.Context,
	fileID, filename, ext string,
	sizeBytes int64,
	integrity string,
	probe MediaProbeResult,
	assessment AnomalyAssessment,
) error {
	resLabel, width, height := inferResolution(filename)
	vCodec := inferCodec(filename)
	aCodec := ""
	durationMs := int64(0)
	bitrateKbps := 0

	if probe.ProbeStatus == ProbeStatusOK {
		if probe.Width > 0 && probe.Height > 0 {
			width = probe.Width
			height = probe.Height
			resLabel = resolutionLabelFromDimensions(width, height, resLabel)
		}
		if probe.VideoCodec != "" {
			vCodec = normalizeProbedCodec(probe.VideoCodec)
		}
		aCodec = probe.AudioCodec
		if probe.DurationSeconds > 0 {
			durationMs = int64(probe.DurationSeconds * 1000)
		}
		bitrateKbps = probe.BitrateKbps
	}

	hdr := inferHDR(filename)
	score := computeQualityScore(resLabel, vCodec, hdr, sizeBytes, integrity)
	role := "primary"
	if integrity != "healthy" {
		role = "corrupted"
	} else if assessment.IsSampleClip {
		role = "lower_quality_extra"
		score = 0
	}

	var verID string
	err := s.sqlDB.QueryRowContext(ctx, `SELECT id FROM media_versions WHERE file_id = ?`, fileID).Scan(&verID)
	if errors.Is(err, sql.ErrNoRows) {
		verID = domain.NewImmutableID()
		_, err = s.sqlDB.ExecContext(ctx, `
			INSERT INTO media_versions (
				id, file_id, container_format, width, height, resolution_label,
				video_codec, hdr_format, bitrate_kbps, duration_ms, audio_codec,
				computed_quality_score, version_role
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, verID, fileID, strings.TrimPrefix(ext, "."), width, height, resLabel, vCodec, hdr, bitrateKbps, durationMs, aCodec, score, role)
		return err
	}
	if err != nil {
		return err
	}
	_, err = s.sqlDB.ExecContext(ctx, `
		UPDATE media_versions
		SET container_format = ?, width = ?, height = ?, resolution_label = ?,
		    video_codec = ?, hdr_format = ?, bitrate_kbps = ?, duration_ms = ?, audio_codec = ?,
		    computed_quality_score = ?,
		    version_role = CASE WHEN ? IN ('corrupted', 'lower_quality_extra') THEN ? ELSE version_role END
		WHERE id = ?
	`, strings.TrimPrefix(ext, "."), width, height, resLabel, vCodec, hdr, bitrateKbps, durationMs, aCodec, score, role, role, verID)
	return err
}

func (s *Service) markMissingForRoot(ctx context.Context, rootID string, seen map[string]bool) int {
	rows, err := s.sqlDB.QueryContext(ctx, `
		SELECT id, relative_path FROM media_files WHERE root_id = ? AND presence_status = 'present'
	`, rootID)
	if err != nil {
		return 0
	}
	type item struct{ id, rel string }
	var missing []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.id, &it.rel); err == nil && !seen[it.rel] {
			if errorsbox.IsInsideManagedErrorPath(it.rel) {
				continue
			}
			missing = append(missing, it)
		}
	}
	rows.Close()

	nowStr := time.Now().UTC().Format(time.RFC3339Nano)
	for _, m := range missing {
		_, _ = s.sqlDB.ExecContext(ctx, `
			UPDATE media_files SET presence_status = 'missing', updated_at = ? WHERE id = ?
		`, nowStr, m.id)
	}
	return len(missing)
}

func computeRelToMount(mountPath, fullFilePath string) (string, error) {
	rel, err := filepath.Rel(mountPath, fullFilePath)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

func computePartialFingerprint(path string, size int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	_, _ = fmt.Fprintf(h, "sz:%d:", size)
	const chunk = 64 * 1024
	buf := make([]byte, chunk)
	n, err := f.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	h.Write(buf[:n])

	if size > chunk*2 {
		if _, sErr := f.Seek(size-chunk, io.SeekStart); sErr == nil {
			n2, _ := f.Read(buf)
			h.Write(buf[:n2])
		}
	}
	return hex.EncodeToString(h.Sum(nil))[:32], nil
}

func computeFullSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func inspectZipReadOnly(path string) (string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer zr.Close()
	names := make([]string, 0, len(zr.File))
	for i, f := range zr.File {
		if i >= 50 {
			break
		}
		names = append(names, f.Name)
	}
	raw, _ := json.Marshal(names)
	return string(raw), nil
}

func inferResolution(name string) (string, int, int) {
	switch {
	case resRegex4K.MatchString(name):
		return "4K", 3840, 2160
	case resRegex1080.MatchString(name):
		return "1080p", 1920, 1080
	case resRegex720.MatchString(name):
		return "720p", 1280, 720
	case resRegex480.MatchString(name):
		return "480p", 854, 480
	default:
		return "1080p", 1920, 1080
	}
}

func resolutionLabelFromDimensions(w, h int, fallback string) string {
	switch {
	case w >= 3200 || h >= 1800:
		return "4K"
	case w >= 1800 || h >= 950:
		return "1080p"
	case w >= 1150 || h >= 650:
		return "720p"
	case w > 0 && h > 0:
		return "480p"
	default:
		return fallback
	}
}

func normalizeProbedCodec(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "hevc", "h265":
		return "HEVC"
	case "av1":
		return "AV1"
	case "h264", "avc", "avc1":
		return "H.264"
	case "rv40", "rv30":
		return "RealVideo"
	default:
		return strings.ToUpper(raw)
	}
}

func inferCodec(name string) string {
	switch {
	case codecAV1.MatchString(name):
		return "AV1"
	case codecHEVC.MatchString(name):
		return "HEVC"
	case codecH264.MatchString(name):
		return "H.264"
	default:
		return "H.264"
	}
}

func inferHDR(name string) string {
	m := strings.ToLower(hdrToken.FindString(name))
	switch m {
	case "hdr10+":
		return "HDR10+"
	case "hdr10":
		return "HDR10"
	case "dolbyvision", "dv":
		return "DolbyVision"
	case "hlg":
		return "HLG"
	default:
		return "SDR"
	}
}

func computeQualityScore(res, codec, hdr string, sizeBytes int64, integrity string) float64 {
	if integrity != "healthy" || sizeBytes <= 0 {
		return 0
	}
	score := 0.0
	switch res {
	case "4K":
		score += 400
	case "1080p":
		score += 300
	case "720p":
		score += 200
	default:
		score += 100
	}
	switch codec {
	case "AV1":
		score += 45
	case "HEVC":
		score += 35
	case "H.264":
		score += 25
	default:
		score += 10
	}
	if hdr != "SDR" {
		score += 25
	}
	gb := float64(sizeBytes) / (1024 * 1024 * 1024)
	if gb > 25 {
		gb = 25
	}
	score += gb
	return score
}

func boolToSQLInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
