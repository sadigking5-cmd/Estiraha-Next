package identify

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/sadigking5-cmd/Estiraha-Next/internal/domain"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/naming"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/scanner"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/secrets"
)

var (
	sxxExxRegex        = regexp.MustCompile(`(?i)\bS(\d{1,2})[\s._-]*E(\d{1,3})\b`)
	arabicEpRegex      = regexp.MustCompile(`(?:الحلقة|حلقة|ep|episode)[\s._-]*(\d{1,3})`)
	seasonDirRegex     = regexp.MustCompile(`(?i)^(?:s|season|الموسم|موسم)[\s._-]*(\d{1,2})$`)
	bareEpStemRegex    = regexp.MustCompile(`^(\d{1,3})$`)
	yearTokenRegex     = regexp.MustCompile(`\b(19\d{2}|20\d{2})\b`)
	promoImageBannerRe = regexp.MustCompile(`(?i)(?:^|[\s._\-\[\(])(?:www\.|http|yify|yts|usabit|rarbg|torrent|torrents|demonoid|1337x|extratorrent|kickass|piratebay|ettv|eztv|ganool|arabseed|akwam|mycima|cima4u|egybest)(?:[\s._\-\]\)]|$|\.)`)
)

var approvedCompanionSubdirNames = map[string]bool{
	"subs": true, "sub": true, "subtitle": true, "subtitles": true,
	"vobsubs": true, "vobsub": true, "idx": true, "srt": true,
	"other": true, "others": true, "extra": true, "extras": true,
	"artwork": true, "covers": true, "cover": true, "posters": true,
	"مجلد جديد": true, "ترجمة": true, "ترجمات": true, "ملحقات": true,
}

type IdentificationSummary struct {
	ProcessedVideos  int `json:"processedVideos"`
	MoviesIdentified int `json:"moviesIdentified"`
	SeriesIdentified int `json:"seriesIdentified"`
	EpisodesLinked   int `json:"episodesLinked"`
	NeedsReviewCount int `json:"needsReviewCount"`
	CompanionsLinked int `json:"companionsLinked"`
}

type TMDbCandidate struct {
	ExternalID     string  `json:"externalId"`
	MediaType      string  `json:"mediaType"`
	TitleOriginal  string  `json:"titleOriginal"`
	TitleAr        string  `json:"titleAr"`
	TitleEn        string  `json:"titleEn"`
	ProductionYear int     `json:"productionYear"`
	Overview       string  `json:"overview"`
	Confidence     float64 `json:"confidence"`
}

type Service struct {
	sqlDB      *sql.DB
	vault      secrets.Vault
	httpClient *http.Client
}

func NewService(sqlDB *sql.DB, vault secrets.Vault) *Service {
	return &Service{
		sqlDB:      sqlDB,
		vault:      vault,
		httpClient: &http.Client{Timeout: 8 * time.Second},
	}
}

type rawVideoRow struct {
	fileID       string
	diskID       string
	rootID       string
	rootRel      string
	rootCategory string
	relPath      string
	parentRel    string
	filename     string
	ext          string
}

func (s *Service) IdentifyLibrary(ctx context.Context) (IdentificationSummary, error) {
	var sum IdentificationSummary

	rows, err := s.sqlDB.QueryContext(ctx, `
		SELECT mf.id, mf.disk_id, COALESCE(mf.root_id, ''), COALESCE(lr.relative_root_path, ''),
		       COALESCE(lr.content_category, 'mixed'),
		       mf.relative_path, mf.parent_rel_path, mf.filename, mf.extension
		FROM media_files mf
		LEFT JOIN library_roots lr ON lr.id = mf.root_id
		WHERE mf.file_kind = 'video'
		  AND mf.presence_status = 'present'
		  AND mf.work_id IS NULL
		  AND mf.is_sample_clip = 0
		  AND mf.is_apple_double = 0
		  AND mf.anomaly_category NOT IN ('zero_byte_files', 'samples_and_short_clips', 'auxiliary_or_unknown')
		ORDER BY mf.relative_path ASC
	`)
	if err != nil {
		return sum, err
	}
	var videos []rawVideoRow
	for rows.Next() {
		var r rawVideoRow
		if err := rows.Scan(&r.fileID, &r.diskID, &r.rootID, &r.rootRel, &r.rootCategory, &r.relPath, &r.parentRel, &r.filename, &r.ext); err == nil {
			videos = append(videos, r)
		}
	}
	rows.Close()

	for _, v := range videos {
		sum.ProcessedVideos++
		isEpisodic, seriesRawTitle, seasonNum, epNum, epConf := detectEpisodicSignals(v)
		if isEpisodic {
			workID, epID, status, err := s.ensureSeriesAndEpisode(ctx, v, seriesRawTitle, seasonNum, epNum, epConf)
			if err != nil {
				continue
			}
			_, _ = s.sqlDB.ExecContext(ctx, `
				UPDATE media_files SET work_id = ?, episode_id = ?, classification_reason = 'multi_signal_episodic'
				WHERE id = ?
			`, workID, epID, v.fileID)
			_, _ = s.sqlDB.ExecContext(ctx, `
				UPDATE media_versions SET work_id = ?, episode_id = ? WHERE file_id = ?
			`, workID, epID, v.fileID)
			sum.EpisodesLinked++
			if status == "needs_review" {
				sum.NeedsReviewCount++
			} else {
				sum.SeriesIdentified++
			}
			sum.CompanionsLinked += s.linkEpisodicCompanions(ctx, v, workID, epID)
			continue
		}

		workID, workTitle, workYear, status, err := s.ensureStandaloneMovieOrWork(ctx, v)
		if err != nil {
			continue
		}
		_, _ = s.sqlDB.ExecContext(ctx, `
			UPDATE media_files SET work_id = ?, classification_reason = 'multi_signal_standalone'
			WHERE id = ?
		`, workID, v.fileID)
		_, _ = s.sqlDB.ExecContext(ctx, `
			UPDATE media_versions SET work_id = ? WHERE file_id = ?
		`, workID, v.fileID)
		if status == "needs_review" || status == "unknown_program" {
			sum.NeedsReviewCount++
		} else {
			sum.MoviesIdentified++
		}
		sum.CompanionsLinked += s.linkMovieBundleCompanions(ctx, v, workID, workTitle, workYear)
	}

	sum.CompanionsLinked += s.reconcileRemainingCompanionsAndOrphans(ctx)
	return sum, nil
}

