package reports

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/sadigking5-cmd/Estiraha-Next/internal/domain"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/series"
)

type HealthIssueView struct {
	ID             string `json:"id"`
	IssueKind      string `json:"issueKind"`
	Severity       string `json:"severity"`
	WorkID         string `json:"workId"`
	FileID         string `json:"fileId"`
	DiskID         string `json:"diskId"`
	TitleAr        string `json:"titleAr"`
	TitleEn        string `json:"titleEn"`
	DetailsAr      string `json:"detailsAr"`
	DetailsEn      string `json:"detailsEn"`
	SuggestedFixAr string `json:"suggestedFixAr"`
	SuggestedFixEn string `json:"suggestedFixEn"`
	DetectedAt     string `json:"detectedAt"`
}

type DashboardStats struct {
	TotalDisks           int            `json:"totalDisks"`
	OnlineDisks          int            `json:"onlineDisks"`
	TotalRoots           int            `json:"totalRoots"`
	TotalMediaFiles      int            `json:"totalMediaFiles"`
	TotalSizeBytes       int64          `json:"totalSizeBytes"`
	TotalMovies          int            `json:"totalMovies"`
	TotalSeries          int            `json:"totalSeries"`
	TotalEpisodesPresent int            `json:"totalEpisodesPresent"`
	ProtectedShortsCount int            `json:"protectedShortsCount"`
	NeedsReviewCount     int            `json:"needsReviewCount"`
	PendingReviewBatches int            `json:"pendingReviewBatches"`
	VerifiedBackupsCount int            `json:"verifiedBackupsCount"`
	WatchingNowCount     int            `json:"watchingNowCount"`
	IssuesByKind         map[string]int `json:"issuesByKind"`
}

type Service struct {
	sqlDB     *sql.DB
	seriesSvc *series.Service
}

func NewService(sqlDB *sql.DB, seriesSvc *series.Service) *Service {
	return &Service{
		sqlDB:     sqlDB,
		seriesSvc: seriesSvc,
	}
}

