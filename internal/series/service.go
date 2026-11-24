package series

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/sadigking5-cmd/Estiraha-Next/internal/library"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/naming"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/search"
)

type SeriesFilter struct {
	Query       string `json:"query"`
	MediaType   string `json:"mediaType"`
	MissingOnly bool   `json:"missingOnly"`
	ArabicOnly  bool   `json:"arabicOnly"`
	Limit       int    `json:"limit"`
}

type SeriesCard struct {
	ID                   string  `json:"id"`
	MediaType            string  `json:"mediaType"`
	TitleOriginal        string  `json:"titleOriginal"`
	TitleAr              string  `json:"titleAr"`
	TitleEn              string  `json:"titleEn"`
	IsArabicOrigin       bool    `json:"isArabicOrigin"`
	CanonicalFolderName  string  `json:"canonicalFolderName"`
	IdentificationStatus string  `json:"identificationStatus"`
	ConfidenceScore      float64 `json:"confidenceScore"`
	SeasonCount          int     `json:"seasonCount"`
	PresentEpisodeCount  int     `json:"presentEpisodeCount"`
	MissingEpisodeCount  int     `json:"missingEpisodeCount"`
	MissingSeasonNumbers []int   `json:"missingSeasonNumbers"`
	PosterFileID         string  `json:"posterFileId"`
	WatchStatus          string  `json:"watchStatus"`
}

type EpisodeVersionItem struct {
	FileID          string  `json:"fileId"`
	DiskLabel       string  `json:"diskLabel"`
	FullPath        string  `json:"fullPath"`
	Filename        string  `json:"filename"`
	ResolutionLabel string  `json:"resolutionLabel"`
	VideoCodec      string  `json:"videoCodec"`
	SizeBytes       int64   `json:"sizeBytes"`
	IntegrityStatus string  `json:"integrityStatus"`
	QualityScore    float64 `json:"qualityScore"`
	IsBestVersion   bool    `json:"isBestVersion"`
}

type EpisodeView struct {
	ID                string               `json:"id"`
	SeasonNumber      int                  `json:"seasonNumber"`
	EpisodeNumber     int                  `json:"episodeNumber"`
	CanonicalFileStem string               `json:"canonicalFileStem"`
	TitleAr           string               `json:"titleAr"`
	TitleEn           string               `json:"titleEn"`
	IsMissing         bool                 `json:"isMissing"`
	WatchStatus       string               `json:"watchStatus"`
	ResumePositionMs  int64                `json:"resumePositionMs"`
	BestFileID        string               `json:"bestFileId"`
	BestResolution    string               `json:"bestResolution"`
	SubtitleCount     int                  `json:"subtitleCount"`
	HasShorts         bool                 `json:"hasShorts"`
	Versions          []EpisodeVersionItem `json:"versions"`
}

type SeasonView struct {
	ID                    string        `json:"id"`
	SeasonNumber          int           `json:"seasonNumber"`
	CanonicalFolderCode   string        `json:"canonicalFolderCode"`
	ExpectedEpisodeCount  int           `json:"expectedEpisodeCount"`
	PresentEpisodeCount   int           `json:"presentEpisodeCount"`
	MissingEpisodeNumbers []int         `json:"missingEpisodeNumbers"`
	Episodes              []EpisodeView `json:"episodes"`
}

type SeriesDetail struct {
	SeriesCard
	SynopsisAr string       `json:"synopsisAr"`
	SynopsisEn string       `json:"synopsisEn"`
	Seasons    []SeasonView `json:"seasons"`
}

type UpdateSeriesMetadataInput struct {
	WorkID         string `json:"workId"`
	TitleOriginal  string `json:"titleOriginal"`
	TitleAr        string `json:"titleAr"`
	TitleEn        string `json:"titleEn"`
	IsArabicOrigin bool   `json:"isArabicOrigin"`
	SynopsisAr     string `json:"synopsisAr"`
	SynopsisEn     string `json:"synopsisEn"`
}

type Service struct {
	sqlDB *sql.DB
}

func NewService(sqlDB *sql.DB) *Service {
	return &Service{sqlDB: sqlDB}
}