func IsPromotionalBannerImage(filename string) bool {
	base := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	return promoImageBannerRe.MatchString(strings.ToLower(strings.TrimSpace(base)))
}

func isApprovedCompanionSubfolder(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	if approvedCompanionSubdirNames[lower] {
		return true
	}
	if strings.HasPrefix(lower, "cd") && len(lower) <= 4 {
		return true
	}
	if strings.HasPrefix(lower, "disc") && len(lower) <= 6 {
		return true
	}
	return false
}

func resolveMoviePackageDirRel(v rawVideoRow) (packageRel string, isDedicatedMovieDir bool) {
	parentNorm := strings.Trim(filepath.ToSlash(v.parentRel), "/")
	rootNorm := strings.Trim(filepath.ToSlash(v.rootRel), "/")
	if rootNorm == "." {
		rootNorm = ""
	}
	if parentNorm == "." {
		parentNorm = ""
	}
	if parentNorm == rootNorm {
		return parentNorm, false
	}

	relInsideRoot := parentNorm
	if rootNorm != "" && strings.HasPrefix(parentNorm, rootNorm+"/") {
		relInsideRoot = strings.TrimPrefix(parentNorm, rootNorm+"/")
	}
	segs := strings.Split(strings.Trim(relInsideRoot, "/"), "/")
	if len(segs) == 0 || segs[0] == "" {
		return parentNorm, false
	}
	if len(segs) >= 2 && isApprovedCompanionSubfolder(segs[len(segs)-1]) {
		upInside := strings.Join(segs[:len(segs)-1], "/")
		if rootNorm == "" {
			return upInside, true
		}
		return rootNorm + "/" + upInside, true
	}
	return parentNorm, true
}

type companionRow struct {
	id       string
	filename string
	ext      string
	kind     string
	size     int64
	relPath  string
	parent   string
}

