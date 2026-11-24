package movies

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/sadigking5-cmd/Estiraha-Next/internal/domain"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/library"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/naming"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/search"
)

type MovieFilter struct {
	Query                string `json:"query"`
	IdentificationStatus string `json:"identificationStatus"`
	WatchStatus          string `json:"watchStatus"`
	ArabicOnly           bool   `json:"arabicOnly"`
	ForeignOnly          bool   `json:"foreignOnly"`
	DiskID               string `json:"diskId"`
	Limit                int    `json:"limit"`
}

type MovieCard struct {
	ID                   string  `json:"id"`
	MediaType            string  `json:"mediaType"`
	TitleOriginal        string  `json:"titleOriginal"`
	TitleAr              string  `json:"titleAr"`
	TitleEn              string  `json:"titleEn"`
	IsArabicOrigin       bool    `json:"isArabicOrigin"`
	ProductionYear       int     `json:"productionYear"`
	CanonicalFolderName  string  `json:"canonicalFolderName"`
	IdentificationStatus string  `json:"identificationStatus"`
	ConfidenceScore      float64 `json:"confidenceScore"`
	WatchStatus          string  `json:"watchStatus"`
	ResumePositionMs     int64   `json:"resumePositionMs"`
	DurationMs           int64   `json:"durationMs"`
	RatingOutOf10        float64 `json:"ratingOutOf10"`
	IsFavorite           bool    `json:"isFavorite"`
	BestResolution       string  `json:"bestResolution"`
	VersionCount         int     `json:"versionCount"`
	SubtitleCount        int     `json:"subtitleCount"`
	ShortsCount          int     `json:"shortsCount"`
	PosterFileID         string  `json:"posterFileId"`
	DiskLabels           string  `json:"diskLabels"`
}

type AssociatedFileView struct {
	FileID          string  `json:"fileId"`
	DiskID          string  `json:"diskId"`
	DiskLabel       string  `json:"diskLabel"`
	DiskOnline      bool    `json:"diskOnline"`
	RelativePath    string  `json:"relativePath"`
	FullPath        string  `json:"fullPath"`
	Filename        string  `json:"filename"`
	Extension       string  `json:"extension"`
	FileKind        string  `json:"fileKind"`
	SizeBytes       int64   `json:"sizeBytes"`
	IntegrityStatus string  `json:"integrityStatus"`
	PresenceStatus  string  `json:"presenceStatus"`
	ResolutionLabel string  `json:"resolutionLabel"`
	VideoCodec      string  `json:"videoCodec"`
	QualityScore    float64 `json:"qualityScore"`
	ProtectedShorts bool    `json:"protectedShorts"`
}

type MovieDetail struct {
	MovieCard
	SynopsisAr   string               `json:"synopsisAr"`
	SynopsisEn   string               `json:"synopsisEn"`
	PersonalNote string               `json:"personalNote"`
	Files        []AssociatedFileView `json:"files"`
}

type UpdateMovieMetadataInput struct {
	WorkID         string  `json:"workId"`
	TitleOriginal  string  `json:"titleOriginal"`
	TitleAr        string  `json:"titleAr"`
	TitleEn        string  `json:"titleEn"`
	ProductionYear int     `json:"productionYear"`
	IsArabicOrigin bool    `json:"isArabicOrigin"`
	SynopsisAr     string  `json:"synopsisAr"`
	SynopsisEn     string  `json:"synopsisEn"`
	RatingOutOf10  float64 `json:"ratingOutOf10"`
	PersonalNote   string  `json:"personalNote"`
	IsFavorite     bool    `json:"isFavorite"`
}

type Service struct {
	sqlDB *sql.DB
}

func NewService(sqlDB *sql.DB) *Service {
	return &Service{sqlDB: sqlDB}
}

