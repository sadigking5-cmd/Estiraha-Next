package review

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sadigking5-cmd/Estiraha-Next/internal/domain"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/errorsbox"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/library"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/naming"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/scanner"
)

type ReviewBatchView struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	BatchType     string `json:"batchType"`
	BatchStatus   string `json:"batchStatus"`
	TotalItems    int    `json:"totalItems"`
	ApprovedItems int    `json:"approvedItems"`
	PendingItems  int    `json:"pendingItems"`
	RejectedItems int    `json:"rejectedItems"`
	ExecutedItems int    `json:"executedItems"`
	ApprovedByAt  string `json:"approvedByUserAt"`
	CreatedAt     string `json:"createdAt"`
}

type ReviewItemView struct {
	ID                  string   `json:"id"`
	BatchID             string   `json:"batchId"`
	FileID              string   `json:"fileId"`
	WorkID              string   `json:"workId"`
	WorkTitle           string   `json:"workTitle"`
	WorkMediaType       string   `json:"workMediaType"`
	ItemFileKind        string   `json:"itemFileKind"`
	MediaFileKind       string   `json:"mediaFileKind"`
	ActionKind          string   `json:"actionKind"`
	ErrorCategoryCode   string   `json:"errorCategoryCode"`
	SourceDiskID        string   `json:"sourceDiskId"`
	DiskLabel           string   `json:"diskLabel"`
	OldRelPath          string   `json:"oldRelPath"`
	OldDirPath          string   `json:"oldDirPath"`
	OldFullPath         string   `json:"oldFullPath"`
	OldName             string   `json:"oldName"`
	TargetDiskID        string   `json:"targetDiskId"`
	ProposedRelPath     string   `json:"proposedRelPath"`
	ProposedDirPath     string   `json:"proposedDirPath"`
	EffectiveRelPath    string   `json:"effectiveRelPath"`
	EffectiveDirPath    string   `json:"effectiveDirPath"`
	EffectiveFullPath   string   `json:"effectiveFullPath"`
	ProposedName        string   `json:"proposedName"`
	EffectiveName       string   `json:"effectiveName"`
	UserEditedRelPath   string   `json:"userEditedRelPath"`
	UserEditedName      string   `json:"userEditedName"`
	ReasonDescription   string   `json:"reasonDescription"`
	DecisionStatus      string   `json:"decisionStatus"`
	ConfidenceScore     float64  `json:"confidenceScore"`
	ConflictDetected    bool     `json:"conflictDetected"`
	ConflictDetails     string   `json:"conflictDetails"`
	ProtectedShortsFile bool     `json:"protectedShortsFile"`
	LinkedCompanions    []string `json:"linkedCompanions"`
	SafetyChecks        []string `json:"safetyChecks"`
	FileSizeBytes       int64    `json:"fileSizeBytes"`
	ResolutionLabel     string   `json:"resolutionLabel"`
	VideoCodec          string   `json:"videoCodec"`
	OperationState      string   `json:"operationState"`
}

type ReviewItemDetailView struct {
	Item            ReviewItemView `json:"item"`
	Extension       string         `json:"extension"`
	FileSHA256      string         `json:"fileSha256"`
	DurationSeconds float64        `json:"durationSeconds"`
	VideoWidth      int            `json:"videoWidth"`
	VideoHeight     int            `json:"videoHeight"`
	ProbeStatus     string         `json:"probeStatus"`
	IntegrityStatus string         `json:"integrityStatus"`
	PresenceStatus  string         `json:"presenceStatus"`
	ProductionYear  int            `json:"productionYear"`
	WorkStatus      string         `json:"workStatus"`
	LastJournalMsg  string         `json:"lastJournalMsg"`
}

type ItemDecisionInput struct {
	ItemID            string `json:"itemId"`
	Decision          string `json:"decision"`
	UserEditedRelPath string `json:"userEditedRelPath"`
	UserEditedName    string `json:"userEditedName"`
	UserEditedDirPath string `json:"userEditedDirPath"`
}

type BulkDecisionInput struct {
	BatchID  string   `json:"batchId"`
	ItemIDs  []string `json:"itemIds"`
	Decision string   `json:"decision"`
}

type BulkDecisionResult struct {
	BatchID      string   `json:"batchId"`
	UpdatedCount int      `json:"updatedCount"`
	SkippedCount int      `json:"skippedCount"`
	Warnings     []string `json:"warnings"`
}

type ManifestStepSpec struct {
	Sequence            int    `json:"sequence"`
	ReviewItemID        string `json:"reviewItemId"`
	FileID              string `json:"fileId"`
	ItemFileKind        string `json:"itemFileKind"`
	MediaFileKind       string `json:"mediaFileKind"`
	ActionKind          string `json:"actionKind"`
	ErrorCategoryCode   string `json:"errorCategoryCode"`
	DiskID              string `json:"diskId"`
	DiskLabel           string `json:"diskLabel"`
	MountPath           string `json:"mountPath"`
	RootID              string `json:"rootId"`
	RootRelPath         string `json:"rootRelPath"`
	OldRelPath          string `json:"oldRelPath"`
	OldName             string `json:"oldName"`
	EffectiveRelPath    string `json:"effectiveRelPath"`
	EffectiveName       string `json:"effectiveName"`
	ProtectedShortsFile bool   `json:"protectedShortsFile"`
	ReasonDescription   string `json:"reasonDescription"`
	ApprovedAt          string `json:"approvedAt"`
}

type ExecutionManifestSummary struct {
	BatchID                   string             `json:"batchId"`
	BatchTitle                string             `json:"batchTitle"`
	BatchStatus               string             `json:"batchStatus"`
	ManifestSHA256            string             `json:"manifestSha256"`
	TotalApprovedOperations   int                `json:"totalApprovedOperations"`
	FilesToRename             int                `json:"filesToRename"`
	FilesToMove               int                `json:"filesToMove"`
	FoldersToCreate           int                `json:"foldersToCreate"`
	EmptyFoldersToMoveErrors  int                `json:"emptyFoldersToMoveErrors"`
	SubtitlesToAssociate      int                `json:"subtitlesToAssociate"`
	PostersArtworkToAssociate int                `json:"postersArtworkToAssociate"`
	ShortsToPreserve          int                `json:"shortsToPreserve"`
	UnresolvedConflicts       int                `json:"unresolvedConflicts"`
	UnchangedFilesCount       int                `json:"unchangedFilesCount"`
	TargetFolders             []string           `json:"targetFolders"`
	Steps                     []ManifestStepSpec `json:"steps"`
}

type Service struct {
	sqlDB  *sql.DB
	errBox *errorsbox.Service
}

