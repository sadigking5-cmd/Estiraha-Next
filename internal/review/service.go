package review

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/sadigking5-cmd/Estiraha-Next/internal/domain"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/library"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/naming"
)

type ReviewBatchView struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	BatchType     string `json:"batchType"`
	BatchStatus   string `json:"batchStatus"`
	TotalItems    int    `json:"totalItems"`
	ApprovedItems int    `json:"approvedItems"`
	PendingItems  int    `json:"pendingItems"`
	ExecutedItems int    `json:"executedItems"`
	ApprovedByAt  string `json:"approvedByUserAt"`
	CreatedAt     string `json:"createdAt"`
}

type ReviewItemView struct {
	ID                  string   `json:"id"`
	BatchID             string   `json:"batchId"`
	FileID              string   `json:"fileId"`
	WorkID              string   `json:"workId"`
	ActionKind          string   `json:"actionKind"`
	SourceDiskID        string   `json:"sourceDiskId"`
	DiskLabel           string   `json:"diskLabel"`
	OldRelPath          string   `json:"oldRelPath"`
	OldFullPath         string   `json:"oldFullPath"`
	OldName             string   `json:"oldName"`
	TargetDiskID        string   `json:"targetDiskId"`
	ProposedRelPath     string   `json:"proposedRelPath"`
	EffectiveRelPath    string   `json:"effectiveRelPath"`
	EffectiveFullPath   string   `json:"effectiveFullPath"`
	ProposedName        string   `json:"proposedName"`
	UserEditedRelPath   string   `json:"userEditedRelPath"`
	UserEditedName      string   `json:"userEditedName"`
	ReasonDescription   string   `json:"reasonDescription"`
	DecisionStatus      string   `json:"decisionStatus"`
	ConfidenceScore     float64  `json:"confidenceScore"`
	ConflictDetected    bool     `json:"conflictDetected"`
	ConflictDetails     string   `json:"conflictDetails"`
	ProtectedShortsFile bool     `json:"protectedShortsFile"`
	LinkedCompanions    []string `json:"linkedCompanions"`
}

type ItemDecisionInput struct {
	ItemID            string `json:"itemId"`
	Decision          string `json:"decision"`
	UserEditedRelPath string `json:"userEditedRelPath"`
	UserEditedName    string `json:"userEditedName"`
}

type Service struct {
	sqlDB *sql.DB
}

func NewService(sqlDB *sql.DB) *Service {
	return &Service{sqlDB: sqlDB}
}