func (s *Service) ListMovies(ctx context.Context, f MovieFilter) ([]MovieCard, error) {
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}

	args := []any{}
	where := []string{"w.sub_format = 'standalone'", "w.media_type IN ('movie', 'documentary_movie', 'franchise_entry', 'tv_program')"}

	if q := strings.TrimSpace(f.Query); q != "" {
		norm := search.NormalizeQuery(q)
		likeRaw := "%" + q + "%"
		likeNorm := "%" + norm + "%"
		where = append(where, `(
			w.id IN (SELECT work_id FROM works_fts WHERE works_fts MATCH ?)
			OR w.title_original LIKE ? OR w.title_ar LIKE ? OR w.title_en LIKE ? OR w.canonical_folder_name LIKE ?
		)`)
		ftsToken := `"` + strings.ReplaceAll(norm, `"`, "") + `"*`
		args = append(args, ftsToken, likeRaw, likeRaw, likeNorm, likeRaw)
	}

	if f.IdentificationStatus != "" && f.IdentificationStatus != "all" {
		where = append(where, "w.identification_status = ?")
		args = append(args, f.IdentificationStatus)
	}
	if f.WatchStatus != "" && f.WatchStatus != "all" {
		where = append(where, "COALESCE(ws.watch_status, 'unwatched') = ?")
		args = append(args, f.WatchStatus)
	}
	if f.ArabicOnly {
		where = append(where, "w.is_arabic_origin = 1")
	}
	if f.ForeignOnly {
		where = append(where, "w.is_arabic_origin = 0")
	}
	if f.DiskID != "" && f.DiskID != "all" {
		where = append(where, "EXISTS (SELECT 1 FROM media_files mf WHERE mf.work_id = w.id AND mf.disk_id = ?)")
		args = append(args, f.DiskID)
	}

	args = append(args, limit)

	query := fmt.Sprintf(`
		SELECT
			w.id, w.media_type, w.title_original, w.title_ar, w.title_en, w.is_arabic_origin,
			COALESCE(w.production_year, 0), w.canonical_folder_name, w.identification_status,
			w.confidence_score,
			COALESCE(ws.watch_status, 'unwatched'), COALESCE(ws.resume_position_ms, 0), COALESCE(ws.duration_ms, 0),
			COALESCE(urn.rating_out_of_10, 0), COALESCE(urn.is_favorite, 0),
			COALESCE((SELECT mv.resolution_label FROM media_versions mv WHERE mv.work_id = w.id ORDER BY mv.computed_quality_score DESC LIMIT 1), ''),
			(SELECT COUNT(*) FROM media_files mf WHERE mf.work_id = w.id AND mf.file_kind = 'video' AND mf.presence_status = 'present'),
			(SELECT COUNT(*) FROM media_files mf WHERE mf.work_id = w.id AND mf.file_kind = 'subtitle' AND mf.presence_status = 'present'),
			(SELECT COUNT(*) FROM media_files mf WHERE mf.work_id = w.id AND mf.file_kind = 'shorts' AND mf.presence_status = 'present'),
			w.poster_rel_path,
			COALESCE((SELECT GROUP_CONCAT(DISTINCT d.label) FROM media_files mf JOIN disks d ON d.id = mf.disk_id WHERE mf.work_id = w.id), '')
		FROM works w
		LEFT JOIN watch_states ws ON ws.work_id = w.id AND ws.episode_id IS NULL
		LEFT JOIN user_ratings_notes urn ON urn.work_id = w.id
		WHERE %s
		ORDER BY w.updated_at DESC
		LIMIT ?
	`, strings.Join(where, " AND "))

	rows, err := s.sqlDB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []MovieCard{}
	for rows.Next() {
		var m MovieCard
		var arInt, favInt int
		if err := rows.Scan(
			&m.ID, &m.MediaType, &m.TitleOriginal, &m.TitleAr, &m.TitleEn, &arInt,
			&m.ProductionYear, &m.CanonicalFolderName, &m.IdentificationStatus,
			&m.ConfidenceScore,
			&m.WatchStatus, &m.ResumePositionMs, &m.DurationMs,
			&m.RatingOutOf10, &favInt,
			&m.BestResolution, &m.VersionCount, &m.SubtitleCount, &m.ShortsCount,
			&m.PosterFileID, &m.DiskLabels,
		); err != nil {
			return nil, err
		}
		m.IsArabicOrigin = arInt == 1
		m.IsFavorite = favInt == 1
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Service) GetMovieDetail(ctx context.Context, workID string) (MovieDetail, error) {
	var d MovieDetail
	var arInt, favInt int
	err := s.sqlDB.QueryRowContext(ctx, `
		SELECT
			w.id, w.media_type, w.title_original, w.title_ar, w.title_en, w.is_arabic_origin,
			COALESCE(w.production_year, 0), w.canonical_folder_name, w.identification_status,
			w.confidence_score, w.synopsis_ar, w.synopsis_en, w.poster_rel_path,
			COALESCE(ws.watch_status, 'unwatched'), COALESCE(ws.resume_position_ms, 0), COALESCE(ws.duration_ms, 0),
			COALESCE(urn.rating_out_of_10, 0), COALESCE(urn.personal_notes, ''), COALESCE(urn.is_favorite, 0)
		FROM works w
		LEFT JOIN watch_states ws ON ws.work_id = w.id AND ws.episode_id IS NULL
		LEFT JOIN user_ratings_notes urn ON urn.work_id = w.id
		WHERE w.id = ?
	`, workID).Scan(
		&d.ID, &d.MediaType, &d.TitleOriginal, &d.TitleAr, &d.TitleEn, &arInt,
		&d.ProductionYear, &d.CanonicalFolderName, &d.IdentificationStatus,
		&d.ConfidenceScore, &d.SynopsisAr, &d.SynopsisEn, &d.PosterFileID,
		&d.WatchStatus, &d.ResumePositionMs, &d.DurationMs,
		&d.RatingOutOf10, &d.PersonalNote, &favInt,
	)
	if err != nil {
		return MovieDetail{}, err
	}
	d.IsArabicOrigin = arInt == 1
	d.IsFavorite = favInt == 1

	fRows, err := s.sqlDB.QueryContext(ctx, `
		SELECT mf.id, mf.disk_id, d.label, d.mount_path, d.connection_state,
		       mf.relative_path, mf.filename, mf.extension, mf.file_kind, mf.size_bytes,
		       mf.integrity_status, mf.presence_status,
		       COALESCE(mv.resolution_label, ''), COALESCE(mv.video_codec, ''), COALESCE(mv.computed_quality_score, 0)
		FROM media_files mf
		JOIN disks d ON d.id = mf.disk_id
		LEFT JOIN media_versions mv ON mv.file_id = mf.id
		WHERE mf.work_id = ?
		ORDER BY CASE mf.file_kind WHEN 'video' THEN 1 WHEN 'subtitle' THEN 2 WHEN 'shorts' THEN 3 ELSE 4 END,
		         COALESCE(mv.computed_quality_score, 0) DESC
	`, workID)
	if err != nil {
		return d, err
	}
	defer fRows.Close()

	d.Files = []AssociatedFileView{}
	for fRows.Next() {
		var fv AssociatedFileView
		var mountPath, connState string
		if err := fRows.Scan(
			&fv.FileID, &fv.DiskID, &fv.DiskLabel, &mountPath, &connState,
			&fv.RelativePath, &fv.Filename, &fv.Extension, &fv.FileKind, &fv.SizeBytes,
			&fv.IntegrityStatus, &fv.PresenceStatus,
			&fv.ResolutionLabel, &fv.VideoCodec, &fv.QualityScore,
		); err != nil {
			return d, err
		}
		fv.DiskOnline = connState == "online"
		fv.FullPath = library.ResolveFullPath(mountPath, fv.RelativePath)
		fv.ProtectedShorts = fv.FileKind == "shorts" || naming.IsShortsExtension(fv.Filename)
		if fv.FileKind == "video" && fv.PresenceStatus == "present" {
			d.VersionCount++
			if d.BestResolution == "" {
				d.BestResolution = fv.ResolutionLabel
			}
		} else if fv.FileKind == "subtitle" {
			d.SubtitleCount++
		} else if fv.FileKind == "shorts" {
			d.ShortsCount++
		}
		d.Files = append(d.Files, fv)
	}
	return d, nil
}