func NewService(sqlDB *sql.DB) *Service {
	return &Service{
		sqlDB:  sqlDB,
		errBox: errorsbox.NewService(sqlDB),
	}
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
	reservedDestByDisk := map[string]map[string]bool{}
	reserveForDisk := func(diskID string) map[string]bool {
		if reservedDestByDisk[diskID] == nil {
			reservedDestByDisk[diskID] = map[string]bool{}
		}
		return reservedDestByDisk[diskID]
	}

	plannedMoveOutByDisk := map[string]map[string]bool{}
	markPlannedMoveOut := func(diskID, oldRel, newRel string) {
		if normalizeSlash(oldRel) == normalizeSlash(newRel) {
			return
		}
		if plannedMoveOutByDisk[diskID] == nil {
			plannedMoveOutByDisk[diskID] = map[string]bool{}
		}
		plannedMoveOutByDisk[diskID][strings.ToLower(normalizeSlash(oldRel))] = true
	}

	// 1. STANDALONE MOVIES, DOCUMENTARIES & PROGRAMS BUNDLING
	mRows, err := s.sqlDB.QueryContext(ctx, `
		SELECT w.id, w.title_original, COALESCE(w.production_year, 0), w.canonical_folder_name,
		       w.identification_status, w.confidence_score, w.poster_rel_path
		FROM works w
		WHERE w.sub_format = 'standalone' AND w.media_type IN ('movie', 'documentary_movie', 'tv_program')
	`)
	if err != nil {
		return ReviewBatchView{}, err
	}
	type moviePlanWork struct {
		id, title, canon, status, posterFileID string
		year                                   int
		conf                                   float64
	}
	var mWorks []moviePlanWork
	for mRows.Next() {
		var mw moviePlanWork
		if err := mRows.Scan(&mw.id, &mw.title, &mw.year, &mw.canon, &mw.status, &mw.conf, &mw.posterFileID); err == nil {
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
			checksJSON, _ := json.Marshal([]string{"manual_review_required", "no_auto_rename_low_confidence"})
			_, _ = s.sqlDB.ExecContext(ctx, `
				INSERT INTO review_items (
					id, batch_id, file_id, work_id, item_file_kind, action_kind, error_category_code,
					source_disk_id, old_rel_path, old_name, target_disk_id, proposed_rel_path, proposed_name,
					reason_description, decision_status, confidence_score, conflict_detected, conflict_details,
					safety_checks_json, created_at
				) VALUES (?, ?, ?, ?, 'file', 'classify_program', '', ?, ?, ?, ?, ?, ?, ?, 'deferred', ?, 1, ?, ?, ?)
			`, domain.NewImmutableID(), batchID, f.fileID, mw.id, f.diskID, f.relPath, f.filename,
				f.diskID, f.relPath, f.filename,
				"ملف منخفض الثقة / غير مؤكد سنة الإنتاج — يتطلب مراجعة يدوية قبل إعادة التسمية (Unidentified / Low Confidence)",
				mw.conf, "Needs manual year/title confirmation before naming", string(checksJSON), nowStr)
			totalAdded++
			continue
		}

		byRoot := map[string][]workFileRow{}
		for _, f := range files {
			if f.keepDecision == "candidate_error_move" || f.anomalyCategory != "" {
				continue
			}
			byRoot[f.diskID+"|"+f.rootRel] = append(byRoot[f.diskID+"|"+f.rootRel], f)
		}

		for _, groupFiles := range byRoot {
			subCount := 0
			imageCount := 0
			for _, gf := range groupFiles {
				if gf.kind == "subtitle" {
					subCount++
				}
				if gf.kind == "image" {
					imageCount++
				}
			}

			comps := make([]naming.CompanionFilePlan, 0, len(groupFiles))
			for _, gf := range groupFiles {
				isPrimaryPoster := false
				if gf.kind == "image" {
					if mw.posterFileID != "" && gf.fileID == mw.posterFileID {
						isPrimaryPoster = true
					} else if mw.posterFileID == "" && imageCount == 1 {
						isPrimaryPoster = true
					}
				}
				verTag := strings.TrimSpace(gf.resLabel)
				if verTag == "" {
					verTag = strings.TrimSpace(gf.vCodec)
				}
				comps = append(comps, naming.CompanionFilePlan{
					OriginalFilename: gf.filename,
					OriginalRelPath:  gf.relPath,
					Extension:        gf.ext,
					FileKind:         gf.kind,
					LanguageHint:     gf.subLang,
					IsPrimaryPoster:  isPrimaryPoster,
					VersionTag:       verTag,
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

			diskReserved := reserveForDisk(groupFiles[0].diskID)

			for idx, gf := range groupFiles {
				targetRelDir := folderStem
				if gf.rootRel != "" && gf.rootRel != "." {
					targetRelDir = strings.TrimRight(gf.rootRel, "/") + "/" + folderStem
				}
				proposedName := planned[idx].ProposedFilename
				proposedRel := targetRelDir + "/" + proposedName

				if normalizeSlash(proposedRel) == normalizeSlash(gf.relPath) {
					diskReserved[strings.ToLower(normalizeSlash(proposedRel))] = true
					continue
				}

				conflict, conflictMsg := s.detectDestinationConflict(ctx, gf.diskID, gf.mountPath, proposedRel, gf.fileID, diskReserved)
				diskReserved[strings.ToLower(normalizeSlash(proposedRel))] = true

				action := "move"
				if filepath.ToSlash(filepath.Dir(filepath.FromSlash(gf.relPath))) == targetRelDir {
					action = "rename"
				}

				checks := []string{"sha256_verified_move", "no_overwrite_guard", "reversible_rollback"}
				reason := fmt.Sprintf("توحيد تسمية الفيلم ومرفقاته في حزمة واحدة (%s)", folderStem)
				if planned[idx].ProtectedShorts {
					reason = fmt.Sprintf("الحفاظ الإلزامي على ملف .shorts المرافق للفيلم مع الاحتفاظ بامتداده (%s)", proposedName)
					checks = append(checks, "protected_shorts_preserved")
				} else if gf.kind == "subtitle" {
					reason = fmt.Sprintf("ضم ملف الترجمة إلى مجلد الفيلم المعتمد (%s)", proposedName)
					checks = append(checks, "multilingual_subtitle_preserved")
				} else if gf.kind == "image" {
					if planned[idx].IsPrimaryPoster {
						reason = fmt.Sprintf("اعتماد البوستر المحلي الحقيقي للفيلم (%s)", proposedName)
					} else {
						reason = fmt.Sprintf("الحفاظ على الصورة الإضافية الصالحة للفيلم دون استبدال البوستر (%s)", proposedName)
					}
					checks = append(checks, "genuine_artwork_verified")
				} else if gf.kind == "video" && strings.Contains(proposedName, "[") {
					reason = fmt.Sprintf("حفظ النسخة الإضافية للفيلم على نفس القرص دون أي حذف تلقائي (%s)", proposedName)
					checks = append(checks, "multi_version_preserved_no_auto_delete")
				}
				checksJSON, _ := json.Marshal(checks)

				_, insErr := s.sqlDB.ExecContext(ctx, `
					INSERT INTO review_items (
						id, batch_id, file_id, work_id, item_file_kind, action_kind, error_category_code,
						source_disk_id, old_rel_path, old_name, target_disk_id, proposed_rel_path, proposed_name,
						reason_description, decision_status, confidence_score, linked_files_json,
						conflict_detected, conflict_details, safety_checks_json, created_at
					) VALUES (?, ?, ?, ?, 'file', ?, '', ?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?, ?, ?, ?, ?)
				`, domain.NewImmutableID(), batchID, gf.fileID, mw.id, action, gf.diskID, gf.relPath, gf.filename,
					gf.diskID, proposedRel, proposedName, reason, mw.conf, string(linkedJSON),
					boolInt(conflict), conflictMsg, string(checksJSON), nowStr)
				if insErr == nil {
					totalAdded++
					if !conflict {
						markPlannedMoveOut(gf.diskID, gf.relPath, proposedRel)
					}
				}
			}
		}
	}

	// 2. SERIES EPISODES & SERIES POSTERS BUNDLING
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
				if f.keepDecision == "candidate_error_move" || f.anomalyCategory != "" {
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
						OriginalRelPath:  gf.relPath,
						Extension:        gf.ext,
						FileKind:         gf.kind,
						LanguageHint:     gf.subLang,
					})
				}

				sFolder, snFolder, planned, pErr := naming.PlanEpisodeBundle(er.seriesTitle, er.seasonNum, er.epNum, comps, subCount > 1)
				if pErr != nil {
					continue
				}

				diskReserved := reserveForDisk(filtered[0].diskID)

				for idx, gf := range filtered {
					basePrefix := sFolder + "/" + snFolder
					if gf.rootRel != "" && gf.rootRel != "." {
						basePrefix = strings.TrimRight(gf.rootRel, "/") + "/" + basePrefix
					}
					proposedName := planned[idx].ProposedFilename
					proposedRel := basePrefix + "/" + proposedName

					if normalizeSlash(proposedRel) == normalizeSlash(gf.relPath) {
						diskReserved[strings.ToLower(normalizeSlash(proposedRel))] = true
						continue
					}
					conflict, conflictMsg := s.detectDestinationConflict(ctx, gf.diskID, gf.mountPath, proposedRel, gf.fileID, diskReserved)
					diskReserved[strings.ToLower(normalizeSlash(proposedRel))] = true

					checks := []string{"sha256_verified_move", "no_overwrite_guard", "reversible_rollback"}
					reason := fmt.Sprintf("تنظيم المسلسل (%s/%s/%s) بدون سنة إنتاج", sFolder, snFolder, proposedName)
					if planned[idx].ProtectedShorts {
						reason = fmt.Sprintf("الحفاظ الإلزامي على ملف .shorts المرافق للحلقة (%s)", proposedName)
						checks = append(checks, "protected_shorts_preserved")
					}
					checksJSON, _ := json.Marshal(checks)

					_, insErr := s.sqlDB.ExecContext(ctx, `
						INSERT INTO review_items (
							id, batch_id, file_id, work_id, item_file_kind, action_kind, error_category_code,
							source_disk_id, old_rel_path, old_name, target_disk_id, proposed_rel_path, proposed_name,
							reason_description, decision_status, confidence_score, conflict_detected, conflict_details,
							safety_checks_json, created_at
						) VALUES (?, ?, ?, ?, 'file', 'move', '', ?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?, ?, ?, ?)
					`, domain.NewImmutableID(), batchID, gf.fileID, er.workID, gf.diskID, gf.relPath, gf.filename,
						gf.diskID, proposedRel, proposedName, reason, er.conf, boolInt(conflict), conflictMsg, string(checksJSON), nowStr)
					if insErr == nil {
						totalAdded++
						if !conflict {
							markPlannedMoveOut(gf.diskID, gf.relPath, proposedRel)
						}
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
		WHERE w.sub_format = 'episodic' AND mf.episode_id IS NULL AND mf.file_kind = 'image'
		  AND mf.presence_status = 'present' AND mf.is_apple_double = 0 AND mf.is_system_artifact = 0
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
			diskReserved := reserveForDisk(sp.dID)
			if normalizeSlash(proposedRel) == normalizeSlash(sp.rel) {
				diskReserved[strings.ToLower(normalizeSlash(proposedRel))] = true
				continue
			}
			conflict, conflictMsg := s.detectDestinationConflict(ctx, sp.dID, sp.mp, proposedRel, sp.fID, diskReserved)
			diskReserved[strings.ToLower(normalizeSlash(proposedRel))] = true
			checksJSON, _ := json.Marshal([]string{"sha256_verified_move", "genuine_artwork_verified", "reversible_rollback"})
			_, insErr := s.sqlDB.ExecContext(ctx, `
				INSERT INTO review_items (
					id, batch_id, file_id, work_id, item_file_kind, action_kind, error_category_code,
					source_disk_id, old_rel_path, old_name, target_disk_id, proposed_rel_path, proposed_name,
					reason_description, decision_status, confidence_score, conflict_detected, conflict_details,
					safety_checks_json, created_at
				) VALUES (?, ?, ?, ?, 'file', 'move', '', ?, ?, ?, ?, ?, ?, 'بوستر المسلسل الرئيسي (poster)', 'pending', ?, ?, ?, ?, ?)
			`, domain.NewImmutableID(), batchID, sp.fID, sp.wID, sp.dID, sp.rel, sp.fname, sp.dID, proposedRel, proposedName, sp.conf, boolInt(conflict), conflictMsg, string(checksJSON), nowStr)
			if insErr == nil {
				totalAdded++
				if !conflict {
					markPlannedMoveOut(sp.dID, sp.rel, proposedRel)
				}
			}
		}
	}

	// 3. UNLINKED / UNCERTAIN `.shorts` PROTECTION (Rule 5: NEVER sample/errors!)
	shRows, err := s.sqlDB.QueryContext(ctx, `
		SELECT mf.id, mf.disk_id, mf.relative_path, mf.filename
		FROM media_files mf
		JOIN disks d ON d.id = mf.disk_id
		WHERE mf.file_kind = 'shorts'
		  AND mf.work_id IS NULL
		  AND mf.presence_status = 'present'
		  AND d.connection_state = 'online'
	`)
	if err == nil {
		type unlinkedShorts struct {
			id, diskID, rel, fname string
		}
		var uShorts []unlinkedShorts
		for shRows.Next() {
			var us unlinkedShorts
			if err := shRows.Scan(&us.id, &us.diskID, &us.rel, &us.fname); err == nil {
				uShorts = append(uShorts, us)
			}
		}
		shRows.Close()

		for _, us := range uShorts {
			checksJSON, _ := json.Marshal([]string{"protected_shorts_preserved", "forbidden_from_errors_or_samples", "manual_review_required"})
			_, insErr := s.sqlDB.ExecContext(ctx, `
				INSERT INTO review_items (
					id, batch_id, file_id, work_id, item_file_kind, action_kind, error_category_code,
					source_disk_id, old_rel_path, old_name, target_disk_id, proposed_rel_path, proposed_name,
					reason_description, decision_status, confidence_score, conflict_detected, conflict_details,
					safety_checks_json, created_at
				) VALUES (?, ?, ?, NULL, 'file', 'classify_program', '', ?, ?, ?, ?, ?, ?, ?, 'deferred', 0.50, 1, ?, ?, ?)
			`, domain.NewImmutableID(), batchID, us.id, us.diskID, us.rel, us.fname, us.diskID, us.rel, us.fname,
				"ملف .shorts محمي غير مرتبط بعمل مؤكد — يُمنع نقله إلى العينات أو الأخطاء ويُحتفظ به للمراجعة اليدوية",
				"Protected .shorts file without confirmed movie match; kept untouched for manual user assignment",
				string(checksJSON), nowStr)
			if insErr == nil {
				totalAdded++
			}
		}
	}

	// 4. ANOMALIES & MANAGED ERROR FOLDER (`الأخطاء/`) FILE ITEMS
	aRows, err := s.sqlDB.QueryContext(ctx, `
		SELECT mf.id, mf.disk_id, d.mount_path, COALESCE(lr.relative_root_path, ''),
		       mf.relative_path, mf.filename, mf.file_kind, mf.anomaly_category, mf.anomaly_reason
		FROM media_files mf
		JOIN disks d ON d.id = mf.disk_id
		LEFT JOIN library_roots lr ON lr.id = mf.root_id
		WHERE mf.presence_status = 'present'
		  AND d.connection_state = 'online'
		  AND mf.anomaly_category != ''
		  AND mf.file_kind != 'shorts'
		ORDER BY mf.relative_path ASC
	`)
	if err == nil {
		type anomRow struct {
			id, diskID, mountPath, rootRel, rel, fname, kind, cat, reason string
		}
		var anoms []anomRow
		for aRows.Next() {
			var ar anomRow
			if err := aRows.Scan(&ar.id, &ar.diskID, &ar.mountPath, &ar.rootRel, &ar.rel, &ar.fname, &ar.kind, &ar.cat, &ar.reason); err == nil {
				anoms = append(anoms, ar)
			}
		}
		aRows.Close()

		for _, ar := range anoms {
			if naming.IsShortsExtension(ar.fname) {
				continue
			}
			catCode := errorsbox.CategoryCode(ar.cat)
			spec, lErr := errorsbox.LookupCategory(catCode)
			if lErr != nil {
				continue
			}
			diskReserved := reserveForDisk(ar.diskID)
			targetErrRel, pErr := s.errBox.PlanCollisionFreeRelPath(ctx, ar.diskID, ar.mountPath, ar.rootRel, catCode, ar.rel, diskReserved)
			if pErr != nil {
				continue
			}
			targetErrName := filepath.Base(filepath.FromSlash(targetErrRel))

			requiresExplicitConflictReview := catCode == errorsbox.CategoryConflictsReview ||
				(catCode == errorsbox.CategoryUnlinkedSubtitles && strings.Contains(ar.reason, "multi-video"))
			confScore := 0.96
			conflictDetails := ""
			if requiresExplicitConflictReview {
				confScore = 0.55
				conflictDetails = ar.reason
			}
			reasonAr := fmt.Sprintf("نقل آمن وقابل للاسترجاع إلى %s/%s — %s", errorsbox.OfficialRootFolderName, spec.FolderAr, ar.reason)
			checksJSON, _ := json.Marshal([]string{
				"errorsbox_collision_free_path",
				"sha256_verified_quarantine",
				"one_click_restore_ready",
				"never_auto_deleted",
			})

			_, insErr := s.sqlDB.ExecContext(ctx, `
				INSERT INTO review_items (
					id, batch_id, file_id, work_id, item_file_kind, action_kind, error_category_code,
					source_disk_id, old_rel_path, old_name, target_disk_id, proposed_rel_path, proposed_name,
					reason_description, decision_status, confidence_score, conflict_detected, conflict_details,
					safety_checks_json, created_at
				) VALUES (?, ?, ?, NULL, 'file', 'move_to_errors', ?, ?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?, ?, ?, ?)
			`, domain.NewImmutableID(), batchID, ar.id, string(catCode), ar.diskID, ar.rel, ar.fname,
				ar.diskID, targetErrRel, targetErrName, reasonAr, confScore,
				boolInt(requiresExplicitConflictReview), conflictDetails, string(checksJSON), nowStr)
			if insErr == nil {
				totalAdded++
				markPlannedMoveOut(ar.diskID, ar.rel, targetErrRel)
			}
		}
	}

	// 5. EMPTY SOURCE FOLDERS DETECTION (Pre-existing & Evacuated After Moves)
	totalAdded += s.planEmptyAndEvacuatedDirectories(ctx, batchID, nowStr, reserveForDisk, plannedMoveOutByDisk)

	_, _ = s.sqlDB.ExecContext(ctx, `
		UPDATE review_batches SET total_items = ?, updated_at = ? WHERE id = ?
	`, totalAdded, nowStr, batchID)

	return s.GetBatch(ctx, batchID)
}

func (s *Service) planEmptyAndEvacuatedDirectories(
	ctx context.Context,
	batchID string,
	nowStr string,
	reserveForDisk func(string) map[string]bool,
	plannedMoveOutByDisk map[string]map[string]bool,
) int {
	added := 0
	rRows, err := s.sqlDB.QueryContext(ctx, `
		SELECT lr.id, lr.disk_id, d.mount_path, lr.relative_root_path
		FROM library_roots lr
		JOIN disks d ON d.id = lr.disk_id
		WHERE d.connection_state = 'online' AND lr.scan_enabled = 1
	`)
	if err != nil {
		return 0
	}
	type rootInfo struct {
		rootID, diskID, mountPath, rootRel string
	}
	var roots []rootInfo
	for rRows.Next() {
		var ri rootInfo
		if err := rRows.Scan(&ri.rootID, &ri.diskID, &ri.mountPath, &ri.rootRel); err == nil {
			roots = append(roots, ri)
		}
	}
	rRows.Close()

	for _, rt := range roots {
		rootFull := library.ResolveFullPath(rt.mountPath, rt.rootRel)
		st, sErr := os.Stat(rootFull)
		if sErr != nil || !st.IsDir() {
			continue
		}
		diskReserved := reserveForDisk(rt.diskID)
		moveOutMap := plannedMoveOutByDisk[rt.diskID]

		topEntries, rErr := os.ReadDir(rootFull)
		if rErr != nil {
			continue
		}

		var candidateDirs []string
		for _, te := range topEntries {
			if !te.IsDir() {
				continue
			}
			name := te.Name()
			if errorsbox.IsManagedErrorDirName(name) || scanner.IsSkippedThumbnailDir(name) {
				continue
			}
			lowerName := strings.ToLower(name)
			if lowerName == "$recycle.bin" || lowerName == "system volume information" || lowerName == ".estiraha_trash" {
				continue
			}
			candidateDirs = append(candidateDirs, filepath.Join(rootFull, name))
		}
		sort.Strings(candidateDirs)

		for _, dirFull := range candidateDirs {
			dirRel, relErr := library.RelativeFromMount(rt.mountPath, dirFull)
			if relErr != nil || dirRel == "" {
				continue
			}
			normDirRelLower := strings.ToLower(normalizeSlash(dirRel))

			isTargetDir := false
			for resPath := range diskReserved {
				if resPath == normDirRelLower || strings.HasPrefix(resPath, normDirRelLower+"/") {
					isTargetDir = true
					break
				}
			}
			if isTargetDir {
				continue
			}

			totalRealFiles := 0
			unmovedRealFiles := 0
			hasThumbDir := false

			_ = filepath.WalkDir(dirFull, func(p string, d fs.DirEntry, wErr error) error {
				if wErr != nil {
					unmovedRealFiles++
					return nil
				}
				if d.IsDir() {
					if p != dirFull && scanner.IsSkippedThumbnailDir(d.Name()) {
						hasThumbDir = true
						return filepath.SkipDir
					}
					return nil
				}
				totalRealFiles++
				fRel, fErr := library.RelativeFromMount(rt.mountPath, p)
				if fErr != nil {
					unmovedRealFiles++
					return nil
				}
				if moveOutMap == nil || !moveOutMap[strings.ToLower(normalizeSlash(fRel))] {
					unmovedRealFiles++
				}
				return nil
			})

			if unmovedRealFiles > 0 || hasThumbDir {
				continue
			}

			targetErrRel, pErr := s.errBox.PlanCollisionFreeRelPath(ctx, rt.diskID, rt.mountPath, rt.rootRel, errorsbox.CategoryEmptyFolders, dirRel, diskReserved)
			if pErr != nil {
				continue
			}
			targetErrName := filepath.Base(filepath.FromSlash(targetErrRel))
			dirBaseName := filepath.Base(dirFull)

			reasonAr := fmt.Sprintf("نقل المجلد الفارغ إلى %s/مجلدات فارغة بعد التحقق من خلوه التام من أي ملفات", errorsbox.OfficialRootFolderName)
			if totalRealFiles > 0 {
				reasonAr = fmt.Sprintf("نقل مجلد المصدر القديم (%s) إلى %s/مجلدات فارغة بعد إخلائه بالكامل إلى المجلد الرسمي النظيف", dirBaseName, errorsbox.OfficialRootFolderName)
			}
			checksJSON, _ := json.Marshal([]string{
				"verified_zero_remaining_files",
				"errorsbox_collision_free_path",
				"one_click_restore_ready",
			})

			_, insErr := s.sqlDB.ExecContext(ctx, `
				INSERT INTO review_items (
					id, batch_id, file_id, work_id, item_file_kind, action_kind, error_category_code,
					source_disk_id, old_rel_path, old_name, target_disk_id, proposed_rel_path, proposed_name,
					reason_description, decision_status, confidence_score, conflict_detected, conflict_details,
					safety_checks_json, created_at
				) VALUES (?, ?, NULL, NULL, 'directory', 'move_to_errors', ?, ?, ?, ?, ?, ?, ?, ?, 'pending', 0.98, 0, '', ?, ?)
			`, domain.NewImmutableID(), batchID, string(errorsbox.CategoryEmptyFolders),
				rt.diskID, dirRel, dirBaseName, rt.diskID, targetErrRel, targetErrName,
				reasonAr, string(checksJSON), nowStr)
			if insErr == nil {
				added++
			}
		}
	}
	return added
}

func (s *Service) DecideItem(ctx context.Context, in ItemDecisionInput) (ReviewItemView, error) {
	switch in.Decision {
	case "approved", "rejected", "deferred", "excluded", "pending":
	default:
		return ReviewItemView{}, fmt.Errorf("review: unsupported decision %q", in.Decision)
	}

	var oldName, oldRel, proposedRel, proposedName, curEditedRel, curEditedName, diskID, mountPath, fileID, actionKind, errorCat string
	var conflictInt int
	err := s.sqlDB.QueryRowContext(ctx, `
		SELECT ri.old_name, ri.old_rel_path, ri.proposed_rel_path, ri.proposed_name,
		       COALESCE(ri.user_edited_rel_path, ''), COALESCE(ri.user_edited_name, ''),
		       ri.target_disk_id, d.mount_path,
		       COALESCE(ri.file_id, ''), ri.action_kind, ri.error_category_code, ri.conflict_detected
		FROM review_items ri
		JOIN disks d ON d.id = ri.target_disk_id
		WHERE ri.id = ?
	`, in.ItemID).Scan(&oldName, &oldRel, &proposedRel, &proposedName, &curEditedRel, &curEditedName, &diskID, &mountPath, &fileID, &actionKind, &errorCat, &conflictInt)
	if err != nil {
		return ReviewItemView{}, err
	}

	if in.Decision == "approved" && naming.IsShortsExtension(oldName) && actionKind == "move_to_errors" {
		return ReviewItemView{}, errors.New("safety law: .shorts files are protected and cannot be moved to error folders")
	}

	var editedRel, editedName sql.NullString
	effectiveRel := proposedRel
	if curEditedRel != "" {
		effectiveRel = curEditedRel
	}

	if strings.TrimSpace(in.UserEditedName) != "" || strings.TrimSpace(in.UserEditedRelPath) != "" || strings.TrimSpace(in.UserEditedDirPath) != "" {
		baseEffective := effectiveRel
		curBaseName := filepath.Base(filepath.FromSlash(baseEffective))
		curDirRel := filepath.ToSlash(filepath.Dir(filepath.FromSlash(baseEffective)))
		if curDirRel == "." {
			curDirRel = ""
		}

		newName := strings.TrimSpace(in.UserEditedName)
		if newName == "" && strings.TrimSpace(in.UserEditedRelPath) != "" {
			newName = filepath.Base(filepath.FromSlash(in.UserEditedRelPath))
		}
		if newName == "" {
			newName = curBaseName
		}
		if naming.IsShortsExtension(oldName) && !naming.IsShortsExtension(newName) {
			return ReviewItemView{}, errors.New("safety law: .shorts extension is protected and cannot be removed or changed")
		}

		var newRel string
		if strings.TrimSpace(in.UserEditedRelPath) != "" {
			newRel = normalizeSlash(in.UserEditedRelPath)
		} else if strings.TrimSpace(in.UserEditedDirPath) != "" {
			cleanDir := normalizeSlash(in.UserEditedDirPath)
			if cleanDir == "" || cleanDir == "." {
				newRel = newName
			} else {
				newRel = cleanDir + "/" + newName
			}
		} else {
			if curDirRel == "" {
				newRel = newName
			} else {
				newRel = curDirRel + "/" + newName
			}
		}

		if strings.Contains(newRel, "..") {
			return ReviewItemView{}, errors.New("safety law: relative destination cannot contain '..'")
		}

		editedName = sql.NullString{String: filepath.Base(filepath.FromSlash(newRel)), Valid: true}
		editedRel = sql.NullString{String: filepath.ToSlash(newRel), Valid: true}
		effectiveRel = editedRel.String

		oldDir := normalizeSlash(filepath.ToSlash(filepath.Dir(filepath.FromSlash(oldRel))))
		if oldDir == "." {
			oldDir = ""
		}
		newDir := normalizeSlash(filepath.ToSlash(filepath.Dir(filepath.FromSlash(effectiveRel))))
		if newDir == "." {
			newDir = ""
		}
		if actionKind == "move" || actionKind == "rename" {
			if oldDir == newDir {
				actionKind = "rename"
			} else {
				actionKind = "move"
			}
		}

		conflict, msg := s.detectDestinationConflict(ctx, diskID, mountPath, effectiveRel, fileID, nil)
		conflictInt = boolInt(conflict)
		_, _ = s.sqlDB.ExecContext(ctx, `UPDATE review_items SET action_kind = ?, conflict_detected = ?, conflict_details = ? WHERE id = ?`, actionKind, conflictInt, msg, in.ItemID)
	}

	if in.Decision == "approved" && conflictInt == 1 {
		if actionKind == "move_to_errors" && errorCat != "" {
			destConflict, _ := s.detectDestinationConflict(ctx, diskID, mountPath, effectiveRel, fileID, nil)
			if destConflict {
				return ReviewItemView{}, ErrTargetCollisionAbort
			}
		} else {
			return ReviewItemView{}, ErrTargetCollisionAbort
		}
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

func (s *Service) BulkDecideItems(ctx context.Context, in BulkDecisionInput) (BulkDecisionResult, error) {
	res := BulkDecisionResult{
		BatchID:  in.BatchID,
		Warnings: []string{},
	}
	switch in.Decision {
	case "approved", "rejected", "deferred", "pending":
	default:
		return res, fmt.Errorf("review: unsupported bulk decision %q", in.Decision)
	}

	for _, rawID := range in.ItemIDs {
		id := strings.TrimSpace(rawID)
		if id == "" {
			continue
		}
		var curStatus string
		if err := s.sqlDB.QueryRowContext(ctx, `SELECT decision_status FROM review_items WHERE id = ?`, id).Scan(&curStatus); err != nil {
			res.SkippedCount++
			continue
		}
		if curStatus == "executed" {
			res.SkippedCount++
			continue
		}
		if _, dErr := s.DecideItem(ctx, ItemDecisionInput{
			ItemID:   id,
			Decision: in.Decision,
		}); dErr != nil {
			res.SkippedCount++
			res.Warnings = append(res.Warnings, fmt.Sprintf("%s: %v", id, dErr))
			continue
		}
		res.UpdatedCount++
	}
	if in.BatchID != "" {
		s.recomputeBatchStatus(ctx, in.BatchID)
	}
	return res, nil
}

func (s *Service) GetItemDetails(ctx context.Context, itemID string) (ReviewItemDetailView, error) {
	var batchID string
	if err := s.sqlDB.QueryRowContext(ctx, `SELECT batch_id FROM review_items WHERE id = ?`, strings.TrimSpace(itemID)).Scan(&batchID); err != nil {
		return ReviewItemDetailView{}, err
	}
	items, err := s.ListBatchItems(ctx, batchID)
	if err != nil {
		return ReviewItemDetailView{}, err
	}
	var found *ReviewItemView
	for i := range items {
		if items[i].ID == strings.TrimSpace(itemID) {
			found = &items[i]
			break
		}
	}
	if found == nil {
		return ReviewItemDetailView{}, sql.ErrNoRows
	}

	detail := ReviewItemDetailView{
		Item: *found,
	}
	if found.FileID != "" {
		_ = s.sqlDB.QueryRowContext(ctx, `
			SELECT extension, COALESCE(sha256_hash, ''), duration_seconds, video_width, video_height,
			       probe_status, integrity_status, presence_status
			FROM media_files
			WHERE id = ?
		`, found.FileID).Scan(
			&detail.Extension, &detail.FileSHA256, &detail.DurationSeconds,
			&detail.VideoWidth, &detail.VideoHeight, &detail.ProbeStatus,
			&detail.IntegrityStatus, &detail.PresenceStatus,
		)
	}
	if found.WorkID != "" {
		_ = s.sqlDB.QueryRowContext(ctx, `
			SELECT COALESCE(production_year, 0), identification_status
			FROM works WHERE id = ?
		`, found.WorkID).Scan(&detail.ProductionYear, &detail.WorkStatus)
	}
	_ = s.sqlDB.QueryRowContext(ctx, `
		SELECT COALESCE(error_message, '')
		FROM operation_journal
		WHERE review_item_id = ?
		ORDER BY step_sequence DESC LIMIT 1
	`, found.ID).Scan(&detail.LastJournalMsg)

	return detail, nil
}

func (s *Service) BuildExecutionManifestSummary(ctx context.Context, batchID string) (ExecutionManifestSummary, error) {
	batch, err := s.GetBatch(ctx, batchID)
	if err != nil {
		return ExecutionManifestSummary{}, err
	}

	out := ExecutionManifestSummary{
		BatchID:       batch.ID,
		BatchTitle:    batch.Title,
		BatchStatus:   batch.BatchStatus,
		TargetFolders: []string{},
		Steps:         []ManifestStepSpec{},
	}

	rows, err := s.sqlDB.QueryContext(ctx, `
		SELECT ri.id, COALESCE(ri.file_id, ''),
		       COALESCE(mf.root_id, ''), COALESCE(lr.relative_root_path, ''),
		       ri.item_file_kind, COALESCE(mf.file_kind, CASE WHEN ri.item_file_kind = 'directory' THEN 'directory' ELSE 'unknown_extra' END),
		       ri.action_kind, ri.error_category_code, ri.reason_description,
		       ri.source_disk_id, d.label, d.mount_path,
		       ri.old_rel_path, ri.old_name,
		       COALESCE(NULLIF(ri.user_edited_rel_path, ''), ri.proposed_rel_path),
		       COALESCE(NULLIF(ri.user_edited_name, ''), ri.proposed_name),
		       COALESCE(ri.decided_at, '')
		FROM review_items ri
		JOIN disks d ON d.id = ri.source_disk_id
		LEFT JOIN media_files mf ON mf.id = ri.file_id
		LEFT JOIN library_roots lr ON lr.id = mf.root_id
		WHERE ri.batch_id = ? AND ri.decision_status = 'approved'
		ORDER BY CASE ri.item_file_kind WHEN 'file' THEN 0 ELSE 1 END ASC, ri.created_at ASC, ri.id ASC
	`, batchID)
	if err != nil {
		return out, err
	}
	defer rows.Close()

	newDirsSet := map[string]string{}
	approvedFileIDs := map[string]bool{}
	seq := 0

	for rows.Next() {
		seq++
		var st ManifestStepSpec
		if err := rows.Scan(
			&st.ReviewItemID, &st.FileID,
			&st.RootID, &st.RootRelPath,
			&st.ItemFileKind, &st.MediaFileKind,
			&st.ActionKind, &st.ErrorCategoryCode, &st.ReasonDescription,
			&st.DiskID, &st.DiskLabel, &st.MountPath,
			&st.OldRelPath, &st.OldName,
			&st.EffectiveRelPath, &st.EffectiveName,
			&st.ApprovedAt,
		); err != nil {
			return out, err
		}
		st.Sequence = seq
		st.EffectiveName = filepath.Base(filepath.FromSlash(st.EffectiveRelPath))
		st.ProtectedShortsFile = naming.IsShortsExtension(st.OldName) || st.MediaFileKind == "shorts"
		out.Steps = append(out.Steps, st)

		if st.FileID != "" {
			approvedFileIDs[st.FileID] = true
		}

		oldDir := normalizeSlash(filepath.ToSlash(filepath.Dir(filepath.FromSlash(st.OldRelPath))))
		newDir := normalizeSlash(filepath.ToSlash(filepath.Dir(filepath.FromSlash(st.EffectiveRelPath))))
		if oldDir == "." {
			oldDir = ""
		}
		if newDir == "." {
			newDir = ""
		}

		if st.ItemFileKind == "directory" {
			if st.ActionKind == "move_to_errors" && st.ErrorCategoryCode == string(errorsbox.CategoryEmptyFolders) {
				out.EmptyFoldersToMoveErrors++
			}
		} else {
			if st.ActionKind == "rename" || (st.ActionKind != "move_to_errors" && oldDir == newDir && st.OldName != st.EffectiveName) {
				out.FilesToRename++
			} else if st.ActionKind == "move" || st.ActionKind == "move_to_errors" || oldDir != newDir {
				out.FilesToMove++
			}
			if st.MediaFileKind == "subtitle" && st.ActionKind != "move_to_errors" {
				out.SubtitlesToAssociate++
			}
			if st.MediaFileKind == "image" && st.ActionKind != "move_to_errors" {
				out.PostersArtworkToAssociate++
			}
		}

		if newDir != "" && st.MountPath != "" {
			fullTargetDir := library.ResolveFullPath(st.MountPath, newDir)
			if _, statErr := os.Stat(fullTargetDir); os.IsNotExist(statErr) {
				key := st.DiskID + "|" + strings.ToLower(newDir)
				newDirsSet[key] = newDir
			}
		}
	}
	if err := rows.Err(); err != nil {
		return out, err
	}

	out.TotalApprovedOperations = len(out.Steps)
	for _, dirRel := range newDirsSet {
		out.TargetFolders = append(out.TargetFolders, dirRel)
	}
	sort.Strings(out.TargetFolders)
	out.FoldersToCreate = len(out.TargetFolders)

	_ = s.sqlDB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM media_files mf
		JOIN disks d ON d.id = mf.disk_id
		WHERE mf.file_kind = 'shorts' AND mf.presence_status = 'present' AND d.connection_state = 'online'
	`).Scan(&out.ShortsToPreserve)

	_ = s.sqlDB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM review_items
		WHERE batch_id = ? AND conflict_detected = 1 AND decision_status NOT IN ('approved', 'rejected', 'executed', 'rolled_back')
	`, batchID).Scan(&out.UnresolvedConflicts)

	var totalPresentFiles int
	_ = s.sqlDB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM media_files mf
		JOIN disks d ON d.id = mf.disk_id
		WHERE mf.presence_status = 'present' AND d.connection_state = 'online'
	`).Scan(&totalPresentFiles)
	unchanged := totalPresentFiles - len(approvedFileIDs)
	if unchanged < 0 {
		unchanged = 0
	}
	out.UnchangedFilesCount = unchanged

	canonicalBytes, _ := json.Marshal(out.Steps)
	sum := sha256.Sum256(canonicalBytes)
	out.ManifestSHA256 = hex.EncodeToString(sum[:])

	return out, nil
}

func (s *Service) ApproveAllSafeItems(ctx context.Context, batchID string) (ReviewBatchView, error) {
	nowStr := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.sqlDB.ExecContext(ctx, `
		UPDATE review_items
		SET decision_status = 'approved', decided_at = ?
		WHERE batch_id = ? AND decision_status = 'pending' AND conflict_detected = 0
		  AND action_kind IN ('move', 'rename', 'move_to_errors')
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
		       (SELECT COUNT(*) FROM review_items ri WHERE ri.batch_id = b.id AND ri.decision_status = 'rejected'),
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
			&v.ApprovedByAt, &v.CreatedAt, &v.PendingItems, &v.RejectedItems, &v.ExecutedItems,
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
		       COALESCE(w.canonical_folder_name, COALESCE(w.title_original, '')), COALESCE(w.media_type, ''),
		       ri.item_file_kind,
		       COALESCE(mf.file_kind, CASE WHEN ri.item_file_kind = 'directory' THEN 'directory' ELSE 'unknown_extra' END),
		       ri.action_kind, ri.error_category_code,
		       ri.source_disk_id, d.label, d.mount_path,
		       ri.old_rel_path, ri.old_name, ri.target_disk_id,
		       ri.proposed_rel_path, ri.proposed_name,
		       COALESCE(ri.user_edited_rel_path, ''), COALESCE(ri.user_edited_name, ''),
		       ri.reason_description, ri.decision_status, ri.confidence_score,
		       ri.conflict_detected, ri.conflict_details, ri.linked_files_json, ri.safety_checks_json,
		       COALESCE(mf.size_bytes, 0), COALESCE(mv.resolution_label, ''), COALESCE(mv.video_codec, COALESCE(mf.video_codec, '')),
		       COALESCE((SELECT oj.operation_state FROM operation_journal oj WHERE oj.review_item_id = ri.id ORDER BY oj.step_sequence DESC LIMIT 1), '')
		FROM review_items ri
		JOIN disks d ON d.id = ri.source_disk_id
		LEFT JOIN media_files mf ON mf.id = ri.file_id
		LEFT JOIN works w ON w.id = ri.work_id
		LEFT JOIN media_versions mv ON mv.file_id = ri.file_id
		WHERE ri.batch_id = ?
		ORDER BY CASE ri.item_file_kind WHEN 'file' THEN 0 ELSE 1 END ASC, ri.old_rel_path ASC
	`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []ReviewItemView{}
	for rows.Next() {
		var it ReviewItemView
		var mountPath, linkedJSON, checksJSON string
		var confInt int
		if err := rows.Scan(
			&it.ID, &it.BatchID, &it.FileID, &it.WorkID,
			&it.WorkTitle, &it.WorkMediaType,
			&it.ItemFileKind, &it.MediaFileKind,
			&it.ActionKind, &it.ErrorCategoryCode,
			&it.SourceDiskID, &it.DiskLabel, &mountPath,
			&it.OldRelPath, &it.OldName, &it.TargetDiskID,
			&it.ProposedRelPath, &it.ProposedName,
			&it.UserEditedRelPath, &it.UserEditedName,
			&it.ReasonDescription, &it.DecisionStatus, &it.ConfidenceScore,
			&confInt, &it.ConflictDetails, &linkedJSON, &checksJSON,
			&it.FileSizeBytes, &it.ResolutionLabel, &it.VideoCodec,
			&it.OperationState,
		); err != nil {
			return nil, err
		}
		if it.ActionKind == "classify_program" && naming.IsShortsExtension(it.OldName) {
			it.ActionKind = "needs_manual_review"
		}
		it.ConflictDetected = confInt == 1
		oldDir := filepath.ToSlash(filepath.Dir(filepath.FromSlash(it.OldRelPath)))
		if oldDir == "." {
			oldDir = "/"
		}
		it.OldDirPath = oldDir
		it.OldFullPath = library.ResolveFullPath(mountPath, it.OldRelPath)

		propDir := filepath.ToSlash(filepath.Dir(filepath.FromSlash(it.ProposedRelPath)))
		if propDir == "." {
			propDir = "/"
		}
		it.ProposedDirPath = propDir

		it.EffectiveRelPath = it.ProposedRelPath
		if it.UserEditedRelPath != "" {
			it.EffectiveRelPath = it.UserEditedRelPath
		}
		effDir := filepath.ToSlash(filepath.Dir(filepath.FromSlash(it.EffectiveRelPath)))
		if effDir == "." {
			effDir = "/"
		}
		it.EffectiveDirPath = effDir
		it.EffectiveName = filepath.Base(filepath.FromSlash(it.EffectiveRelPath))
		it.EffectiveFullPath = library.ResolveFullPath(mountPath, it.EffectiveRelPath)
		it.ProtectedShortsFile = naming.IsShortsExtension(it.OldName) || it.MediaFileKind == "shorts"
		_ = json.Unmarshal([]byte(linkedJSON), &it.LinkedCompanions)
		if it.LinkedCompanions == nil {
			it.LinkedCompanions = []string{}
		}
		_ = json.Unmarshal([]byte(checksJSON), &it.SafetyChecks)
		if it.SafetyChecks == nil {
			it.SafetyChecks = []string{}
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

type workFileRow struct {
	fileID, diskID, mountPath, rootRel, relPath, filename, ext, kind, keepDecision string
	anomalyCategory, resLabel, vCodec, subLang                                     string
}

func (s *Service) loadPresentWorkFiles(ctx context.Context, workID, epID string) []workFileRow {
	q := `
		SELECT mf.id, mf.disk_id, d.mount_path, COALESCE(lr.relative_root_path, ''),
		       mf.relative_path, mf.filename, mf.extension, mf.file_kind, mf.keep_decision,
		       mf.anomaly_category,
		       COALESCE(mv.resolution_label, ''), COALESCE(mv.video_codec, mf.video_codec, ''),
		       COALESCE((SELECT st.language_code FROM subtitle_tracks st WHERE st.file_id = mf.id LIMIT 1), '')
		FROM media_files mf
		JOIN disks d ON d.id = mf.disk_id
		LEFT JOIN library_roots lr ON lr.id = mf.root_id
		LEFT JOIN media_versions mv ON mv.file_id = mf.id
		WHERE mf.work_id = ? AND mf.presence_status = 'present' AND d.connection_state = 'online'
		  AND mf.is_apple_double = 0 AND mf.is_system_artifact = 0 AND mf.is_sample_clip = 0
	`
	args := []any{workID}
	if epID != "" {
		q += ` AND mf.episode_id = ?`
		args = append(args, epID)
	}
	q += ` ORDER BY CASE mf.file_kind WHEN 'video' THEN 1 WHEN 'subtitle' THEN 2 WHEN 'shorts' THEN 3 WHEN 'image' THEN 4 ELSE 5 END, COALESCE(mv.computed_quality_score, 0) DESC, mf.size_bytes DESC, mf.relative_path ASC`

	rows, err := s.sqlDB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []workFileRow
	for rows.Next() {
		var r workFileRow
		if err := rows.Scan(
			&r.fileID, &r.diskID, &r.mountPath, &r.rootRel,
			&r.relPath, &r.filename, &r.ext, &r.kind, &r.keepDecision,
			&r.anomalyCategory, &r.resLabel, &r.vCodec, &r.subLang,
		); err == nil {
			if r.subLang == "und" {
				r.subLang = ""
			}
			out = append(out, r)
		}
	}
	return out
}

func (s *Service) detectDestinationConflict(ctx context.Context, diskID, mountPath, targetRel, selfFileID string, reservedInBatch map[string]bool) (bool, string) {
	normKey := strings.ToLower(normalizeSlash(targetRel))
	if reservedInBatch != nil && reservedInBatch[normKey] {
		return true, "Destination path collides with another item in the same batch"
	}
	var otherID string
	err := s.sqlDB.QueryRowContext(ctx, `
		SELECT id FROM media_files WHERE disk_id = ? AND lower(relative_path) = lower(?) AND id != ? AND presence_status = 'present'
	`, diskID, targetRel, selfFileID).Scan(&otherID)
	if err == nil && otherID != "" {
		return true, "Target path already indexed by another file in SQLite"
	}
	if mountPath != "" {
		full := library.ResolveFullPath(mountPath, targetRel)
		if _, stErr := os.Stat(full); stErr == nil {
			if selfFileID != "" {
				var selfRel string
				_ = s.sqlDB.QueryRowContext(ctx, `SELECT relative_path FROM media_files WHERE id = ?`, selfFileID).Scan(&selfRel)
				if strings.EqualFold(normalizeSlash(selfRel), normalizeSlash(targetRel)) {
					return false, ""
				}
			}
			return true, "Target path already exists on physical disk"
		}
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
