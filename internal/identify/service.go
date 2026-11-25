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
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/sadigking5-cmd/Estiraha-Next/internal/domain"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/naming"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/secrets"
)

var (
	sxxExxRegex     = regexp.MustCompile(`(?i)\bS(\d{1,2})[\s._-]*E(\d{1,3})\b`)
	arabicEpRegex   = regexp.MustCompile(`(?:الحلقة|حلقة|ep|episode)[\s._-]*(\d{1,3})`)
	seasonDirRegex  = regexp.MustCompile(`(?i)^(?:s|season|الموسم|موسم)[\s._-]*(\d{1,2})$`)
	bareEpStemRegex = regexp.MustCompile(`^(\d{1,3})$`)
	yearTokenRegex  = regexp.MustCompile(`\b(19\d{2}|20\d{2})\b`)
)

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
	rootCategory string
	relPath      string
	parentRel    string
	filename     string
	ext          string
}

func (s *Service) IdentifyLibrary(ctx context.Context) (IdentificationSummary, error) {
	var sum IdentificationSummary

	rows, err := s.sqlDB.QueryContext(ctx, `
		SELECT mf.id, mf.disk_id, COALESCE(mf.root_id, ''), COALESCE(lr.content_category, 'mixed'),
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
		if err := rows.Scan(&r.fileID, &r.diskID, &r.rootID, &r.rootCategory, &r.relPath, &r.parentRel, &r.filename, &r.ext); err == nil {
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
			sum.CompanionsLinked += s.linkCompanionsInFolder(ctx, v, workID, epID, true)
			continue
		}

		workID, status, err := s.ensureStandaloneMovieOrWork(ctx, v)
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
		sum.CompanionsLinked += s.linkCompanionsInFolder(ctx, v, workID, "", false)
	}

	return sum, nil
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

	var workID string
	qErr := s.sqlDB.QueryRowContext(ctx, `
		SELECT id FROM works WHERE sub_format = 'episodic' AND lower(canonical_folder_name) = lower(?) LIMIT 1
	`, canonicalSeries).Scan(&workID)
	nowStr := time.Now().UTC().Format(time.RFC3339Nano)

	if errors.Is(qErr, sql.ErrNoRows) {
		workID = domain.NewImmutableID()
		_, err = s.sqlDB.ExecContext(ctx, `
			INSERT INTO works (
				id, media_type, sub_format, title_original, title_ar, title_en, sort_title,
				original_language, is_arabic_origin, canonical_folder_name, identification_status,
				confidence_score, is_local_present, created_at, updated_at
			) VALUES (?, ?, 'episodic', ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)
		`, workID, mediaType, canonicalSeries, titleAr, titleEn, canonicalSeries, origLang, boolToInt(isAr), canonicalSeries, status, conf, nowStr, nowStr)
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

	// Link series-level poster in parent series folder if present (`Series Name/poster.jpg`)
	parts := strings.Split(strings.Trim(v.parentRel, "/"), "/")
	if len(parts) > 1 && seasonDirRegex.MatchString(parts[len(parts)-1]) {
		seriesParentRel := strings.Join(parts[:len(parts)-1], "/")
		_, _ = s.sqlDB.ExecContext(ctx, `
			UPDATE media_files SET work_id = ? WHERE disk_id = ? AND parent_rel_path = ? AND file_kind = 'image' AND work_id IS NULL
		`, workID, v.diskID, seriesParentRel)
		var pFileID string
		if pErr := s.sqlDB.QueryRowContext(ctx, `
			SELECT id FROM media_files WHERE disk_id = ? AND parent_rel_path = ? AND file_kind = 'image' LIMIT 1
		`, v.diskID, seriesParentRel).Scan(&pFileID); pErr == nil && pFileID != "" {
			_, _ = s.sqlDB.ExecContext(ctx, `UPDATE works SET poster_rel_path = ? WHERE id = ? AND poster_rel_path = ''`, pFileID, workID)
		}
	}

	return workID, epID, status, nil
}

func (s *Service) ensureStandaloneMovieOrWork(ctx context.Context, v rawVideoRow) (string, string, error) {
	stem := strings.TrimSuffix(v.filename, v.ext)
	parts := strings.Split(strings.Trim(v.parentRel, "/"), "/")
	parentFolder := ""
	if len(parts) > 0 {
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
		return workID, "unknown_program", err
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

	if year >= 1888 && year <= 2100 && len([]rune(cleanedTitle)) >= 2 && !isGenericAmbiguousTitle(cleanedTitle) {
		if formatted, fErr := naming.FormatMovieName(cleanedTitle, year); fErr == nil {
			canonicalFolder = formatted
			yearPtr = year
			conf = 0.91
			status = "identified"
		}
	}

	mediaType := "movie"
	if v.rootCategory == "documentaries" {
		mediaType = "documentary_movie"
	}

	var existingWorkID string
	if yearPtr != nil {
		qErr := s.sqlDB.QueryRowContext(ctx, `
			SELECT id FROM works WHERE media_type = ? AND lower(canonical_folder_name) = lower(?) LIMIT 1
		`, mediaType, canonicalFolder).Scan(&existingWorkID)
		if qErr == nil && existingWorkID != "" {
			return existingWorkID, status, nil
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
			identification_status, confidence_score, is_local_present, created_at, updated_at
		) VALUES (?, ?, 'standalone', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)
	`, workID, mediaType, cleanedTitle, tAr, tEn, cleanedTitle, langCode(isAr), boolToInt(isAr), yearPtr, canonicalFolder, status, conf, nowStr, nowStr)
	if insErr != nil {
		return "", "", insErr
	}

	if isAr {
		_, _ = s.sqlDB.ExecContext(ctx, `INSERT OR IGNORE INTO work_countries (work_id, country_code) VALUES (?, 'EG')`, workID)
	}
	return workID, status, nil
}