func (s *Service) GenerateHealthReport(ctx context.Context) ([]HealthIssueView, error) {
	_, _ = s.sqlDB.ExecContext(ctx, `DELETE FROM library_issues WHERE resolved = 0`)
	nowStr := time.Now().UTC().Format(time.RFC3339Nano)

	seriesList, err := s.seriesSvc.ListSeries(ctx, series.SeriesFilter{Limit: 500})
	if err == nil {
		for _, sc := range seriesList {
			detail, dErr := s.seriesSvc.GetSeriesDetail(ctx, sc.ID)
			if dErr != nil {
				continue
			}
			for _, ms := range detail.MissingSeasonNumbers {
				s.insertIssue(ctx, "missing_season", "warning", sc.ID, "", "",
					fmt.Sprintf("موسم مفقود S%02d — %s", ms, sc.CanonicalFolderName),
					fmt.Sprintf("Missing Season S%02d — %s", ms, sc.CanonicalFolderName),
					fmt.Sprintf("المسلسل يحتوي مواسم لاحقة لكن الموسم S%02d غير موجود على الأقراص المتصلة.", ms),
					fmt.Sprintf("Later seasons exist, but Season S%02d is absent.", ms),
					"تحقق من الأقراص المنفصلة أو أضف مجلد الموسم المفقود ثم شغّل الفحص اليدوي.",
					"Check offline drives or add the missing season folder and run Manual Scan.",
					nowStr,
				)
			}
			for _, sn := range detail.Seasons {
				for _, me := range sn.MissingEpisodeNumbers {
					s.insertIssue(ctx, "missing_episode", "warning", sc.ID, "", "",
						fmt.Sprintf("حلقة مفقودة %s/E%02d — %s", sn.CanonicalFolderCode, me, sc.CanonicalFolderName),
						fmt.Sprintf("Missing Episode %s/E%02d — %s", sn.CanonicalFolderCode, me, sc.CanonicalFolderName),
						fmt.Sprintf("الحلقة رقم %02d مفقودة داخل %s.", me, sn.CanonicalFolderCode),
						fmt.Sprintf("Episode %02d is missing inside %s.", me, sn.CanonicalFolderCode),
						"تحقق من ملفات الموسم أو حدّث العدد المتوقع للحلقات.",
						"Check season folder files or adjust expected episode count.",
						nowStr,
					)
				}
			}
		}
	}

	subRows, err := s.sqlDB.QueryContext(ctx, `
		SELECT w.id, w.canonical_folder_name
		FROM works w
		WHERE w.sub_format = 'standalone' AND w.is_arabic_origin = 0
		  AND NOT EXISTS (SELECT 1 FROM media_files mf WHERE mf.work_id = w.id AND mf.file_kind = 'subtitle' AND mf.presence_status = 'present')
	`)
	if err == nil {
		type wPair struct{ id, name string }
		var list []wPair
		for subRows.Next() {
			var p wPair
			if err := subRows.Scan(&p.id, &p.name); err == nil {
				list = append(list, p)
			}
		}
		subRows.Close()
		for _, p := range list {
			s.insertIssue(ctx, "missing_subtitle", "info", p.id, "", "",
				fmt.Sprintf("ملف ترجمة مفقود — %s", p.name),
				fmt.Sprintf("Missing Subtitle — %s", p.name),
				"لم يتم العثور على ملف ترجمة خارجي (.srt/.ass/.vtt) مرافق للعمل الأجنبي.",
				"No external subtitle (.srt/.ass/.vtt) found alongside this foreign work.",
				"أضف ملف الترجمة بجوار الفيديو ثم أعد الفحص اليدوي.",
				"Place an .srt/.vtt subtitle next to the video and run Manual Scan.",
				nowStr,
			)
		}
	}

	postRows, err := s.sqlDB.QueryContext(ctx, `
		SELECT w.id, w.canonical_folder_name
		FROM works w
		WHERE w.poster_rel_path = ''
		  AND NOT EXISTS (SELECT 1 FROM media_files mf WHERE mf.work_id = w.id AND mf.file_kind = 'image' AND mf.presence_status = 'present')
	`)
	if err == nil {
		type wPair struct{ id, name string }
		var list []wPair
		for postRows.Next() {
			var p wPair
			if err := postRows.Scan(&p.id, &p.name); err == nil {
				list = append(list, p)
			}
		}
		postRows.Close()
		for _, p := range list {
			s.insertIssue(ctx, "missing_poster", "info", p.id, "", "",
				fmt.Sprintf("بوستر محلي مفقود — %s", p.name),
				fmt.Sprintf("Missing Local Poster — %s", p.name),
				"لا توجد صورة بوستر محلية (.jpg/.png/.webp) مرتبطة بهذا العمل.",
				"No local poster image (.jpg/.png/.webp) is linked to this work.",
				"أضف بوستر محلي داخل مجلد العمل.",
				"Add a local poster image inside the work folder.",
				nowStr,
			)
		}
	}

	unRows, err := s.sqlDB.QueryContext(ctx, `
		SELECT w.id, w.title_original, w.identification_status
		FROM works w
		WHERE w.identification_status IN ('needs_review', 'unidentified', 'unknown_program')
		   OR (w.sub_format = 'standalone' AND w.media_type = 'movie' AND w.production_year IS NULL)
	`)
	if err == nil {
		type uRow struct{ id, title, st string }
		var list []uRow
		for unRows.Next() {
			var r uRow
			if err := unRows.Scan(&r.id, &r.title, &r.st); err == nil {
				list = append(list, r)
			}
		}
		unRows.Close()
		for _, r := range list {
			kind := "unidentified_file"
			if r.st == "identified" {
				kind = "missing_metadata"
			}
			s.insertIssue(ctx, kind, "warning", r.id, "", "",
				fmt.Sprintf("يحتاج مراجعة تعريف / بيانات ناقصة — %s", r.title),
				fmt.Sprintf("Unidentified / Incomplete Metadata — %s", r.title),
				"تم إيقاف إعادة التسمية التلقائية التزاماً بسياسة مراجعة الملفات غير المؤكدة.",
				"Auto-renaming was held back per 'Unidentified Files = Review' safety policy.",
				"افتح بطاقة العمل وعدّل الاسم وسنة الإنتاج يدوياً ثم أنشئ خطة تنظيم.",
				"Edit title & production year in the Movie/Series detail drawer, then generate an organization plan.",
				nowStr,
			)
		}
	}

	corrRows, err := s.sqlDB.QueryContext(ctx, `
		SELECT mf.id, COALESCE(mf.work_id, ''), mf.disk_id, mf.filename, mf.integrity_status
		FROM media_files mf
		WHERE mf.integrity_status IN ('corrupted', 'unreadable', 'truncated') AND mf.presence_status = 'present'
	`)
	if err == nil {
		type cRow struct{ fID, wID, dID, name, st string }
		var list []cRow
		for corrRows.Next() {
			var r cRow
			if err := corrRows.Scan(&r.fID, &r.wID, &r.dID, &r.name, &r.st); err == nil {
				list = append(list, r)
			}
		}
		corrRows.Close()
		for _, r := range list {
			kind := "corrupted_file"
			if r.st == "unreadable" {
				kind = "unreadable_file"
			}
			s.insertIssue(ctx, kind, "error", r.wID, r.fID, r.dID,
				fmt.Sprintf("ملف تالف أو فارغ (0 بايت) — %s", r.name),
				fmt.Sprintf("Corrupted / Unreadable File — %s", r.name),
				"الملف بحجم صفر بايت أو تعذرت قراءة ترويسته أثناء الفحص؛ تم استبعاده من اختيار أفضل نسخة.",
				"File is 0-byte or unreadable; excluded from Best-Version selection without deleting it.",
				"استبدل الملف بنسخة سليمة أو انقله يدوياً بعد المراجعة.",
				"Verify the file on disk or replace it with a healthy copy.",
				nowStr,
			)
		}
	}

	dupRows, err := s.sqlDB.QueryContext(ctx, `
		SELECT COALESCE(dg.work_id, ''), COALESCE(w.canonical_folder_name, 'Duplicate Group')
		FROM duplicate_groups dg
		LEFT JOIN works w ON w.id = dg.work_id
		WHERE dg.resolution_state = 'unresolved'
	`)
	if err == nil {
		type dRow struct{ wID, title string }
		var list []dRow
		for dupRows.Next() {
			var r dRow
			if err := dupRows.Scan(&r.wID, &r.title); err == nil {
				list = append(list, r)
			}
		}
		dupRows.Close()
		for _, r := range list {
			s.insertIssue(ctx, "duplicate_conflict", "warning", r.wID, "", "",
				fmt.Sprintf("نسخ مكررة على نفس القرص — %s", r.title),
				fmt.Sprintf("Same-Disk Duplicate Versions — %s", r.title),
				"توجد أكثر من نسخة لنفس العمل على نفس القرص؛ تم تحديد النسخة الأعلى جودة تلقائياً دون حذف أي ملف.",
				"Multiple versions exist on the same drive; best version was scored without deleting lower copies.",
				"راجع قسم التكرارات ومركز المراجعة لاتخاذ قرارك.",
				"Inspect Duplicates & Review Center to decide how to handle extra copies.",
				nowStr,
			)
		}
	}

	offRows, err := s.sqlDB.QueryContext(ctx, `SELECT id, label, mount_path FROM disks WHERE connection_state != 'online'`)
	if err == nil {
		type oRow struct{ id, label, mp string }
		var list []oRow
		for offRows.Next() {
			var r oRow
			if err := offRows.Scan(&r.id, &r.label, &r.mp); err == nil {
				list = append(list, r)
			}
		}
		offRows.Close()
		for _, r := range list {
			s.insertIssue(ctx, "offline_disk", "warning", "", "", r.id,
				fmt.Sprintf("قرص غير متصل حالياً — %s (%s)", r.label, r.mp),
				fmt.Sprintf("Offline Storage Drive — %s (%s)", r.label, r.mp),
				"تم الاحتفاظ بجميع سجلات الأعمال المرتبطة بهذا القرص بأمان دون حذف.",
				"All catalog entries on this drive are preserved safely until reconnected.",
				"وصّل القرص الخارجي ثم اضغط تحديث حالة الأقراص.",
				"Reconnect the drive and click Refresh Connectivity.",
				nowStr,
			)
		}
	}

	return s.ListHealthIssues(ctx, "")
}