func (s *Service) linkMovieBundleCompanions(ctx context.Context, v rawVideoRow, workID, workTitle string, workYear int) int {
	linked := 0
	videoStem := strings.ToLower(strings.TrimSuffix(v.filename, v.ext))
	workKey := naming.NormalizeWorkIdentityKey(workTitle, workYear)
	pkgRel, isDedicatedDir := resolveMoviePackageDirRel(v)

	var rows *sql.Rows
	var err error
	if isDedicatedDir {
		likePrefix := pkgRel + "/%"
		rows, err = s.sqlDB.QueryContext(ctx, `
			SELECT id, filename, extension, file_kind, size_bytes, relative_path, parent_rel_path
			FROM media_files
			WHERE disk_id = ?
			  AND (parent_rel_path = ? OR relative_path LIKE ?)
			  AND id != ?
			  AND presence_status = 'present'
			  AND file_kind IN ('subtitle', 'image', 'shorts')
			  AND work_id IS NULL
			  AND is_apple_double = 0
			  AND is_system_artifact = 0
			  AND anomaly_category NOT IN ('zero_byte_files', 'auxiliary_or_unknown', 'samples_and_short_clips')
		`, v.diskID, pkgRel, likePrefix, v.fileID)
	} else {
		rows, err = s.sqlDB.QueryContext(ctx, `
			SELECT id, filename, extension, file_kind, size_bytes, relative_path, parent_rel_path
			FROM media_files
			WHERE disk_id = ?
			  AND parent_rel_path = ?
			  AND id != ?
			  AND presence_status = 'present'
			  AND file_kind IN ('subtitle', 'image', 'shorts')
			  AND work_id IS NULL
			  AND is_apple_double = 0
			  AND is_system_artifact = 0
			  AND anomaly_category NOT IN ('zero_byte_files', 'auxiliary_or_unknown', 'samples_and_short_clips')
		`, v.diskID, v.parentRel, v.fileID)
	}
	if err != nil {
		return 0
	}

	var candidates []companionRow
	for rows.Next() {
		var c companionRow
		if err := rows.Scan(&c.id, &c.filename, &c.ext, &c.kind, &c.size, &c.relPath, &c.parent); err == nil {
			candidates = append(candidates, c)
		}
	}
	rows.Close()

	videoCountInPkg := 1
	if isDedicatedDir {
		_ = s.sqlDB.QueryRowContext(ctx, `
			SELECT COUNT(DISTINCT COALESCE(work_id, id))
			FROM media_files
			WHERE disk_id = ?
			  AND (parent_rel_path = ? OR relative_path LIKE ?)
			  AND file_kind = 'video'
			  AND presence_status = 'present'
			  AND is_sample_clip = 0
			  AND is_apple_double = 0
			  AND anomaly_category NOT IN ('zero_byte_files', 'samples_and_short_clips', 'auxiliary_or_unknown')
		`, v.diskID, pkgRel, pkgRel+"/%").Scan(&videoCountInPkg)
	}

	singleMovieFolder := isDedicatedDir && videoCountInPkg <= 1
	var acceptedImages []companionRow

	for _, c := range candidates {
		if isDedicatedDir && c.parent != pkgRel {
			subRel := strings.TrimPrefix(c.parent, pkgRel+"/")
			allApproved := true
			for _, seg := range strings.Split(subRel, "/") {
				if seg != "" && !isApprovedCompanionSubfolder(seg) {
					allApproved = false
					break
				}
			}
			if !allApproved {
				continue
			}
		}

		switch c.kind {
		case "image":
			if c.size <= 0 {
				continue
			}
			if IsPromotionalBannerImage(c.filename) {
				_, _ = s.sqlDB.ExecContext(ctx, `
					UPDATE media_files
					SET anomaly_category = ?, anomaly_reason = 'Confirmed promotional torrent/website banner image', is_system_artifact = 1, keep_decision = 'candidate_error_move'
					WHERE id = ?
				`, scanner.AnomalyAuxiliaryOrUnknown, c.id)
				continue
			}
			cStem := strings.ToLower(strings.TrimSuffix(c.filename, c.ext))
			if !singleMovieFolder {
				if !companionMatchesMovieIdentity(cStem, videoStem, workTitle, workYear, workKey) {
					continue
				}
			}
			acceptedImages = append(acceptedImages, c)

		case "subtitle":
			cStem := strings.TrimSuffix(c.filename, c.ext)
			if singleMovieFolder {
				if subtitleExplicitlyConflictsWithMovie(cStem, workTitle, workYear, workKey) {
					_, _ = s.sqlDB.ExecContext(ctx, `
						UPDATE media_files
						SET anomaly_category = ?, anomaly_reason = 'Subtitle filename references a different movie title/year inside folder; routed for user review', keep_decision = 'candidate_error_move'
						WHERE id = ?
					`, scanner.AnomalyConflictsNeedingReview, c.id)
					continue
				}
			} else {
				if !companionMatchesMovieIdentity(strings.ToLower(cStem), videoStem, workTitle, workYear, workKey) {
					continue
				}
			}

			_, _ = s.sqlDB.ExecContext(ctx, `UPDATE media_files SET work_id = ? WHERE id = ?`, workID, c.id)
			linked++
			s.registerSubtitleTrack(ctx, workID, nil, c.id, v.fileID, c.filename, c.ext)

		case "shorts":
			cStem := strings.TrimSuffix(c.filename, ".shorts")
			if singleMovieFolder {
				if subtitleExplicitlyConflictsWithMovie(cStem, workTitle, workYear, workKey) {
					continue
				}
			} else {
				if !companionMatchesMovieIdentity(strings.ToLower(cStem), videoStem, workTitle, workYear, workKey) {
					continue
				}
			}
			_, _ = s.sqlDB.ExecContext(ctx, `UPDATE media_files SET work_id = ?, keep_decision = 'keep' WHERE id = ?`, workID, c.id)
			linked++
		}
	}

	if len(acceptedImages) > 0 {
		sort.Slice(acceptedImages, func(i, j int) bool {
			ri := scorePosterCandidate(acceptedImages[i].filename, videoStem, workTitle, acceptedImages[i].parent == pkgRel)
			rj := scorePosterCandidate(acceptedImages[j].filename, videoStem, workTitle, acceptedImages[j].parent == pkgRel)
			if ri != rj {
				return ri > rj
			}
			return acceptedImages[i].size > acceptedImages[j].size
		})

		var currentPosterID string
		_ = s.sqlDB.QueryRowContext(ctx, `SELECT poster_rel_path FROM works WHERE id = ?`, workID).Scan(&currentPosterID)

		for idx, img := range acceptedImages {
			_, _ = s.sqlDB.ExecContext(ctx, `UPDATE media_files SET work_id = ? WHERE id = ?`, workID, img.id)
			linked++

			isPrimary := 0
			artKind := "extra_image"
			if idx == 0 && currentPosterID == "" {
				isPrimary = 1
				artKind = "poster"
				currentPosterID = img.id
				_, _ = s.sqlDB.ExecContext(ctx, `UPDATE works SET poster_rel_path = ? WHERE id = ?`, img.id, workID)
			} else if strings.Contains(strings.ToLower(img.filename), "backdrop") || strings.Contains(strings.ToLower(img.filename), "fanart") {
				artKind = "backdrop"
			}

			mime := "image/jpeg"
			switch strings.ToLower(img.ext) {
			case ".png":
				mime = "image/png"
			case ".webp":
				mime = "image/webp"
			}
			var existingArt string
			if aErr := s.sqlDB.QueryRowContext(ctx, `SELECT id FROM artwork_assets WHERE local_file_id = ?`, img.id).Scan(&existingArt); errors.Is(aErr, sql.ErrNoRows) {
				_, _ = s.sqlDB.ExecContext(ctx, `
					INSERT INTO artwork_assets (
						id, entity_type, entity_id, artwork_kind, origin_type, local_file_id,
						mime_type, width, height, byte_size, is_primary, is_protected_user_asset
					) VALUES (?, 'work', ?, ?, 'user_local', ?, ?, 600, 900, ?, ?, 1)
				`, domain.NewImmutableID(), workID, artKind, img.id, mime, img.size, isPrimary)
			}
		}
	}

	return linked
}

func scorePosterCandidate(filename, videoStemLower, workTitle string, inTopMovieDir bool) int {
	stem := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(filename, filepath.Ext(filename))))
	score := 0
	if inTopMovieDir {
		score += 20
	}
	switch stem {
	case "poster":
		score += 100
	case "folder":
		score += 95
	case "cover", "front":
		score += 90
	case "movie":
		score += 80
	}
	if videoStemLower != "" && (stem == videoStemLower || strings.HasPrefix(stem, videoStemLower)) {
		score += 85
	}
	cleanWorkLower := strings.ToLower(strings.TrimSpace(workTitle))
	if cleanWorkLower != "" && strings.Contains(stem, cleanWorkLower) {
		score += 70
	}
	if strings.Contains(stem, "backdrop") || strings.Contains(stem, "fanart") || strings.Contains(stem, "screenshot") || strings.Contains(stem, "screen") {
		score -= 25
	}
	return score
}