func (s *Service) linkCompanionsInFolder(ctx context.Context, v rawVideoRow, workID, epID string, isEpisodic bool) int {
	linked := 0
	videoStem := strings.ToLower(strings.TrimSuffix(v.filename, v.ext))

	rows, err := s.sqlDB.QueryContext(ctx, `
		SELECT id, filename, extension, file_kind, size_bytes, relative_path
		FROM media_files
		WHERE disk_id = ?
		  AND parent_rel_path = ?
		  AND id != ?
		  AND presence_status = 'present'
		  AND file_kind != 'video'
		  AND is_apple_double = 0
		  AND is_system_artifact = 0
		  AND anomaly_category NOT IN ('zero_byte_files', 'auxiliary_or_unknown', 'samples_and_short_clips')
	`, v.diskID, v.parentRel, v.fileID)
	if err != nil {
		return 0
	}
	type comp struct {
		id, filename, ext, kind, rel string
		size                         int64
	}
	var comps []comp
	for rows.Next() {
		var c comp
		if err := rows.Scan(&c.id, &c.filename, &c.ext, &c.kind, &c.size, &c.rel); err == nil {
			comps = append(comps, c)
		}
	}
	rows.Close()

	for _, c := range comps {
		cStem := strings.ToLower(strings.TrimSuffix(c.filename, c.ext))
		if isEpisodic {
			isSeasonPoster := c.kind == "image" && (cStem == "poster" || cStem == "folder" || cStem == "cover")
			if !isSeasonPoster && !strings.HasPrefix(cStem, videoStem) {
				continue
			}
		}

		var epNullable any = nil
		if epID != "" && !(c.kind == "image" && (cStem == "poster" || cStem == "folder")) {
			epNullable = epID
		}

		_, _ = s.sqlDB.ExecContext(ctx, `
			UPDATE media_files SET work_id = ?, episode_id = COALESCE(?, episode_id) WHERE id = ?
		`, workID, epNullable, c.id)
		linked++

		if c.kind == "subtitle" {
			lang := strings.TrimPrefix(naming.ExtractLanguageSuffix(c.filename), ".")
			if lang == "" {
				lang = "ar"
			}
			subFmt := strings.TrimPrefix(c.ext, ".")
			if subFmt == "sub" {
				subFmt = "subrip"
			}
			var existingSub string
			if sErr := s.sqlDB.QueryRowContext(ctx, `SELECT id FROM subtitle_tracks WHERE file_id = ?`, c.id).Scan(&existingSub); errors.Is(sErr, sql.ErrNoRows) {
				_, _ = s.sqlDB.ExecContext(ctx, `
					INSERT INTO subtitle_tracks (
						id, work_id, episode_id, file_id, matched_video_file_id,
						source_type, format, language_code, is_matched_primary
					) VALUES (?, ?, ?, ?, ?, 'external_file', ?, ?, 1)
				`, domain.NewImmutableID(), workID, epNullable, c.id, v.fileID, subFmt, lang)
			}
		}

		if c.kind == "image" && c.size > 0 {
			mime := "image/jpeg"
			switch c.ext {
			case ".png":
				mime = "image/png"
			case ".webp":
				mime = "image/webp"
			}
			var existingArt string
			if aErr := s.sqlDB.QueryRowContext(ctx, `SELECT id FROM artwork_assets WHERE local_file_id = ?`, c.id).Scan(&existingArt); errors.Is(aErr, sql.ErrNoRows) {
				_, _ = s.sqlDB.ExecContext(ctx, `
					INSERT INTO artwork_assets (
						id, entity_type, entity_id, artwork_kind, origin_type, local_file_id,
						mime_type, width, height, byte_size, is_primary, is_protected_user_asset
					) VALUES (?, 'work', ?, 'poster', 'user_local', ?, ?, 600, 900, ?, 1, 1)
				`, domain.NewImmutableID(), workID, c.id, mime, c.size)
			}
			_, _ = s.sqlDB.ExecContext(ctx, `UPDATE works SET poster_rel_path = ? WHERE id = ? AND poster_rel_path = ''`, c.id, workID)
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
	if ym := yearTokenRegex.FindStringSubmatchIndex(stem); len(ym) >= 4 {
		yr, _ := strconv.Atoi(stem[ym[2]:ym[3]])
		prefix := strings.TrimSpace(stem[:ym[0]])
		if prefix != "" {
			return yr, prefix
		}
	}
	lowerParent := strings.ToLower(filepath.Base(parentFolder))
	if parentFolder != "" && lowerParent != rootCategory && lowerParent != "movies" && lowerParent != "افلام" && lowerParent != "أفلام" {
		if ym := yearTokenRegex.FindStringSubmatchIndex(parentFolder); len(ym) >= 4 {
			yr, _ := strconv.Atoi(parentFolder[ym[2]:ym[3]])
			prefix := strings.TrimSpace(parentFolder[:ym[0]])
			if prefix != "" {
				return yr, prefix
			}
		}
	}
	return 0, stem
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