func (s *Service) UpdateMovieMetadata(ctx context.Context, in UpdateMovieMetadataInput) (MovieDetail, error) {
	primaryTitle := strings.TrimSpace(in.TitleOriginal)
	if primaryTitle == "" {
		if in.IsArabicOrigin && strings.TrimSpace(in.TitleAr) != "" {
			primaryTitle = strings.TrimSpace(in.TitleAr)
		} else {
			primaryTitle = strings.TrimSpace(in.TitleEn)
		}
	}
	canonicalFolder, err := naming.FormatMovieName(primaryTitle, in.ProductionYear)
	if err != nil {
		return MovieDetail{}, err
	}

	arInt := 0
	if in.IsArabicOrigin {
		arInt = 1
	}
	favInt := 0
	if in.IsFavorite {
		favInt = 1
	}
	nowStr := time.Now().UTC().Format(time.RFC3339Nano)

	_, err = s.sqlDB.ExecContext(ctx, `
		UPDATE works
		SET title_original = ?, title_ar = ?, title_en = ?, is_arabic_origin = ?,
		    production_year = ?, canonical_folder_name = ?,
		    synopsis_ar = ?, synopsis_en = ?,
		    identification_status = 'locked_by_user', confidence_score = 1.0,
		    updated_at = ?
		WHERE id = ?
	`, primaryTitle, strings.TrimSpace(in.TitleAr), strings.TrimSpace(in.TitleEn), arInt,
		in.ProductionYear, canonicalFolder,
		strings.TrimSpace(in.SynopsisAr), strings.TrimSpace(in.SynopsisEn),
		nowStr, in.WorkID)
	if err != nil {
		return MovieDetail{}, err
	}

	_, _ = s.sqlDB.ExecContext(ctx, `
		INSERT INTO user_ratings_notes (work_id, rating_out_of_10, personal_notes, is_favorite, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(work_id) DO UPDATE SET
			rating_out_of_10 = excluded.rating_out_of_10,
			personal_notes = excluded.personal_notes,
			is_favorite = excluded.is_favorite,
			updated_at = excluded.updated_at
	`, in.WorkID, in.RatingOutOf10, strings.TrimSpace(in.PersonalNote), favInt, nowStr)

	_, _ = s.sqlDB.ExecContext(ctx, `
		INSERT INTO audit_logs (id, event_category, severity, actor, summary, details_json)
		VALUES (?, 'review', 'info', 'user', ?, ?)
	`, domain.NewImmutableID(), fmt.Sprintf("User updated movie metadata: %s", canonicalFolder), fmt.Sprintf(`{"workId":%q}`, in.WorkID))

	return s.GetMovieDetail(ctx, in.WorkID)
}
