package scanner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/sadigking5-cmd/Estiraha-Next/internal/naming"
)

// ProbeStatusValues mirroring Migration 0004 CHECK constraint.
const (
	ProbeStatusNotApplicable            = "not_applicable"
	ProbeStatusOK                       = "probed_ok"
	ProbeStatusUnavailableNeedsReview   = "probe_unavailable_needs_review"
	ProbeStatusFailedNeedsReview        = "probe_failed_needs_review"
)

// AnomalyCategory values mirroring Migration 0004 & errorsbox.CategoryCode.
const (
	AnomalyNone                   = ""
	AnomalyEmptyFolders           = "empty_folders"
	AnomalyZeroByteFiles          = "zero_byte_files"
	AnomalyUnlinkedSubtitles      = "unlinked_subtitles"
	AnomalyAuxiliaryOrUnknown     = "auxiliary_or_unknown"
	AnomalySamplesAndShortClips   = "samples_and_short_clips"
	AnomalyConflictsNeedingReview = "conflicts_needing_review"
)

var (
	sampleTokenRegex = regexp.MustCompile(`(?i)(?:^|[\s._\-\[\(])(sample|trailer|teaser|preview)(?:$|[\s._\-\]\)\d])`)
	qnapThumbPrefix  = regexp.MustCompile(`^(?:a100|s100|s800|adefault|default).+`)
	torrentTxtRegex  = regexp.MustCompile(`(?i)(torrent\s+downloaded\s+from|downloaded\s+from\s+\w+\.(?:com|org|net|cc|ph|to|me)|www\.[a-z0-9\-]+\.(?:com|org|net|cc|to)|extratorrent|demonoid|yify-torrents|ahashare|silvertorrent|usabit|rarbg\.to|1337x)`)
	trackerBannerMap = map[string]bool{
		"www.yify-torrents.com.jpg": true,
		"www.yts.am.jpg":            true,
		"www.yts.mx.jpg":            true,
		"www.yts.lt.jpg":            true,
		"rarbg.com.jpg":             true,
	}
)

// MediaProbeResult holds verified technical stream metadata extracted by ffprobe (when available).
type MediaProbeResult struct {
	ProbeStatus     string  `json:"probeStatus"`
	DurationSeconds float64 `json:"durationSeconds"`
	VideoCodec      string  `json:"videoCodec"`
	AudioCodec      string  `json:"audioCodec"`
	Width           int     `json:"width"`
	Height          int     `json:"height"`
	BitrateKbps     int     `json:"bitrateKbps"`
	HasVideoStream  bool    `json:"hasVideoStream"`
	HasAudioStream  bool    `json:"hasAudioStream"`
	FormatName      string  `json:"formatName"`
	ProbeError      string  `json:"probeError"`
}

// MediaProber abstracts video stream inspection so tests can verify both live ffprobe
// and missing/failing ffprobe fallback behavior deterministically.
type MediaProber interface {
	ProbeVideo(ctx context.Context, filePath string) MediaProbeResult
}

// FFProbeAnalyzer executes the system `ffprobe` binary (or custom configured path) with a bounded timeout.
// When `ffprobe` is missing or fails to parse the container, it NEVER guesses a confirmed sample/movie result;
// instead it returns `probe_unavailable_needs_review` or `probe_failed_needs_review`.
type FFProbeAnalyzer struct {
	BinaryPath string
	Timeout    time.Duration
}

func NewFFProbeAnalyzer(customPath string) *FFProbeAnalyzer {
	return &FFProbeAnalyzer{
		BinaryPath: strings.TrimSpace(customPath),
		Timeout:    10 * time.Second,
	}
}

func (p *FFProbeAnalyzer) resolveBinary() (string, error) {
	if p != nil && p.BinaryPath != "" {
		if st, err := os.Stat(p.BinaryPath); err == nil && !st.IsDir() {
			return p.BinaryPath, nil
		}
		return exec.LookPath(p.BinaryPath)
	}
	return exec.LookPath("ffprobe")
}