func (s *Service) ListSeries(ctx context.Context, f SeriesFilter) ([]SeriesCard, error) {
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	args := []any{}
	where := []string{"w.sub_format = 'episodic'"}

	if q := strings.TrimSpace(f.Query); q != "" {
		norm := search.NormalizeQuery(q)
		likeRaw := "%" + q + "%"
		likeNorm := "%" + norm + "%"
		ftsToken := `"` + strings.ReplaceAll(norm, `"`, "") + `"*`
		where = append(where, `(
			w.id IN (SELECT work_id FROM works_fts WHERE works_fts MATCH ?)
			OR w.title_original LIKE ? OR w.title_ar LIKE ? OR w.title_en LIKE ? OR w.canonical_folder_name LIKE ?
		)`)
		args = append(args, ftsToken, likeRaw, likeRaw, likeNorm, likeRaw)
	}
	if f.MediaType != "" && f.MediaType != "all" {
		where = append(where, "w.media_type = ?")
		args = append(args, f.MediaType)
	}
	if f.ArabicOnly {
		where = append(where, "w.is_arabic_origin = 1")
	}
	args = append(args, limit)

	query := fmt.Sprintf(`
		SELECT
			w.id, w.media_type, w.title_original, w.title_ar, w.title_en, w.is_arabic_origin,
			w.canonical_folder_name, w.identification_status, w.confidence_score, w.poster_rel_path,
			COALESCE(ws.watch_status, 'unwatched'),
			(SELECT COUNT(*) FROM seasons sn WHERE sn.work_id = w.id),
			(SELECT COUNT(*) FROM episodes ep WHERE ep.work_id = w.id AND ep.is_missing = 0)
		FROM works w
		LEFT JOIN watch_states ws ON ws.work_id = w.id AND ws.episode_id IS NULL
		WHERE %s
		ORDER BY w.updated_at DESC
		LIMIT ?
	`, strings.Join(where, " AND "))

	rows, err := s.sqlDB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	var cards []SeriesCard
	for rows.Next() {
		var c SeriesCard
		var arInt int
		if err := rows.Scan(
			&c.ID, &c.MediaType, &c.TitleOriginal, &c.TitleAr, &c.TitleEn, &arInt,
			&c.CanonicalFolderName, &c.IdentificationStatus, &c.ConfidenceScore, &c.PosterFileID,
			&c.WatchStatus, &c.SeasonCount, &c.PresentEpisodeCount,
		); err != nil {
			rows.Close()
			return nil, err
		}
		c.IsArabicOrigin = arInt == 1
		cards = append(cards, c)
	}
	rows.Close()

	filtered := make([]SeriesCard, 0, len(cards))
	for _, c := range cards {
		missingSeasons, missingEps := s.computeSeriesGaps(ctx, c.ID)
		c.MissingSeasonNumbers = missingSeasons
		c.MissingEpisodeCount = missingEps
		if f.MissingOnly && c.MissingEpisodeCount == 0 && len(c.MissingSeasonNumbers) == 0 {
			continue
		}
		filtered = append(filtered, c)
	}
	return filtered, nil
}