func (s *Service) GenerateOrganizationPlan(ctx context.Context, customTitle string) (ReviewBatchView, error) {
	nowStr := time.Now().UTC().Format(time.RFC3339Nano)
	title := strings.TrimSpace(customTitle)
	if title == "" {
		title = fmt.Sprintf("خطة تنظيم المكتبة — Library Organization Plan (%s)", time.Now().Format("2006-01-02 15:04"))
	}

	batchID := domain.NewImmutableID()
	if _, err := s.sqlDB.ExecContext(ctx, `
		INSERT INTO review_batches (id, title, batch_type, batch_status, total_items, approved_items, created_at, updated_at)
		VALUES (?, ?, 'organize_library', 'awaiting_approval', 0, 0, ?, ?)
	`, batchID, title, nowStr, nowStr); err != nil {
		return ReviewBatchView{}, err
	}

	totalAdded := 0

	mRows, err := s.sqlDB.QueryContext(ctx, `
		SELECT w.id, w.title_original, COALESCE(w.production_year, 0), w.canonical_folder_name,
		       w.identification_status, w.confidence_score
		FROM works w
		WHERE w.sub_format = 'standalone' AND w.media_type IN ('movie', 'documentary_movie', 'tv_program')
	`)
	if err != nil {
		return ReviewBatchView{}, err
	}
	type moviePlanWork struct {
		id, title, canon, status string
		year                     int
		conf                     float64
	}
	var mWorks []moviePlanWork
	for mRows.Next() {
		var mw moviePlanWork
		if err := mRows.Scan(&mw.id, &mw.title, &mw.year, &mw.canon, &mw.status, &mw.conf); err == nil {
			mWorks = append(mWorks, mw)
		}
	}
	mRows.Close()

	for _, mw := range mWorks {
		files := s.loadPresentWorkFiles(ctx, mw.id, "")
		if len(files) == 0 {
			continue
		}

		if (mw.status == "needs_review" || mw.status == "unidentified" || mw.status == "unknown_program" || mw.year < 1888) && mw.status != "locked_by_user" {
			f := files[0]
			itemID := domain.NewImmutableID()
			_, _ = s.sqlDB.ExecContext(ctx, `
				INSERT INTO review_items (
					id, batch_id, file_id, work_id, action_kind, source_disk_id, old_rel_path, old_name,
					target_disk_id, proposed_rel_path, proposed_name, reason_description,
					decision_status, confidence_score, conflict_detected, conflict_details, created_at
				) VALUES (?, ?, ?, ?, 'classify_program', ?, ?, ?, ?, ?, ?, ?, 'deferred', ?, 1, ?, ?)
			`, itemID, batchID, f.fileID, mw.id, f.diskID, f.relPath, f.filename, f.diskID, f.relPath, f.filename,
				"ملف منخفض الثقة / غير مؤكد سنة الإنتاج — يتطلب مراجعة يدوية قبل إعادة التسمية (Unidentified / Low Confidence)",
				mw.conf, "Needs manual year/title confirmation before naming", nowStr)
			totalAdded++
			continue
		}

		byRoot := map[string][]workFileRow{}
		for _, f := range files {
			if f.keepDecision == "candidate_error_move" {
				continue
			}
			byRoot[f.diskID+"|"+f.rootRel] = append(byRoot[f.diskID+"|"+f.rootRel], f)
		}

		for _, groupFiles := range byRoot {
			// Keep only the best-scored video per disk for canonical primary naming; leave same-disk extra copies untouched unless reviewed
			filtered := []workFileRow{}
			seenVideo := false
			subCount := 0
			for _, gf := range groupFiles {
				if gf.kind == "video" {
					if seenVideo {
						continue
					}
					seenVideo = true
				}
				if gf.kind == "subtitle" {
					subCount++
				}
				filtered = append(filtered, gf)
			}

			comps := make([]naming.CompanionFilePlan, 0, len(filtered))
			for _, gf := range filtered {
				comps = append(comps, naming.CompanionFilePlan{
					OriginalFilename: gf.filename,
					Extension:        gf.ext,
					FileKind:         gf.kind,
				})
			}

			folderStem, planned, pErr := naming.PlanMovieBundle(mw.title, mw.year, comps, subCount > 1)
			if pErr != nil {
				continue
			}

			linkedNames := make([]string, 0, len(planned))
			for _, pl := range planned {
				linkedNames = append(linkedNames, pl.ProposedFilename)
			}
			linkedJSON, _ := json.Marshal(linkedNames)

			for idx, gf := range filtered {
				targetRelDir := folderStem
				if gf.rootRel != "" && gf.rootRel != "." {
					targetRelDir = strings.TrimRight(gf.rootRel, "/") + "/" + folderStem
				}
				proposedName := planned[idx].ProposedFilename
				proposedRel := targetRelDir + "/" + proposedName

				if normalizeSlash(proposedRel) == normalizeSlash(gf.relPath) {
					continue
				}

				conflict, conflictMsg := s.detectDestinationConflict(ctx, gf.diskID, gf.mountPath, proposedRel, gf.fileID)
				action := "move"
				if filepath.ToSlash(filepath.Dir(filepath.FromSlash(gf.relPath))) == targetRelDir {
					action = "rename"
				}
				reason := fmt.Sprintf("توحيد تسمية الفيلم ومرفقاته وفق القاعدة الرسمية (%s)", folderStem)
				if planned[idx].ProtectedShorts {
					reason = fmt.Sprintf("الحفاظ الإلزامي على ملف .shorts المرافق للفيلم (%s)", proposedName)
				}

				_, err := s.sqlDB.ExecContext(ctx, `
					INSERT INTO review_items (
						id, batch_id, file_id, work_id, action_kind, source_disk_id, old_rel_path, old_name,
						target_disk_id, proposed_rel_path, proposed_name, reason_description,
						decision_status, confidence_score, linked_files_json, conflict_detected, conflict_details, created_at
					) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?, ?, ?, ?)
				`, domain.NewImmutableID(), batchID, gf.fileID, mw.id, action, gf.diskID, gf.relPath, gf.filename,
					gf.diskID, proposedRel, proposedName, reason, mw.conf, string(linkedJSON), boolInt(conflict), conflictMsg, nowStr)
				if err == nil {
					totalAdded++
				}
			}
		}
	}

	eRows, err := s.sqlDB.QueryContext(ctx, `
		SELECT ep.id, ep.episode_number, sn.season_number, w.id, w.canonical_folder_name,
		       w.identification_status, w.confidence_score
		FROM episodes ep
		JOIN seasons sn ON sn.id = ep.season_id
		JOIN works w ON w.id = ep.work_id
		WHERE ep.is_missing = 0
	`)
	if err == nil {
		type epPlanRow struct {
			epID, workID, seriesTitle, status string
			epNum, seasonNum                  int
			conf                              float64
		}
		var eps []epPlanRow
		for eRows.Next() {
			var er epPlanRow
			if err := eRows.Scan(&er.epID, &er.epNum, &er.seasonNum, &er.workID, &er.seriesTitle, &er.status, &er.conf); err == nil {
				eps = append(eps, er)
			}
		}
		eRows.Close()

		for _, er := range eps {
			files := s.loadPresentWorkFiles(ctx, er.workID, er.epID)
			if len(files) == 0 {
				continue
			}
			byRoot := map[string][]workFileRow{}
			for _, f := range files {
				if f.keepDecision == "candidate_error_move" {
					continue
				}
				byRoot[f.diskID+"|"+f.rootRel] = append(byRoot[f.diskID+"|"+f.rootRel], f)
			}

			for _, groupFiles := range byRoot {
				filtered := []workFileRow{}
				seenVideo := false
				subCount := 0
				for _, gf := range groupFiles {
					if gf.kind == "video" {
						if seenVideo {
							continue
						}
						seenVideo = true
					}
					if gf.kind == "subtitle" {
						subCount++
					}
					filtered = append(filtered, gf)
				}

				comps := make([]naming.CompanionFilePlan, 0, len(filtered))
				for _, gf := range filtered {
					comps = append(comps, naming.CompanionFilePlan{
						OriginalFilename: gf.filename,
						Extension:        gf.ext,
						FileKind:         gf.kind,
					})
				}

				sFolder, snFolder, planned, pErr := naming.PlanEpisodeBundle(er.seriesTitle, er.seasonNum, er.epNum, comps, subCount > 1)
				if pErr != nil {
					continue
				}

				for idx, gf := range filtered {
					basePrefix := sFolder + "/" + snFolder
					if gf.rootRel != "" && gf.rootRel != "." {
						basePrefix = strings.TrimRight(gf.rootRel, "/") + "/" + basePrefix
					}
					proposedName := planned[idx].ProposedFilename
					proposedRel := basePrefix + "/" + proposedName

					if normalizeSlash(proposedRel) == normalizeSlash(gf.relPath) {
						continue
					}
					conflict, conflictMsg := s.detectDestinationConflict(ctx, gf.diskID, gf.mountPath, proposedRel, gf.fileID)
					reason := fmt.Sprintf("تنظيم المسلسل (%s/%s/%s) بدون سنة إنتاج", sFolder, snFolder, proposedName)
					if planned[idx].ProtectedShorts {
						reason = fmt.Sprintf("الحفاظ الإلزامي على ملف .shorts المرافق للحلقة (%s)", proposedName)
					}

					_, insErr := s.sqlDB.ExecContext(ctx, `
						INSERT INTO review_items (
							id, batch_id, file_id, work_id, action_kind, source_disk_id, old_rel_path, old_name,
							target_disk_id, proposed_rel_path, proposed_name, reason_description,
							decision_status, confidence_score, conflict_detected, conflict_details, created_at
						) VALUES (?, ?, ?, ?, 'move', ?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?, ?, ?)
					`, domain.NewImmutableID(), batchID, gf.fileID, er.workID, gf.diskID, gf.relPath, gf.filename,
						gf.diskID, proposedRel, proposedName, reason, er.conf, boolInt(conflict), conflictMsg, nowStr)
					if insErr == nil {
						totalAdded++
					}
				}
			}
		}
	}

	pRows, err := s.sqlDB.QueryContext(ctx, `
		SELECT mf.id, mf.disk_id, d.mount_path, COALESCE(lr.relative_root_path, ''),
		       mf.relative_path, mf.filename, mf.extension, w.id, w.canonical_folder_name, w.confidence_score
		FROM media_files mf
		JOIN works w ON w.id = mf.work_id
		JOIN disks d ON d.id = mf.disk_id
		LEFT JOIN library_roots lr ON lr.id = mf.root_id
		WHERE w.sub_format = 'episodic' AND mf.episode_id IS NULL AND mf.file_kind = 'image' AND mf.presence_status = 'present'
	`)
	if err == nil {
		type sPoster struct {
			fID, dID, mp, rootRel, rel, fname, ext, wID, sCanon string
			conf                                                float64
		}
		var posters []sPoster
		for pRows.Next() {
			var sp sPoster
			if err := pRows.Scan(&sp.fID, &sp.dID, &sp.mp, &sp.rootRel, &sp.rel, &sp.fname, &sp.ext, &sp.wID, &sp.sCanon, &sp.conf); err == nil {
				posters = append(posters, sp)
			}
		}
		pRows.Close()

		for _, sp := range posters {
			sFolder, fErr := naming.FormatSeriesFolder(sp.sCanon)
			if fErr != nil {
				continue
			}
			proposedName := "poster" + strings.ToLower(sp.ext)
			proposedRel := sFolder + "/" + proposedName
			if sp.rootRel != "" && sp.rootRel != "." {
				proposedRel = strings.TrimRight(sp.rootRel, "/") + "/" + proposedRel
			}
			if normalizeSlash(proposedRel) == normalizeSlash(sp.rel) {
				continue
			}
			conflict, conflictMsg := s.detectDestinationConflict(ctx, sp.dID, sp.mp, proposedRel, sp.fID)
			_, insErr := s.sqlDB.ExecContext(ctx, `
				INSERT INTO review_items (
					id, batch_id, file_id, work_id, action_kind, source_disk_id, old_rel_path, old_name,
					target_disk_id, proposed_rel_path, proposed_name, reason_description,
					decision_status, confidence_score, conflict_detected, conflict_details, created_at
				) VALUES (?, ?, ?, ?, 'move', ?, ?, ?, ?, ?, ?, 'بوستر المسلسل الرئيسي (poster)', 'pending', ?, ?, ?, ?)
			`, domain.NewImmutableID(), batchID, sp.fID, sp.wID, sp.dID, sp.rel, sp.fname, sp.dID, proposedRel, proposedName, sp.conf, boolInt(conflict), conflictMsg, nowStr)
			if insErr == nil {
				totalAdded++
			}
		}
	}

	_, _ = s.sqlDB.ExecContext(ctx, `
		UPDATE review_batches SET total_items = ?, updated_at = ? WHERE id = ?
	`, totalAdded, nowStr, batchID)

	return s.GetBatch(ctx, batchID)
}