func (s *Service) ListHealthIssues(ctx context.Context, kindFilter string) ([]HealthIssueView, error) {
	q := `
		SELECT id, issue_kind, severity, COALESCE(work_id, ''), COALESCE(file_id, ''), COALESCE(disk_id, ''),
		       title_ar, title_en, details_ar, details_en, suggested_fix_ar, suggested_fix_en, detected_at
		FROM library_issues
		WHERE resolved = 0
	`
	args := []any{}
	if kindFilter != "" && kindFilter != "all" {
		q += ` AND issue_kind = ?`
		args = append(args, kindFilter)
	}
	q += ` ORDER BY CASE severity WHEN 'critical' THEN 1 WHEN 'error' THEN 2 WHEN 'warning' THEN 3 ELSE 4 END, detected_at DESC LIMIT 300`

	rows, err := s.sqlDB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []HealthIssueView{}
	for rows.Next() {
		var it HealthIssueView
		if err := rows.Scan(
			&it.ID, &it.IssueKind, &it.Severity, &it.WorkID, &it.FileID, &it.DiskID,
			&it.TitleAr, &it.TitleEn, &it.DetailsAr, &it.DetailsEn, &it.SuggestedFixAr, &it.SuggestedFixEn, &it.DetectedAt,
		); err == nil {
			out = append(out, it)
		}
	}
	return out, rows.Err()
}