func (s *Service) GetSeriesDetail(ctx context.Context, workID string) (SeriesDetail, error) {
	var d SeriesDetail
	var arInt int
	err := s.sqlDB.QueryRowContext(ctx, `
		SELECT w.id, w.media_type, w.title_original, w.title_ar, w.title_en, w.is_arabic_origin,
		       w.canonical_folder_name, w.identification_status, w.confidence_score,
		       w.synopsis_ar, w.synopsis_en, w.poster_rel_path,
		       COALESCE(ws.watch_status, 'unwatched')
		FROM works w
		LEFT JOIN watch_states ws ON ws.work_id = w.id AND ws.episode_id IS NULL
		WHERE w.id = ?
	`, workID).Scan(
		&d.ID, &d.MediaType, &d.TitleOriginal, &d.TitleAr, &d.TitleEn, &arInt,
		&d.CanonicalFolderName, &d.IdentificationStatus, &d.ConfidenceScore,
		&d.SynopsisAr, &d.SynopsisEn, &d.PosterFileID, &d.WatchStatus,
	)
	if err != nil {
		return SeriesDetail{}, err
	}
	d.IsArabicOrigin = arInt == 1

	sRows, err := s.sqlDB.QueryContext(ctx, `
		SELECT id, season_number, canonical_folder_code, COALESCE(expected_episode_count, 0)
		FROM seasons WHERE work_id = ? ORDER BY season_number ASC
	`, workID)
	if err != nil {
		return d, err
	}
	var seasons []SeasonView
	var seasonNums []int
	for sRows.Next() {
		var sv SeasonView
		if err := sRows.Scan(&sv.ID, &sv.SeasonNumber, &sv.CanonicalFolderCode, &sv.ExpectedEpisodeCount); err == nil {
			seasons = append(seasons, sv)
			if sv.SeasonNumber > 0 {
				seasonNums = append(seasonNums, sv.SeasonNumber)
			}
		}
	}
	sRows.Close()

	d.MissingSeasonNumbers = detectNumericGaps(seasonNums, 0)
	d.SeasonCount = len(seasons)

	for si := range seasons {
		sv := &seasons[si]
		eRows, err := s.sqlDB.QueryContext(ctx, `
			SELECT ep.id, ep.episode_number, ep.canonical_file_stem, ep.title_ar, ep.title_en, ep.is_missing,
			       COALESCE(ws.watch_status, 'unwatched'), COALESCE(ws.resume_position_ms, 0),
			       (SELECT COUNT(*) FROM media_files mf WHERE mf.episode_id = ep.id AND mf.file_kind = 'subtitle' AND mf.presence_status = 'present'),
			       (SELECT COUNT(*) FROM media_files mf WHERE mf.episode_id = ep.id AND mf.file_kind = 'shorts' AND mf.presence_status = 'present')
			FROM episodes ep
			LEFT JOIN watch_states ws ON ws.episode_id = ep.id
			WHERE ep.season_id = ?
			ORDER BY ep.episode_number ASC
		`, sv.ID)
		if err != nil {
			continue
		}
		presentEpNums := []int{}
		epMap := map[int]EpisodeView{}
		for eRows.Next() {
			var ev EpisodeView
			var missInt, shortsCnt int
			if err := eRows.Scan(
				&ev.ID, &ev.EpisodeNumber, &ev.CanonicalFileStem, &ev.TitleAr, &ev.TitleEn, &missInt,
				&ev.WatchStatus, &ev.ResumePositionMs, &ev.SubtitleCount, &shortsCnt,
			); err == nil {
				ev.SeasonNumber = sv.SeasonNumber
				ev.IsMissing = missInt == 1
				ev.HasShorts = shortsCnt > 0
				ev.Versions = s.loadEpisodeVersions(ctx, ev.ID)
				if len(ev.Versions) == 0 {
					ev.IsMissing = true
				} else {
					ev.BestFileID = ev.Versions[0].FileID
					ev.BestResolution = ev.Versions[0].ResolutionLabel
					presentEpNums = append(presentEpNums, ev.EpisodeNumber)
					sv.PresentEpisodeCount++
					d.PresentEpisodeCount++
				}
				epMap[ev.EpisodeNumber] = ev
			}
		}
		eRows.Close()

		sv.MissingEpisodeNumbers = detectNumericGaps(presentEpNums, sv.ExpectedEpisodeCount)
		d.MissingEpisodeCount += len(sv.MissingEpisodeNumbers)

		maxEp := sv.ExpectedEpisodeCount
		for _, n := range presentEpNums {
			if n > maxEp {
				maxEp = n
			}
		}
		sv.Episodes = []EpisodeView{}
		for n := 1; n <= maxEp; n++ {
			if existing, ok := epMap[n]; ok {
				sv.Episodes = append(sv.Episodes, existing)
			} else {
				sv.Episodes = append(sv.Episodes, EpisodeView{
					ID:                fmt.Sprintf("missing-s%02d-e%02d", sv.SeasonNumber, n),
					SeasonNumber:      sv.SeasonNumber,
					EpisodeNumber:     n,
					CanonicalFileStem: fmt.Sprintf("%02d", n),
					TitleAr:           fmt.Sprintf("الحلقة %d (مفقودة)", n),
					TitleEn:           fmt.Sprintf("Episode %d (Missing)", n),
					IsMissing:         true,
					WatchStatus:       "unwatched",
					Versions:          []EpisodeVersionItem{},
				})
			}
		}
	}

	d.Seasons = seasons
	return d, nil
}

func (s *Service) SetSeasonExpectedEpisodes(ctx context.Context, seasonID string, expectedCount int) error {
	if expectedCount < 0 {
		expectedCount = 0
	}
	_, err := s.sqlDB.ExecContext(ctx, `
		UPDATE seasons SET expected_episode_count = ?, updated_at = ? WHERE id = ?
	`, expectedCount, time.Now().UTC().Format(time.RFC3339Nano), seasonID)
	return err
}