func (s *Service) DecideItem(ctx context.Context, in ItemDecisionInput) (ReviewItemView, error) {
	switch in.Decision {
	case "approved", "rejected", "deferred", "excluded", "pending":
	default:
		return ReviewItemView{}, fmt.Errorf("review: unsupported decision %q", in.Decision)
	}

	var oldName, proposedRel, proposedName, diskID, mountPath, fileID string
	var conflictInt int
	err := s.sqlDB.QueryRowContext(ctx, `
		SELECT ri.old_name, ri.proposed_rel_path, ri.proposed_name, ri.target_disk_id, d.mount_path, COALESCE(ri.file_id, ''), ri.conflict_detected
		FROM review_items ri
		JOIN disks d ON d.id = ri.target_disk_id
		WHERE ri.id = ?
	`, in.ItemID).Scan(&oldName, &proposedRel, &proposedName, &diskID, &mountPath, &fileID, &conflictInt)
	if err != nil {
		return ReviewItemView{}, err
	}

	var editedRel, editedName sql.NullString
	effectiveRel := proposedRel

	if strings.TrimSpace(in.UserEditedName) != "" || strings.TrimSpace(in.UserEditedRelPath) != "" {
		newName := strings.TrimSpace(in.UserEditedName)
		if newName == "" {
			newName = filepath.Base(filepath.FromSlash(in.UserEditedRelPath))
		}
		if naming.IsShortsExtension(oldName) && !naming.IsShortsExtension(newName) {
			return ReviewItemView{}, errors.New("safety law: .shorts extension is protected and cannot be removed or changed")
		}
		newRel := strings.TrimSpace(in.UserEditedRelPath)
		if newRel == "" {
			parent := filepath.ToSlash(filepath.Dir(filepath.FromSlash(proposedRel)))
			newRel = parent + "/" + newName
		}
		editedName = sql.NullString{String: newName, Valid: true}
		editedRel = sql.NullString{String: filepath.ToSlash(newRel), Valid: true}
		effectiveRel = editedRel.String

		conflict, msg := s.detectDestinationConflict(ctx, diskID, mountPath, effectiveRel, fileID)
		conflictInt = boolInt(conflict)
		_, _ = s.sqlDB.ExecContext(ctx, `UPDATE review_items SET conflict_detected = ?, conflict_details = ? WHERE id = ?`, conflictInt, msg, in.ItemID)
	}

	if in.Decision == "approved" && conflictInt == 1 {
		return ReviewItemView{}, ErrTargetCollisionAbort
	}

	nowStr := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.sqlDB.ExecContext(ctx, `
		UPDATE review_items
		SET decision_status = ?,
		    user_edited_rel_path = COALESCE(?, user_edited_rel_path),
		    user_edited_name = COALESCE(?, user_edited_name),
		    decided_at = ?
		WHERE id = ?
	`, in.Decision, editedRel, editedName, nowStr, in.ItemID)
	if err != nil {
		return ReviewItemView{}, err
	}

	var batchID string
	_ = s.sqlDB.QueryRowContext(ctx, `SELECT batch_id FROM review_items WHERE id = ?`, in.ItemID).Scan(&batchID)
	s.recomputeBatchStatus(ctx, batchID)

	items, err := s.ListBatchItems(ctx, batchID)
	if err != nil {
		return ReviewItemView{}, err
	}
	for _, it := range items {
		if it.ID == in.ItemID {
			return it, nil
		}
	}
	return ReviewItemView{}, sql.ErrNoRows
}