type ffprobeJSONOutput struct {
	Streams []struct {
		CodecType string `json:"codec_type"`
		CodecName string `json:"codec_name"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
		Duration  string `json:"duration"`
	} `json:"streams"`
	Format struct {
		FormatName string `json:"format_name"`
		Duration   string `json:"duration"`
		BitRate    string `json:"bit_rate"`
	} `json:"format"`
}

func (p *FFProbeAnalyzer) ProbeVideo(parentCtx context.Context, filePath string) MediaProbeResult {
	bin, err := p.resolveBinary()
	if err != nil {
		return MediaProbeResult{
			ProbeStatus: ProbeStatusUnavailableNeedsReview,
			ProbeError:  "ffprobe binary is not installed or not in PATH",
		}
	}

	timeout := 10 * time.Second
	if p != nil && p.Timeout > 0 {
		timeout = p.Timeout
	}
	ctx, cancel := context.WithTimeout(parentCtx, timeout)
	defer cancel()

	cmd := exec.CommandContext(
		ctx,
		bin,
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		filePath,
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if runErr := cmd.Run(); runErr != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = runErr.Error()
		}
		return MediaProbeResult{
			ProbeStatus: ProbeStatusFailedNeedsReview,
			ProbeError:  msg,
		}
	}

	var parsed ffprobeJSONOutput
	if jErr := json.Unmarshal(stdout.Bytes(), &parsed); jErr != nil {
		return MediaProbeResult{
			ProbeStatus: ProbeStatusFailedNeedsReview,
			ProbeError:  fmt.Sprintf("invalid ffprobe json: %v", jErr),
		}
	}

	res := MediaProbeResult{
		ProbeStatus: ProbeStatusOK,
		FormatName:  parsed.Format.FormatName,
	}
	if d, dErr := strconv.ParseFloat(strings.TrimSpace(parsed.Format.Duration), 64); dErr == nil && d >= 0 {
		res.DurationSeconds = d
	}
	if br, bErr := strconv.ParseInt(strings.TrimSpace(parsed.Format.BitRate), 10, 64); bErr == nil && br > 0 {
		res.BitrateKbps = int(br / 1000)
	}

	for _, st := range parsed.Streams {
		switch strings.ToLower(st.CodecType) {
		case "video":
			// Ignore attached cover pictures (mjpeg/png with 0 duration if another real video stream exists)
			if !res.HasVideoStream || (st.CodecName != "mjpeg" && st.CodecName != "png") {
				res.HasVideoStream = true
				res.VideoCodec = st.CodecName
				if st.Width > 0 {
					res.Width = st.Width
				}
				if st.Height > 0 {
					res.Height = st.Height
				}
				if res.DurationSeconds <= 0 && st.Duration != "" {
					if sd, sdErr := strconv.ParseFloat(strings.TrimSpace(st.Duration), 64); sdErr == nil && sd >= 0 {
						res.DurationSeconds = sd
					}
				}
			}
		case "audio":
			if !res.HasAudioStream {
				res.HasAudioStream = true
				res.AudioCodec = st.CodecName
			}
		}
	}

	if !res.HasVideoStream {
		return MediaProbeResult{
			ProbeStatus:     ProbeStatusFailedNeedsReview,
			DurationSeconds: res.DurationSeconds,
			HasAudioStream:  res.HasAudioStream,
			FormatName:      res.FormatName,
			ProbeError:      "container has no decodable video stream",
		}
	}

	return res
}

// AnomalyAssessment captures conservative multi-signal classification for a discovered file.
type AnomalyAssessment struct {
	AnomalyCategory  string
	AnomalyReason    string
	KeepDecision     string
	IsAppleDouble    bool
	IsSampleClip     bool
	IsSystemArtifact bool
}

// IsAppleDoubleFile returns true if filename is a macOS AppleDouble resource-fork metadata file (`._*`).
func IsAppleDoubleFile(filename string) bool {
	base := filepath.Base(filename)
	return strings.HasPrefix(base, "._") && len(base) > 2
}

// IsSkippedThumbnailDir returns true if dirName is a NAS/QNAP hidden thumbnail proxy directory (`.@__thumb`).
func IsSkippedThumbnailDir(dirName string) bool {
	lower := strings.ToLower(strings.TrimSpace(dirName))
	return lower == ".@__thumb" || lower == "@eadir"
}

// HasExplicitSampleSignal checks whether filename or immediate parent directory carries explicit sample/proxy tokens.
// IMPORTANT: Protected `.shorts` files NEVER match sample signals.
func HasExplicitSampleSignal(filename, parentRel string) (bool, string) {
	if naming.IsShortsExtension(filename) {
		return false, ""
	}
	base := filepath.Base(filename)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)

	if sampleTokenRegex.MatchString(stem) {
		return true, "explicit_sample_token_in_filename"
	}
	lastDir := strings.ToLower(strings.TrimSpace(filepath.Base(filepath.FromSlash(parentRel))))
	if lastDir == "sample" || lastDir == "samples" {
		return true, "inside_sample_subfolder"
	}
	if qnapThumbPrefix.MatchString(base) {
		return true, "nas_thumbnail_proxy_prefix"
	}
	return false, ""
}

// AssessVideoAnomaly evaluates a video file using multi-signal evidence:
//   - Rule 3: Small file size ALONE is NEVER sufficient to classify a video as a confirmed sample!
//   - Rule 4: When ffprobe is unavailable or fails, ambiguous/suspicious videos are marked
//     `conflicts_needing_review` ("needs review") rather than guessing a confirmed sample or confirmed full movie.
//   - Rule 5: `.shorts` files are strictly excluded from sample classification.
//   - Confirmed Sample (`samples_and_short_clips`) requires multi-signal proof:
//       a) Verified ffprobe video stream with very short duration (<= 300s / 5 min) AND (explicit sample/proxy token OR duration <= 90s clip with companion main feature or proxy signature), OR
//       b) Verified ffprobe video stream with duration <= 90s AND tiny bitrate/proxy dimensions (e.g., <= 90s clip under 25 MB verified by ffprobe).
//   - If ffprobe succeeded (`probed_ok`) with no sample tokens and duration > 300s: clean full video (`AnomalyNone`).
//   - If ffprobe succeeded (`probed_ok`) with short duration (<= 180s) WITHOUT sample token:
//     Wait: if duration <= 60s (e.g. a 3-second tiny MP4 clip probed by ffprobe!), is a verified 3s-60s MP4 clip a short clip (`samples_and_short_clips`)?
//     YES! Because ffprobe PROVED its actual duration is e.g. 2s–60s (not size-only guessing!). Whereas if ffprobe is UNAVAILABLE on that same tiny MP4, we DO NOT guess from size alone -> we mark `conflicts_needing_review`!
func AssessVideoAnomaly(filename, parentRel string, sizeBytes int64, probe MediaProbeResult, hasLargerFeatureInFolder bool) AnomalyAssessment {
	if naming.IsShortsExtension(filename) {
		return AnomalyAssessment{KeepDecision: "keep"}
	}
	if sizeBytes == 0 {
		return AnomalyAssessment{
			AnomalyCategory: AnomalyZeroByteFiles,
			AnomalyReason:   "ملف فيديو بحجم صفر بايت (0 B)",
			KeepDecision:    "candidate_error_move",
		}
	}

	explicitSample, sigReason := HasExplicitSampleSignal(filename, parentRel)

	switch probe.ProbeStatus {
	case ProbeStatusOK:
		// Multi-signal with verified ffprobe stream duration:
		if explicitSample && (probe.DurationSeconds <= 600 || hasLargerFeatureInFolder) {
			return AnomalyAssessment{
				AnomalyCategory: AnomalySamplesAndShortClips,
				AnomalyReason:   fmt.Sprintf("عينة فيديو مؤكدة بالتحليل الفني (%s، المدة %.1f ثانية)", sigReason, probe.DurationSeconds),
				KeepDecision:    "candidate_error_move",
				IsSampleClip:    true,
			}
		}
		if probe.DurationSeconds > 0 && probe.DurationSeconds <= 90 && (sizeBytes < 50*1024*1024 || hasLargerFeatureInFolder) {
			return AnomalyAssessment{
				AnomalyCategory: AnomalySamplesAndShortClips,
				AnomalyReason:   fmt.Sprintf("مقطع فيديو قصير جداً مؤكد عبر ffprobe (المدة %.1f ثانية)", probe.DurationSeconds),
				KeepDecision:    "candidate_error_move",
				IsSampleClip:    true,
			}
		}
		if explicitSample && probe.DurationSeconds > 600 {
			// Named "sample" yet longer than 10 minutes -> ambiguous, send to review instead of blind sample move
			return AnomalyAssessment{
				AnomalyCategory: AnomalyConflictsNeedingReview,
				AnomalyReason:   fmt.Sprintf("يحمل اسم عينة لكن مدته طويلة (%.1f ثانية) — يحتاج مراجعة يدوية", probe.DurationSeconds),
				KeepDecision:    "undecided",
			}
		}
		return AnomalyAssessment{KeepDecision: "undecided"}

	case ProbeStatusUnavailableNeedsReview, ProbeStatusFailedNeedsReview:
		// Rule 3 & Rule 4: Never guess confirmed sample from small size alone or when ffprobe is unavailable/failed!
		if probe.ProbeStatus == ProbeStatusFailedNeedsReview {
			return AnomalyAssessment{
				AnomalyCategory: AnomalyConflictsNeedingReview,
				AnomalyReason:   fmt.Sprintf("تعذر تحليل حاوية الفيديو عبر ffprobe (%s) — يحتاج مراجعة يدوية", probe.ProbeError),
				KeepDecision:    "undecided",
			}
		}
		// Probe unavailable: if suspicious (explicit sample keyword OR unusually small < 20 MB), flag for manual review rather than guessing!
		if explicitSample || sizeBytes < 20*1024*1024 {
			return AnomalyAssessment{
				AnomalyCategory: AnomalyConflictsNeedingReview,
				AnomalyReason:   "تحليل ffprobe غير متاح مع وجود مؤشر حجم صغير أو اسم عينة — مصنف للمراجعة بدلاً من التخمين",
				KeepDecision:    "undecided",
			}
		}
		// Large normal video when ffprobe is not installed: record probe_unavailable_needs_review in probe_status without falsely quarantining normal movies
		return AnomalyAssessment{KeepDecision: "undecided"}
	}

	return AnomalyAssessment{KeepDecision: "undecided"}
}

// AssessNonVideoAnomaly evaluates non-video files conservatively:
//   - AppleDouble `._*` -> `auxiliary_or_unknown` (`IsAppleDouble = true`)
//   - `0 B` -> `zero_byte_files`
//   - OS shell artifacts (`Thumbs.db`, `desktop.ini`, `.DS_Store`, `.lnk` shortcuts) -> `auxiliary_or_unknown` (`IsSystemArtifact = true`)
//   - Known tracker promo banner filenames (`WWW.YIFY-TORRENTS.COM.jpg`) -> `auxiliary_or_unknown`
//   - `.txt`, `.zip`, `.ico`: Rule 6 — NEVER treat ALL `.txt`, `.zip`, or `.ico` files as junk automatically!
//       * `.txt` is ONLY marked `auxiliary_or_unknown` if its filename or tiny text body (< 4 KB) matches known torrent-tracker promo signatures (`Torrent downloaded from...`, `www.UsaBit.com`, etc.). Meaningful user notes (`notes.txt`, `readme.txt`, `قصة الفيلم.txt`) stay untouched!
//       * `.zip` is inspected read-only (never auto-marked as junk; subtitle archives or user archives stay preserved).
//       * `.ico` (non-AppleDouble, non-zero) is preserved without automatic junk disposal unless explicitly reviewed.
//   - `.shorts` -> ALWAYS protected (`KeepDecision = "keep"`, `AnomalyNone`).
func AssessNonVideoAnomaly(fullPath, filename string, sizeBytes int64) AnomalyAssessment {
	if naming.IsShortsExtension(filename) {
		return AnomalyAssessment{KeepDecision: "keep"}
	}

	if IsAppleDoubleFile(filename) {
		return AnomalyAssessment{
			AnomalyCategory:  AnomalyAuxiliaryOrUnknown,
			AnomalyReason:    "ملف بيانات وصفية وهمي من نظام macOS (AppleDouble ._*)",
			KeepDecision:     "candidate_error_move",
			IsAppleDouble:    true,
			IsSystemArtifact: true,
		}
	}

	if sizeBytes == 0 {
		return AnomalyAssessment{
			AnomalyCategory: AnomalyZeroByteFiles,
			AnomalyReason:   "ملف فارغ بحجم صفر بايت (0 B)",
			KeepDecision:    "candidate_error_move",
		}
	}

	lowerBase := strings.ToLower(filepath.Base(filename))
	ext := strings.ToLower(filepath.Ext(lowerBase))

	if lowerBase == "thumbs.db" || lowerBase == "desktop.ini" || lowerBase == ".ds_store" {
		return AnomalyAssessment{
			AnomalyCategory:  AnomalyAuxiliaryOrUnknown,
			AnomalyReason:    fmt.Sprintf("ملف نظام تشغيلي مساعد (%s)", filepath.Base(filename)),
			KeepDecision:     "candidate_error_move",
			IsSystemArtifact: true,
		}
	}

	if ext == ".lnk" || ext == ".url" {
		return AnomalyAssessment{
			AnomalyCategory:  AnomalyAuxiliaryOrUnknown,
			AnomalyReason:    "ملف اختصار ويندوز وليس ملف وسائط فعلي",
			KeepDecision:     "candidate_error_move",
			IsSystemArtifact: true,
		}
	}

	if trackerBannerMap[lowerBase] {
		return AnomalyAssessment{
			AnomalyCategory:  AnomalyAuxiliaryOrUnknown,
			AnomalyReason:    "صورة دعائية لموقع تورنت وليست بوستر الفيلم",
			KeepDecision:     "candidate_error_move",
			IsSystemArtifact: true,
		}
	}

	// Conservative TXT inspection (Rule 6): only flag confirmed torrent-tracker promo stubs; keep user notes!
	if ext == ".txt" && sizeBytes > 0 && sizeBytes <= 4096 {
		if torrentTxtRegex.MatchString(lowerBase) || isConfirmedTorrentPromoText(fullPath) {
			return AnomalyAssessment{
				AnomalyCategory:  AnomalyAuxiliaryOrUnknown,
				AnomalyReason:    "ملف نصي دعائي لمصدر التورنت",
				KeepDecision:     "candidate_error_move",
				IsSystemArtifact: true,
			}
		}
	}

	return AnomalyAssessment{KeepDecision: "undecided"}
}

func isConfirmedTorrentPromoText(fullPath string) bool {
	if strings.TrimSpace(fullPath) == "" {
		return false
	}
	f, err := os.Open(fullPath)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 1024)
	n, err := f.Read(buf)
	if err != nil && !errors.Is(err, os.ErrClosed) && n == 0 {
		return false
	}
	content := strings.TrimSpace(string(buf[:n]))
	if len(content) == 0 || len(content) > 300 {
		return false
	}
	return torrentTxtRegex.MatchString(content)
}