func (s *Service) UpdateSeriesMetadata(ctx context.Context, in UpdateSeriesMetadataInput) (SeriesDetail, error) {
	primary := strings.TrimSpace(in.TitleOriginal)
	if primary == "" {
		if in.IsArabicOrigin && strings.TrimSpace(in.TitleAr) != "" {
			primary = strings.TrimSpace(in.TitleAr)
		} else {
			primary = strings.TrimSpace(in.TitleEn)
		}
	}
	canonicalFolder, err := naming.FormatSeriesFolder(primary)
	if err != nil {
		return SeriesDetail{}, err
	}
	arInt := 0
	if in.IsArabicOrigin {
		arInt = 1
	}
	nowStr := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.sqlDB.ExecContext(ctx, `
		UPDATE works
		SET title_original = ?, title_ar = ?, title_en = ?, is_arabic_origin = ?,
		    canonical_folder_name = ?, synopsis_ar = ?, synopsis_en = ?,
		    identification_status = 'locked_by_user', confidence_score = 1.0, updated_at = ?
		WHERE id = ?
	`, primary, strings.TrimSpace(in.TitleAr), strings.TrimSpace(in.TitleEn), arInt,
		canonicalFolder, strings.TrimSpace(in.SynopsisAr), strings.TrimSpace(in.SynopsisEn), nowStr, in.WorkID)
	if err != nil {
		return SeriesDetail{}, err
	}
	return s.GetSeriesDetail(ctx, in.WorkID)
}

func (s *Service) loadEpisodeVersions(ctx context.Context, episodeID string) []EpisodeVersionItem {
	rows, err := s.sqlDB.QueryContext(ctx, `
		SELECT mf.id, d.label, d.mount_path, mf.relative_path, mf.filename, mf.size_bytes, mf.integrity_status,
		       COALESCE(mv.resolution_label, '1080p'), COALESCE(mv.video_codec, 'H.264'), COALESCE(mv.computed_quality_score, 0)
		FROM media_files mf
		JOIN disks d ON d.id = mf.disk_id
		LEFT JOIN media_versions mv ON mv.file_id = mf.id
		WHERE mf.episode_id = ? AND mf.file_kind = 'video' AND mf.presence_status = 'present'
		ORDER BY COALESCE(mv.computed_quality_score, 0) DESC, mf.size_bytes DESC
	`, episodeID)
	if err != nil {
		return []EpisodeVersionItem{}
	}
	defer rows.Close()

	out := []EpisodeVersionItem{}
	idx := 0
	for rows.Next() {
		var v EpisodeVersionItem
		var mountPath, relPath string
		if err := rows.Scan(
			&v.FileID, &v.DiskLabel, &mountPath, &relPath, &v.Filename, &v.SizeBytes, &v.IntegrityStatus,
			&v.ResolutionLabel, &v.VideoCodec, &v.QualityScore,
		); err == nil {
			v.FullPath = library.ResolveFullPath(mountPath, relPath)
			if idx == 0 && v.IntegrityStatus == "healthy" {
				v.IsBestVersion = true
			}
			out = append(out, v)
			idx++
		}
	}
	return out
}

func (s *Service) computeSeriesGaps(ctx context.Context, workID string) ([]int, int) {
	rows, err := s.sqlDB.QueryContext(ctx, `
		SELECT id, season_number, COALESCE(expected_episode_count, 0) FROM seasons WHERE work_id = ? ORDER BY season_number ASC
	`, workID)
	if err != nil {
		return []int{}, 0
	}
	type sRow struct {
		id       string
		num, exp int
	}
	var sList []sRow
	var sNums []int
	for rows.Next() {
		var r sRow
		if err := rows.Scan(&r.id, &r.num, &r.exp); err == nil {
			sList = append(sList, r)
			if r.num > 0 {
				sNums = append(sNums, r.num)
			}
		}
	}
	rows.Close()

	missingSeasons := detectNumericGaps(sNums, 0)
	missingEps := 0
	for _, sr := range sList {
		eRows, err := s.sqlDB.QueryContext(ctx, `
			SELECT DISTINCT ep.episode_number
			FROM episodes ep
			JOIN media_files mf ON mf.episode_id = ep.id AND mf.file_kind = 'video' AND mf.presence_status = 'present'
			WHERE ep.season_id = ?
		`, sr.id)
		if err != nil {
			continue
		}
		var epNums []int
		for eRows.Next() {
			var n int
			if err := eRows.Scan(&n); err == nil && n > 0 {
				epNums = append(epNums, n)
			}
		}
		eRows.Close()
		missingEps += len(detectNumericGaps(epNums, sr.exp))
	}
	return missingSeasons, missingEps
}

func detectNumericGaps(present []int, expectedMax int) []int {
	if len(present) == 0 && expectedMax <= 0 {
		return []int{}
	}
	set := map[int]bool{}
	maxVal := expectedMax
	for _, n := range present {
		if n > 0 {
			set[n] = true
			if n > maxVal {
				maxVal = n
			}
		}
	}
	var gaps []int
	for i := 1; i <= maxVal; i++ {
		if !set[i] {
			gaps = append(gaps, i)
		}
	}
	sort.Ints(gaps)
	return gaps
}