func (s *Service) ApproveAllSafeItems(ctx context.Context, batchID string) (ReviewBatchView, error) {
	nowStr := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.sqlDB.ExecContext(ctx, `
		UPDATE review_items
		SET decision_status = 'approved', decided_at = ?
		WHERE batch_id = ? AND decision_status = 'pending' AND conflict_detected = 0
		  AND action_kind IN ('move', 'rename')
	`, nowStr, batchID)
	if err != nil {
		return ReviewBatchView{}, err
	}
	s.recomputeBatchStatus(ctx, batchID)
	return s.GetBatch(ctx, batchID)
}

func (s *Service) recomputeBatchStatus(ctx context.Context, batchID string) {
	var total, approved int
	_ = s.sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM review_items WHERE batch_id = ?`, batchID).Scan(&total)
	_ = s.sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM review_items WHERE batch_id = ? AND decision_status = 'approved'`, batchID).Scan(&approved)

	status := "awaiting_approval"
	var approvedAt sql.NullString
	nowStr := time.Now().UTC().Format(time.RFC3339Nano)
	if approved > 0 && approved == total {
		status = "approved"
		approvedAt = sql.NullString{String: nowStr, Valid: true}
	} else if approved > 0 {
		status = "partially_approved"
		approvedAt = sql.NullString{String: nowStr, Valid: true}
	}

	_, _ = s.sqlDB.ExecContext(ctx, `
		UPDATE review_batches
		SET total_items = ?, approved_items = ?, batch_status = ?,
		    approved_by_user_at = COALESCE(?, approved_by_user_at), updated_at = ?
		WHERE id = ? AND batch_status NOT IN ('completed', 'rolled_back', 'executing')
	`, total, approved, status, approvedAt, nowStr, batchID)
}