func companionMatchesMovieIdentity(compStemLower, videoStemLower, workTitle string, workYear int, workKey string) bool {
	if videoStemLower != "" && (compStemLower == videoStemLower || strings.HasPrefix(compStemLower, videoStemLower)) {
		return true
	}
	subYear, subRaw := extractTitleAndYear(compStemLower, "", "movies")
	if subYear > 0 && workYear > 0 && subYear != workYear {
		return false
	}
	candYear := workYear
	if subYear > 0 {
		candYear = subYear
	}
	candKey := naming.NormalizeWorkIdentityKey(subRaw, candYear)
	if workKey != "" && candKey == workKey {
		return true
	}
	cleanComp, err := naming.CleanTitle(compStemLower)
	if err == nil {
		if lang := naming.DetectSubtitleLanguageCode(compStemLower + ".srt"); lang != "" {
			cleanComp = strings.TrimSpace(strings.TrimSuffix(strings.ToLower(cleanComp), " "+lang))
		}
		if naming.NormalizeWorkIdentityKey(cleanComp, workYear) == workKey {
			return true
		}
	}
	return false
}

func subtitleExplicitlyConflictsWithMovie(rawStem, workTitle string, workYear int, workKey string) bool {
	trimmed := strings.TrimSpace(rawStem)
	if trimmed == "" {
		return false
	}
	if naming.DetectSubtitleLanguageCode(trimmed+".srt") != "" && len(strings.Fields(strings.ReplaceAll(strings.ReplaceAll(trimmed, ".", " "), "_", " "))) <= 2 {
		if !yearTokenRegex.MatchString(trimmed) {
			return false
		}
	}
	if ym := yearTokenRegex.FindStringSubmatch(trimmed); len(ym) >= 2 {
		subYr, _ := strconv.Atoi(ym[1])
		if workYear >= 1888 && subYr >= 1888 && subYr != workYear {
			if !strings.Contains(workTitle, ym[1]) {
				return true
			}
		}
	}
	return false
}

func (s *Service) registerSubtitleTrack(ctx context.Context, workID string, epNullable any, fileID, matchedVideoID, filename, ext string) {
	lang := naming.DetectSubtitleLanguageCode(filename)
	if lang == "" {
		lang = "und"
	}
	subFmt := strings.TrimPrefix(strings.ToLower(ext), ".")
	switch subFmt {
	case "srt", "ass", "ssa", "vtt", "pgs":
	case "sub", "idx":
		subFmt = "subrip"
	default:
		subFmt = "other"
	}
	var existingSub string
	if sErr := s.sqlDB.QueryRowContext(ctx, `SELECT id FROM subtitle_tracks WHERE file_id = ?`, fileID).Scan(&existingSub); errors.Is(sErr, sql.ErrNoRows) {
		_, _ = s.sqlDB.ExecContext(ctx, `
			INSERT INTO subtitle_tracks (
				id, work_id, episode_id, file_id, matched_video_file_id,
				source_type, format, language_code, is_matched_primary
			) VALUES (?, ?, ?, ?, ?, 'external_file', ?, ?, 1)
		`, domain.NewImmutableID(), workID, epNullable, fileID, matchedVideoID, subFmt, lang)
	}
}