func (s *Service) GetDashboardStats(ctx context.Context) (DashboardStats, error) {
	st := DashboardStats{
		IssuesByKind: map[string]int{},
	}
	_ = s.sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM disks`).Scan(&st.TotalDisks)
	_ = s.sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM disks WHERE connection_state = 'online'`).Scan(&st.OnlineDisks)
	_ = s.sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM library_roots`).Scan(&st.TotalRoots)
	_ = s.sqlDB.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(size_bytes), 0) FROM media_files WHERE presence_status = 'present'`).Scan(&st.TotalMediaFiles, &st.TotalSizeBytes)
	_ = s.sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM works WHERE sub_format = 'standalone' AND media_type IN ('movie', 'documentary_movie')`).Scan(&st.TotalMovies)
	_ = s.sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM works WHERE sub_format = 'episodic'`).Scan(&st.TotalSeries)
	_ = s.sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM episodes WHERE is_missing = 0`).Scan(&st.TotalEpisodesPresent)
	_ = s.sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_files WHERE file_kind = 'shorts' AND presence_status = 'present'`).Scan(&st.ProtectedShortsCount)
	_ = s.sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM works WHERE identification_status IN ('needs_review', 'unidentified', 'unknown_program')`).Scan(&st.NeedsReviewCount)
	_ = s.sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM review_batches WHERE batch_status IN ('awaiting_approval', 'partially_approved', 'approved')`).Scan(&st.PendingReviewBatches)
	_ = s.sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM backup_records WHERE verification_status = 'verified'`).Scan(&st.VerifiedBackupsCount)
	_ = s.sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM watch_states WHERE watch_status = 'watching'`).Scan(&st.WatchingNowCount)

	rows, err := s.sqlDB.QueryContext(ctx, `SELECT issue_kind, COUNT(*) FROM library_issues WHERE resolved = 0 GROUP BY issue_kind`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var k string
			var c int
			if err := rows.Scan(&k, &c); err == nil {
				st.IssuesByKind[k] = c
			}
		}
	}
	return st, nil
}

func (s *Service) insertIssue(ctx context.Context, kind, sev, workID, fileID, diskID, tAr, tEn, dAr, dEn, fAr, fEn, nowStr string) {
	var wPtr, fPtr, dPtr any = nil, nil, nil
	if workID != "" {
		wPtr = workID
	}
	if fileID != "" {
		fPtr = fileID
	}
	if diskID != "" {
		dPtr = diskID
	}
	_, _ = s.sqlDB.ExecContext(ctx, `
		INSERT INTO library_issues (
			id, issue_kind, severity, work_id, file_id, disk_id,
			title_ar, title_en, details_ar, details_en, suggested_fix_ar, suggested_fix_en, resolved, detected_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?)
	`, domain.NewImmutableID(), kind, sev, wPtr, fPtr, dPtr, tAr, tEn, dAr, dEn, fAr, fEn, nowStr)
}