func (s *Service) GetBatch(ctx context.Context, batchID string) (ReviewBatchView, error) {
	batches, err := s.ListBatches(ctx)
	if err != nil {
		return ReviewBatchView{}, err
	}
	for _, b := range batches {
		if b.ID == batchID {
			return b, nil
		}
	}
	return ReviewBatchView{}, sql.ErrNoRows
}

func (s *Service) ListBatches(ctx context.Context) ([]ReviewBatchView, error) {
	rows, err := s.sqlDB.QueryContext(ctx, `
		SELECT b.id, b.title, b.batch_type, b.batch_status, b.total_items, b.approved_items,
		       COALESCE(b.approved_by_user_at, ''), b.created_at,
		       (SELECT COUNT(*) FROM review_items ri WHERE ri.batch_id = b.id AND ri.decision_status = 'pending'),
		       (SELECT COUNT(*) FROM review_items ri WHERE ri.batch_id = b.id AND ri.decision_status = 'executed')
		FROM review_batches b
		ORDER BY b.created_at DESC
		LIMIT 50
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []ReviewBatchView{}
	for rows.Next() {
		var v ReviewBatchView
		if err := rows.Scan(
			&v.ID, &v.Title, &v.BatchType, &v.BatchStatus, &v.TotalItems, &v.ApprovedItems,
			&v.ApprovedByAt, &v.CreatedAt, &v.PendingItems, &v.ExecutedItems,
		); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Service) ListBatchItems(ctx context.Context, batchID string) ([]ReviewItemView, error) {
	rows, err := s.sqlDB.QueryContext(ctx, `
		SELECT ri.id, ri.batch_id, COALESCE(ri.file_id, ''), COALESCE(ri.work_id, ''),
		       ri.action_kind, ri.source_disk_id, d.label, d.mount_path,
		       ri.old_rel_path, ri.old_name, ri.target_disk_id,
		       ri.proposed_rel_path, ri.proposed_name,
		       COALESCE(ri.user_edited_rel_path, ''), COALESCE(ri.user_edited_name, ''),
		       ri.reason_description, ri.decision_status, ri.confidence_score,
		       ri.conflict_detected, ri.conflict_details, ri.linked_files_json
		FROM review_items ri
		JOIN disks d ON d.id = ri.source_disk_id
		WHERE ri.batch_id = ?
		ORDER BY ri.old_rel_path ASC
	`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []ReviewItemView{}
	for rows.Next() {
		var it ReviewItemView
		var mountPath, linkedJSON string
		var confInt int
		if err := rows.Scan(
			&it.ID, &it.BatchID, &it.FileID, &it.WorkID,
			&it.ActionKind, &it.SourceDiskID, &it.DiskLabel, &mountPath,
			&it.OldRelPath, &it.OldName, &it.TargetDiskID,
			&it.ProposedRelPath, &it.ProposedName,
			&it.UserEditedRelPath, &it.UserEditedName,
			&it.ReasonDescription, &it.DecisionStatus, &it.ConfidenceScore,
			&confInt, &it.ConflictDetails, &linkedJSON,
		); err != nil {
			return nil, err
		}
		it.ConflictDetected = confInt == 1
		it.OldFullPath = library.ResolveFullPath(mountPath, it.OldRelPath)
		it.EffectiveRelPath = it.ProposedRelPath
		if it.UserEditedRelPath != "" {
			it.EffectiveRelPath = it.UserEditedRelPath
		}
		it.EffectiveFullPath = library.ResolveFullPath(mountPath, it.EffectiveRelPath)
		it.ProtectedShortsFile = naming.IsShortsExtension(it.OldName)
		_ = json.Unmarshal([]byte(linkedJSON), &it.LinkedCompanions)
		if it.LinkedCompanions == nil {
			it.LinkedCompanions = []string{}
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

type workFileRow struct {
	fileID, diskID, mountPath, rootRel, relPath, filename, ext, kind, keepDecision string
}

func (s *Service) loadPresentWorkFiles(ctx context.Context, workID, epID string) []workFileRow {
	q := `
		SELECT mf.id, mf.disk_id, d.mount_path, COALESCE(lr.relative_root_path, ''),
		       mf.relative_path, mf.filename, mf.extension, mf.file_kind, mf.keep_decision
		FROM media_files mf
		JOIN disks d ON d.id = mf.disk_id
		LEFT JOIN library_roots lr ON lr.id = mf.root_id
		LEFT JOIN media_versions mv ON mv.file_id = mf.id
		WHERE mf.work_id = ? AND mf.presence_status = 'present' AND d.connection_state = 'online'
	`
	args := []any{workID}
	if epID != "" {
		q += ` AND mf.episode_id = ?`
		args = append(args, epID)
	}
	q += ` ORDER BY CASE mf.file_kind WHEN 'video' THEN 1 WHEN 'subtitle' THEN 2 WHEN 'shorts' THEN 3 ELSE 4 END, COALESCE(mv.computed_quality_score, 0) DESC`

	rows, err := s.sqlDB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []workFileRow
	for rows.Next() {
		var r workFileRow
		if err := rows.Scan(&r.fileID, &r.diskID, &r.mountPath, &r.rootRel, &r.relPath, &r.filename, &r.ext, &r.kind, &r.keepDecision); err == nil {
			out = append(out, r)
		}
	}
	return out
}

func (s *Service) detectDestinationConflict(ctx context.Context, diskID, mountPath, targetRel, selfFileID string) (bool, string) {
	var otherID string
	err := s.sqlDB.QueryRowContext(ctx, `
		SELECT id FROM media_files WHERE disk_id = ? AND lower(relative_path) = lower(?) AND id != ? AND presence_status = 'present'
	`, diskID, targetRel, selfFileID).Scan(&otherID)
	if err == nil && otherID != "" {
		return true, "Target path already indexed by another file in SQLite"
	}
	return false, ""
}

func normalizeSlash(s string) string {
	return strings.Trim(filepath.ToSlash(s), "/")
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