func (s *Service) reconcileRemainingCompanionsAndOrphans(ctx context.Context) int {
	linked := 0

	type knownWork struct {
		workID   string
		diskID   string
		videoID  string
		title    string
		year     int
		identKey string
	}
	wRows, err := s.sqlDB.QueryContext(ctx, `
		SELECT w.id, mf.disk_id, mf.id, w.title_original, COALESCE(w.production_year, 0), w.normalized_identity_key
		FROM works w
		JOIN media_files mf ON mf.work_id = w.id AND mf.file_kind = 'video' AND mf.presence_status = 'present'
		WHERE w.sub_format = 'standalone'
	`)
	var known []knownWork
	if err == nil {
		for wRows.Next() {
			var kw knownWork
			if err := wRows.Scan(&kw.workID, &kw.diskID, &kw.videoID, &kw.title, &kw.year, &kw.identKey); err == nil {
				if kw.identKey == "" && kw.year > 0 {
					kw.identKey = naming.NormalizeWorkIdentityKey(kw.title, kw.year)
				}
				known = append(known, kw)
			}
		}
		wRows.Close()
	}

	uRows, err := s.sqlDB.QueryContext(ctx, `
		SELECT id, disk_id, filename, extension, file_kind, parent_rel_path
		FROM media_files
		WHERE work_id IS NULL
		  AND presence_status = 'present'
		  AND file_kind IN ('subtitle', 'image', 'shorts')
		  AND is_apple_double = 0
		  AND is_system_artifact = 0
		  AND anomaly_category = ''
	`)
	if err != nil {
		return 0
	}
	type unlinkedItem struct {
		id, diskID, filename, ext, kind, parentRel string
	}
	var unlinked []unlinkedItem
	for uRows.Next() {
		var u unlinkedItem
		if err := uRows.Scan(&u.id, &u.diskID, &u.filename, &u.ext, &u.kind, &u.parentRel); err == nil {
			unlinked = append(unlinked, u)
		}
	}
	uRows.Close()

	for _, u := range unlinked {
		if u.kind == "image" && IsPromotionalBannerImage(u.filename) {
			_, _ = s.sqlDB.ExecContext(ctx, `
				UPDATE media_files
				SET anomaly_category = ?, anomaly_reason = 'Confirmed promotional torrent/website banner image', is_system_artifact = 1, keep_decision = 'candidate_error_move'
				WHERE id = ?
			`, scanner.AnomalyAuxiliaryOrUnknown, u.id)
			continue
		}

		stem := strings.TrimSuffix(u.filename, u.ext)
		if u.kind == "shorts" {
			stem = strings.TrimSuffix(u.filename, ".shorts")
		}
		yr, rawTitle := extractTitleAndYear(stem, u.parentRel, "movies")
		var matchedWork *knownWork
		if yr >= 1888 {
			candKey := naming.NormalizeWorkIdentityKey(rawTitle, yr)
			for i := range known {
				if known[i].diskID == u.diskID && known[i].identKey == candKey {
					matchedWork = &known[i]
					break
				}
			}
		}

		if matchedWork != nil {
			if u.kind == "subtitle" {
				_, _ = s.sqlDB.ExecContext(ctx, `UPDATE media_files SET work_id = ? WHERE id = ?`, matchedWork.workID, u.id)
				s.registerSubtitleTrack(ctx, matchedWork.workID, nil, u.id, matchedWork.videoID, u.filename, u.ext)
				linked++
				continue
			}
			if u.kind == "shorts" {
				_, _ = s.sqlDB.ExecContext(ctx, `UPDATE media_files SET work_id = ?, keep_decision = 'keep' WHERE id = ?`, matchedWork.workID, u.id)
				linked++
				continue
			}
		}

		if u.kind == "subtitle" {
			var videosInDir int
			_ = s.sqlDB.QueryRowContext(ctx, `
				SELECT COUNT(*) FROM media_files
				WHERE disk_id = ? AND parent_rel_path = ? AND file_kind = 'video'
				  AND presence_status = 'present' AND is_sample_clip = 0 AND is_apple_double = 0
			`, u.diskID, u.parentRel).Scan(&videosInDir)
			if videosInDir == 0 {
				_, _ = s.sqlDB.ExecContext(ctx, `
					UPDATE media_files
					SET anomaly_category = ?, anomaly_reason = 'Orphan subtitle with no matching video file in folder or library root', keep_decision = 'candidate_error_move'
					WHERE id = ?
				`, scanner.AnomalyUnlinkedSubtitles, u.id)
			} else {
				_, _ = s.sqlDB.ExecContext(ctx, `
					UPDATE media_files
					SET anomaly_category = ?, anomaly_reason = 'Unmatched subtitle in multi-video directory; requires manual review before linking or moving', keep_decision = 'undecided'
					WHERE id = ?
				`, scanner.AnomalyUnlinkedSubtitles, u.id)
			}
		}
	}

	return linked
}

func detectEpisodicSignals(v rawVideoRow) (bool, string, int, int, float64) {
	stem := strings.TrimSuffix(v.filename, v.ext)
	parts := strings.Split(strings.Trim(v.parentRel, "/"), "/")
	lastDir := ""
	prevDir := ""
	if len(parts) > 0 {
		lastDir = parts[len(parts)-1]
	}
	if len(parts) > 1 {
		prevDir = parts[len(parts)-2]
	}

	if m := sxxExxRegex.FindStringSubmatch(stem); len(m) == 3 {
		sNum, _ := strconv.Atoi(m[1])
		eNum, _ := strconv.Atoi(m[2])
		prefix := strings.TrimSpace(stem[:strings.Index(strings.ToLower(stem), strings.ToLower(m[0]))])
		seriesTitle := prefix
		if seriesTitle == "" {
			if seasonDirRegex.MatchString(lastDir) && prevDir != "" {
				seriesTitle = prevDir
			} else {
				seriesTitle = lastDir
			}
		}
		return true, seriesTitle, maxInt(1, sNum), maxInt(1, eNum), 0.92
	}

	if sm := seasonDirRegex.FindStringSubmatch(strings.TrimSpace(lastDir)); len(sm) == 2 {
		sNum, _ := strconv.Atoi(sm[1])
		eNum := 0
		if bm := bareEpStemRegex.FindStringSubmatch(strings.TrimSpace(stem)); len(bm) == 2 {
			eNum, _ = strconv.Atoi(bm[1])
		} else if am := arabicEpRegex.FindStringSubmatch(strings.ToLower(stem)); len(am) == 2 {
			eNum, _ = strconv.Atoi(am[1])
		}
		if eNum > 0 && prevDir != "" {
			return true, prevDir, maxInt(1, sNum), eNum, 0.95
		}
	}

	if v.rootCategory == "series" || v.rootCategory == "anime" || v.rootCategory == "cartoons" {
		if am := arabicEpRegex.FindStringSubmatch(strings.ToLower(stem)); len(am) == 2 {
			eNum, _ := strconv.Atoi(am[1])
			seriesTitle := lastDir
			if seriesTitle == "" {
				seriesTitle = stem
			}
			return true, seriesTitle, 1, maxInt(1, eNum), 0.78
		}
		if bm := bareEpStemRegex.FindStringSubmatch(strings.TrimSpace(stem)); len(bm) == 2 && lastDir != "" {
			eNum, _ := strconv.Atoi(bm[1])
			return true, lastDir, 1, maxInt(1, eNum), 0.80
		}
	}

	return false, "", 0, 0, 0
}

func (s *Service) ensureSeriesAndEpisode(ctx context.Context, v rawVideoRow, rawSeriesTitle string, seasonNum, epNum int, conf float64) (string, string, string, error) {
	cleanedInput := yearTokenRegex.ReplaceAllString(rawSeriesTitle, " ")
	canonicalSeries, err := naming.FormatSeriesFolder(cleanedInput)
	if err != nil {
		canonicalSeries = strings.TrimSpace(rawSeriesTitle)
		conf = 0.45
	}

	mediaType := "series"
	switch v.rootCategory {
	case "anime":
		mediaType = "anime"
	case "cartoons":
		mediaType = "cartoon"
	case "documentaries":
		mediaType = "documentary_series"
	case "programs":
		mediaType = "tv_program"
	}

	isAr := containsArabic(canonicalSeries)
	titleAr, titleEn := "", ""
	origLang := "en"
	if isAr {
		titleAr = canonicalSeries
		origLang = "ar"
	} else {
		titleEn = canonicalSeries
	}

	status := "identified"
	if conf < 0.75 || canonicalSeries == "" {
		status = "needs_review"
	}

	identKey := naming.NormalizeWorkIdentityKey(canonicalSeries, 0)

	var workID string
	qErr := s.sqlDB.QueryRowContext(ctx, `
		SELECT id FROM works
		WHERE sub_format = 'episodic'
		  AND (lower(canonical_folder_name) = lower(?) OR (normalized_identity_key != '' AND normalized_identity_key = ?))
		LIMIT 1
	`, canonicalSeries, identKey).Scan(&workID)
	nowStr := time.Now().UTC().Format(time.RFC3339Nano)

	if errors.Is(qErr, sql.ErrNoRows) {
		workID = domain.NewImmutableID()
		_, err = s.sqlDB.ExecContext(ctx, `
			INSERT INTO works (
				id, media_type, sub_format, title_original, title_ar, title_en, sort_title,
				original_language, is_arabic_origin, canonical_folder_name, normalized_identity_key,
				identification_status, confidence_score, is_local_present, created_at, updated_at
			) VALUES (?, ?, 'episodic', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)
		`, workID, mediaType, canonicalSeries, titleAr, titleEn, canonicalSeries, origLang, boolToInt(isAr), canonicalSeries, identKey, status, conf, nowStr, nowStr)
		if err != nil {
			return "", "", "", err
		}
	} else if qErr != nil {
		return "", "", "", qErr
	}

	seasonCode, _ := naming.FormatSeasonFolder(seasonNum)
	var seasonID string
	sErr := s.sqlDB.QueryRowContext(ctx, `
		SELECT id FROM seasons WHERE work_id = ? AND season_number = ?
	`, workID, seasonNum).Scan(&seasonID)
	if errors.Is(sErr, sql.ErrNoRows) {
		seasonID = domain.NewImmutableID()
		if _, err := s.sqlDB.ExecContext(ctx, `
			INSERT INTO seasons (id, work_id, season_number, canonical_folder_code, title_ar, title_en, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, seasonID, workID, seasonNum, seasonCode, fmt.Sprintf("الموسم %d", seasonNum), fmt.Sprintf("Season %d", seasonNum), nowStr, nowStr); err != nil {
			return "", "", "", err
		}
	}

	epStem := fmt.Sprintf("%02d", epNum)
	var epID string
	eErr := s.sqlDB.QueryRowContext(ctx, `
		SELECT id FROM episodes WHERE season_id = ? AND episode_number = ?
	`, seasonID, epNum).Scan(&epID)
	if errors.Is(eErr, sql.ErrNoRows) {
		epID = domain.NewImmutableID()
		if _, err := s.sqlDB.ExecContext(ctx, `
			INSERT INTO episodes (id, work_id, season_id, episode_number, canonical_file_stem, title_original, title_ar, title_en, is_missing, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?)
		`, epID, workID, seasonID, epNum, epStem, fmt.Sprintf("Episode %d", epNum), fmt.Sprintf("الحلقة %d", epNum), fmt.Sprintf("Episode %d", epNum), nowStr, nowStr); err != nil {
			return "", "", "", err
		}
	} else {
		_, _ = s.sqlDB.ExecContext(ctx, `UPDATE episodes SET is_missing = 0, updated_at = ? WHERE id = ?`, nowStr, epID)
	}

	parts := strings.Split(strings.Trim(v.parentRel, "/"), "/")
	if len(parts) > 1 && seasonDirRegex.MatchString(parts[len(parts)-1]) {
		seriesParentRel := strings.Join(parts[:len(parts)-1], "/")
		pRows, pErr := s.sqlDB.QueryContext(ctx, `
			SELECT id, filename FROM media_files
			WHERE disk_id = ? AND parent_rel_path = ? AND file_kind = 'image'
			  AND is_apple_double = 0 AND is_system_artifact = 0 AND size_bytes > 0
			  AND presence_status = 'present'
		`, v.diskID, seriesParentRel)
		if pErr == nil {
			var chosenPosterID string
			for pRows.Next() {
				var pid, pname string
				if err := pRows.Scan(&pid, &pname); err == nil {
					if IsPromotionalBannerImage(pname) {
						continue
					}
					_, _ = s.sqlDB.ExecContext(ctx, `UPDATE media_files SET work_id = ? WHERE id = ? AND work_id IS NULL`, workID, pid)
					if chosenPosterID == "" {
						chosenPosterID = pid
					}
				}
			}
			pRows.Close()
			if chosenPosterID != "" {
				_, _ = s.sqlDB.ExecContext(ctx, `UPDATE works SET poster_rel_path = ? WHERE id = ? AND poster_rel_path = ''`, chosenPosterID, workID)
			}
		}
	}

	return workID, epID, status, nil
}

func (s *Service) ensureStandaloneMovieOrWork(ctx context.Context, v rawVideoRow) (string, string, int, string, error) {
	stem := strings.TrimSuffix(v.filename, v.ext)
	pkgRel, isDedicatedDir := resolveMoviePackageDirRel(v)
	parentFolder := ""
	if isDedicatedDir && pkgRel != "" {
		parts := strings.Split(strings.Trim(pkgRel, "/"), "/")
		parentFolder = parts[len(parts)-1]
	}

	if v.rootCategory == "programs" {
		plan, _ := naming.PlanProgramNaming(firstNonEmpty(parentFolder, stem), false)
		workID := domain.NewImmutableID()
		isAr := containsArabic(plan.FolderName)
		nowStr := time.Now().UTC().Format(time.RFC3339Nano)
		tAr, tEn := "", ""
		if isAr {
			tAr = plan.FolderName
		} else {
			tEn = plan.FolderName
		}
		_, err := s.sqlDB.ExecContext(ctx, `
			INSERT INTO works (
				id, media_type, sub_format, title_original, title_ar, title_en, sort_title,
				original_language, is_arabic_origin, canonical_folder_name, identification_status,
				confidence_score, is_local_present, created_at, updated_at
			) VALUES (?, 'tv_program', 'standalone', ?, ?, ?, ?, ?, ?, ?, 'unknown_program', 0.45, 1, ?, ?)
		`, workID, plan.FolderName, tAr, tEn, plan.FolderName, langCode(isAr), boolToInt(isAr), plan.FolderName, nowStr, nowStr)
		return workID, plan.FolderName, 0, "unknown_program", err
	}

	year, rawTitle := extractTitleAndYear(stem, parentFolder, v.rootCategory)
	cleanedTitle, err := naming.CleanTitle(rawTitle)
	if err != nil {
		cleanedTitle = strings.TrimSpace(stem)
	}

	conf := 0.55
	status := "needs_review"
	canonicalFolder := cleanedTitle
	var yearPtr any = nil
	identKey := ""

	if year >= 1888 && year <= 2100 && len([]rune(cleanedTitle)) >= 2 && !isGenericAmbiguousTitle(cleanedTitle) {
		if formatted, fErr := naming.FormatMovieName(cleanedTitle, year); fErr == nil {
			canonicalFolder = formatted
			yearPtr = year
			conf = 0.91
			status = "identified"
			identKey = naming.NormalizeWorkIdentityKey(cleanedTitle, year)
		}
	}

	mediaType := "movie"
	if v.rootCategory == "documentaries" {
		mediaType = "documentary_movie"
	}

	if yearPtr != nil {
		var existingWorkID, existingTitle string
		qErr := s.sqlDB.QueryRowContext(ctx, `
			SELECT id, title_original FROM works
			WHERE media_type = ?
			  AND production_year = ?
			  AND (lower(canonical_folder_name) = lower(?) OR (normalized_identity_key != '' AND normalized_identity_key = ?))
			LIMIT 1
		`, mediaType, year, canonicalFolder, identKey).Scan(&existingWorkID, &existingTitle)
		if qErr == nil && existingWorkID != "" {
			if strings.ContainsAny(cleanedTitle, "0123456789") && !strings.ContainsAny(existingTitle, "0123456789") {
				_, _ = s.sqlDB.ExecContext(ctx, `
					UPDATE works SET title_original = ?, title_en = CASE WHEN is_arabic_origin = 0 THEN ? ELSE title_en END,
					                 sort_title = ?, canonical_folder_name = ?
					WHERE id = ? AND identification_status != 'locked_by_user'
				`, cleanedTitle, cleanedTitle, cleanedTitle, canonicalFolder, existingWorkID)
				existingTitle = cleanedTitle
			}
			return existingWorkID, existingTitle, year, status, nil
		}
	}

	isAr := containsArabic(cleanedTitle)
	tAr, tEn := "", ""
	if isAr {
		tAr = cleanedTitle
	} else {
		tEn = cleanedTitle
	}

	workID := domain.NewImmutableID()
	nowStr := time.Now().UTC().Format(time.RFC3339Nano)
	_, insErr := s.sqlDB.ExecContext(ctx, `
		INSERT INTO works (
			id, media_type, sub_format, title_original, title_ar, title_en, sort_title,
			original_language, is_arabic_origin, production_year, canonical_folder_name,
			normalized_identity_key, identification_status, confidence_score, is_local_present, created_at, updated_at
		) VALUES (?, ?, 'standalone', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)
	`, workID, mediaType, cleanedTitle, tAr, tEn, cleanedTitle, langCode(isAr), boolToInt(isAr), yearPtr, canonicalFolder, identKey, status, conf, nowStr, nowStr)
	if insErr != nil {
		return "", "", 0, "", insErr
	}

	if isAr {
		_, _ = s.sqlDB.ExecContext(ctx, `INSERT OR IGNORE INTO work_countries (work_id, country_code) VALUES (?, 'EG')`, workID)
	}
	return workID, cleanedTitle, year, status, nil
}

func (s *Service) linkEpisodicCompanions(ctx context.Context, v rawVideoRow, workID, epID string) int {
	linked := 0
	videoStem := strings.ToLower(strings.TrimSuffix(v.filename, v.ext))

	rows, err := s.sqlDB.QueryContext(ctx, `
		SELECT id, filename, extension, file_kind, size_bytes, relative_path
		FROM media_files
		WHERE disk_id = ?
		  AND parent_rel_path = ?
		  AND id != ?
		  AND presence_status = 'present'
		  AND file_kind IN ('subtitle', 'image', 'shorts')
		  AND is_apple_double = 0
		  AND is_system_artifact = 0
		  AND anomaly_category NOT IN ('zero_byte_files', 'auxiliary_or_unknown', 'samples_and_short_clips')
	`, v.diskID, v.parentRel, v.fileID)
	if err != nil {
		return 0
	}
	var comps []companionRow
	for rows.Next() {
		var c companionRow
		if err := rows.Scan(&c.id, &c.filename, &c.ext, &c.kind, &c.size, &c.relPath); err == nil {
			comps = append(comps, c)
		}
	}
	rows.Close()

	for _, c := range comps {
		if c.kind == "image" && IsPromotionalBannerImage(c.filename) {
			continue
		}
		cStem := strings.ToLower(strings.TrimSuffix(c.filename, c.ext))
		if c.kind == "shorts" {
			cStem = strings.ToLower(strings.TrimSuffix(c.filename, ".shorts"))
		}
		isSeasonPoster := c.kind == "image" && (cStem == "poster" || cStem == "folder" || cStem == "cover")
		if !isSeasonPoster && !strings.HasPrefix(cStem, videoStem) {
			continue
		}

		var epNullable any = nil
		if epID != "" && !isSeasonPoster {
			epNullable = epID
		}

		_, _ = s.sqlDB.ExecContext(ctx, `
			UPDATE media_files SET work_id = ?, episode_id = COALESCE(?, episode_id) WHERE id = ?
		`, workID, epNullable, c.id)
		linked++

		if c.kind == "subtitle" {
			s.registerSubtitleTrack(ctx, workID, epNullable, c.id, v.fileID, c.filename, c.ext)
		}
	}
	return linked
}

func (s *Service) LookupTMDbOptional(ctx context.Context, query string, mediaType string, year int) ([]TMDbCandidate, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, errors.New("identify: search query cannot be empty")
	}
	apiKey, err := s.vault.Get("EstirahaNext/Providers/TMDb")
	if err != nil || strings.TrimSpace(apiKey) == "" {
		return []TMDbCandidate{}, nil
	}

	endpoint := "https://api.themoviedb.org/3/search/movie"
	if mediaType == "series" || mediaType == "tv" {
		endpoint = "https://api.themoviedb.org/3/search/tv"
	}
	u, _ := url.Parse(endpoint)
	params := u.Query()
	params.Set("api_key", strings.TrimSpace(apiKey))
	params.Set("query", q)
	params.Set("language", "ar-SA")
	if year > 1888 && endpoint == "https://api.themoviedb.org/3/search/movie" {
		params.Set("year", strconv.Itoa(year))
	}
	u.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tmdb status %d", resp.StatusCode)
	}

	var payload struct {
		Results []struct {
			ID            int    `json:"id"`
			Title         string `json:"title"`
			Name          string `json:"name"`
			OriginalTitle string `json:"original_title"`
			OriginalName  string `json:"original_name"`
			OriginalLang  string `json:"original_language"`
			ReleaseDate   string `json:"release_date"`
			FirstAirDate  string `json:"first_air_date"`
			Overview      string `json:"overview"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	out := make([]TMDbCandidate, 0, len(payload.Results))
	for i, r := range payload.Results {
		if i >= 8 {
			break
		}
		orig := firstNonEmpty(r.OriginalTitle, r.OriginalName)
		localized := firstNonEmpty(r.Title, r.Name)
		dateStr := firstNonEmpty(r.ReleaseDate, r.FirstAirDate)
		yr := 0
		if len(dateStr) >= 4 {
			yr, _ = strconv.Atoi(dateStr[:4])
		}
		c := TMDbCandidate{
			ExternalID:     strconv.Itoa(r.ID),
			MediaType:      mediaType,
			TitleOriginal:  orig,
			Overview:       r.Overview,
			ProductionYear: yr,
			Confidence:     0.90,
		}
		if r.OriginalLang == "ar" || containsArabic(orig) {
			c.TitleAr = orig
		} else {
			c.TitleEn = orig
			if containsArabic(localized) {
				c.TitleAr = localized
			}
		}
		out = append(out, c)
	}
	return out, nil
}

func extractTitleAndYear(stem, parentFolder, rootCategory string) (int, string) {
	if yr, pref, ok := splitLastYearWithNonEmptyPrefix(stem); ok {
		return yr, pref
	}
	lowerParent := strings.ToLower(filepath.Base(parentFolder))
	if parentFolder != "" && lowerParent != rootCategory && lowerParent != "movies" && lowerParent != "افلام" && lowerParent != "أفلام" {
		if yr, pref, ok := splitLastYearWithNonEmptyPrefix(parentFolder); ok {
			return yr, pref
		}
	}
	return 0, stem
}

func splitLastYearWithNonEmptyPrefix(raw string) (int, string, bool) {
	cleaned := strings.TrimSpace(raw)
	matches := yearTokenRegex.FindAllStringSubmatchIndex(cleaned, -1)
	for i := len(matches) - 1; i >= 0; i-- {
		m := matches[i]
		yr, _ := strconv.Atoi(cleaned[m[2]:m[3]])
		prefix := strings.TrimSpace(cleaned[:m[0]])
		if cleanPref, err := naming.CleanTitle(prefix); err == nil && cleanPref != "" {
			return yr, prefix, true
		}
	}
	return 0, "", false
}

func isGenericAmbiguousTitle(s string) bool {
	lower := strings.ToLower(strings.TrimSpace(s))
	switch lower {
	case "movie", "video", "sample", "track", "unknown", "new folder", "film", "فيلم", "فيديو", "نسخة":
		return true
	default:
		return false
	}
}

func containsArabic(s string) bool {
	for _, r := range s {
		if unicode.In(r, unicode.Arabic) {
			return true
		}
	}
	return false
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func langCode(isAr bool) string {
	if isAr {
		return "ar"
	}
	return "en"
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
