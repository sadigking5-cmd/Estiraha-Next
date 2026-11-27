package organizer

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sadigking5-cmd/Estiraha-Next/internal/backup"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/domain"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/errorsbox"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/fsops"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/library"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/naming"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/review"
	"strings"
)

var (
	ErrNoApprovedItems       = errors.New("organizer: batch has zero user-approved items to execute")
	ErrOrganizerBusy         = errors.New("organizer: another file organization or rollback operation is currently running")
	ErrHashMismatchAfterMove = errors.New("organizer critical safety halt: post-move SHA-256 did not match pre-move SHA-256")
)

type ExecutionResult struct {
	BatchID          string   `json:"batchId"`
	PreBatchBackupID string   `json:"preBatchBackupId"`
	ExecutedCount    int      `json:"executedCount"`
	SkippedCount     int      `json:"skippedCount"`
	AbortedCount     int      `json:"abortedCount"`
	Errors           []string `json:"errors"`
	CompletedAt      string   `json:"completedAt"`
}

type RollbackResult struct {
	BatchID       string   `json:"batchId"`
	RestoredCount int      `json:"restoredCount"`
	BlockedCount  int      `json:"blockedCount"`
	Errors        []string `json:"errors"`
	CompletedAt   string   `json:"completedAt"`
}

type JournalEntryView struct {
	ID              string `json:"id"`
	BatchID         string `json:"batchId"`
	StepSequence    int    `json:"stepSequence"`
	OperationState  string `json:"operationState"`
	SourcePath      string `json:"sourcePath"`
	DestinationPath string `json:"destinationPath"`
	PreSHA256       string `json:"preSha256"`
	PostSHA256      string `json:"postSha256"`
	ErrorMessage    string `json:"errorMessage"`
	CompletedAt     string `json:"completedAt"`
}

type Service struct {
	sqlDB     *sql.DB
	backupSvc *backup.Service
	errBoxSvc *errorsbox.Service
	mu        sync.Mutex
}

func NewService(sqlDB *sql.DB, backupSvc *backup.Service) *Service {
	return &Service{
		sqlDB:     sqlDB,
		backupSvc: backupSvc,
		errBoxSvc: errorsbox.NewService(sqlDB),
	}
}

type approvedStep struct {
	itemID       string
	fileID       string
	rootID       string
	rootRel      string
	itemFileKind string
	actionKind   string
	errorCatCode string
	reasonDesc   string
	diskID       string
	mountPath    string
	oldRel       string
	oldName      string
	effectiveRel string
	decidedAt    string
}

func (s *Service) ExecuteApprovedBatch(ctx context.Context, batchID string) (ExecutionResult, error) {
	if !s.mu.TryLock() {
		return ExecutionResult{}, ErrOrganizerBusy
	}
	defer s.mu.Unlock()

	res := ExecutionResult{
		BatchID: batchID,
		Errors:  []string{},
	}

	rows, err := s.sqlDB.QueryContext(ctx, `
		SELECT ri.id, COALESCE(ri.file_id, ''),
		       COALESCE(mf.root_id, ''), COALESCE(lr.relative_root_path, ''),
		       ri.item_file_kind, ri.action_kind, ri.error_category_code, ri.reason_description,
		       ri.source_disk_id, d.mount_path,
		       ri.old_rel_path, ri.old_name,
		       COALESCE(NULLIF(ri.user_edited_rel_path, ''), ri.proposed_rel_path),
		       COALESCE(ri.decided_at, '')
		FROM review_items ri
		JOIN disks d ON d.id = ri.source_disk_id
		LEFT JOIN media_files mf ON mf.id = ri.file_id
		LEFT JOIN library_roots lr ON lr.id = mf.root_id
		WHERE ri.batch_id = ? AND ri.decision_status = 'approved'
		ORDER BY CASE ri.item_file_kind WHEN 'file' THEN 0 ELSE 1 END ASC, ri.created_at ASC
	`, batchID)
	if err != nil {
		return res, err
	}
	var steps []approvedStep
	for rows.Next() {
		var st approvedStep
		if err := rows.Scan(
			&st.itemID, &st.fileID, &st.rootID, &st.rootRel,
			&st.itemFileKind, &st.actionKind, &st.errorCatCode, &st.reasonDesc,
			&st.diskID, &st.mountPath, &st.oldRel, &st.oldName, &st.effectiveRel, &st.decidedAt,
		); err == nil {
			steps = append(steps, st)
		}
	}
	rows.Close()

	if len(steps) == 0 {
		return res, ErrNoApprovedItems
	}

	scheduledEmptyDirs := map[string]bool{}
	for _, st := range steps {
		if st.itemFileKind == "directory" {
			fullDir := filepath.Clean(library.ResolveFullPath(st.mountPath, st.oldRel))
			scheduledEmptyDirs[fullDir] = true
		}
	}

	if s.backupSvc != nil && s.backupSvc.BackupDir() != "" {
		bk, bkErr := s.backupSvc.CreateVerifiedBackup(ctx, "pre_batch")
		if bkErr != nil {
			return res, fmt.Errorf("organizer safety gate: pre-batch verified backup failed: %w", bkErr)
		}
		res.PreBatchBackupID = bk.ID
	}

	nowStr := time.Now().UTC().Format(time.RFC3339Nano)
	_, _ = s.sqlDB.ExecContext(ctx, `UPDATE review_batches SET batch_status = 'executing', updated_at = ? WHERE id = ?`, nowStr, batchID)

	var maxSeq int
	_ = s.sqlDB.QueryRowContext(ctx, `SELECT COALESCE(MAX(step_sequence), 0) FROM operation_journal WHERE batch_id = ?`, batchID).Scan(&maxSeq)

	for idx, st := range steps {
		seq := maxSeq + idx + 1
		approvedTime, tErr := time.Parse(time.RFC3339Nano, st.decidedAt)
		if tErr != nil || approvedTime.IsZero() {
			approvedTime = time.Now().UTC()
		}
		permit := review.ExecutionPermit{
			BatchID:      batchID,
			ReviewItemID: st.itemID,
			Decision:     review.DecisionApproved,
			ApprovedAt:   approvedTime,
		}

		if st.actionKind == "move_to_errors" {
			if naming.IsShortsExtension(st.oldName) {
				res.AbortedCount++
				res.Errors = append(res.Errors, fmt.Sprintf("Safety law blocked moving .shorts to الأخطاء: %s", st.oldName))
				continue
			}

			rootID, rootRel := st.rootID, st.rootRel
			if rootRel == "" && st.rootID == "" {
				rootID, rootRel = s.resolveEnclosingRoot(ctx, st.diskID, st.oldRel)
			}
			rootFull := library.ResolveFullPath(st.mountPath, rootRel)
			if _, _, ensErr := s.errBoxSvc.EnsureErrorStructure(rootFull); ensErr != nil {
				res.AbortedCount++
				res.Errors = append(res.Errors, ensErr.Error())
				continue
			}

			srcFull := library.ResolveFullPath(st.mountPath, st.oldRel)
			if st.itemFileKind == "directory" {
				pruneEmptyChildSubdirs(srcFull)
				entries, rdErr := os.ReadDir(srcFull)
				if rdErr != nil || len(entries) > 0 {
					res.SkippedCount++
					_, _ = s.sqlDB.ExecContext(ctx, `
						UPDATE review_items SET decision_status = 'deferred', conflict_details = 'Skipped empty-folder move because unmoved files remain inside folder'
						WHERE id = ?
					`, st.itemID)
					continue
				}
			}

			journalID := domain.NewImmutableID()
			stepStart := time.Now().UTC().Format(time.RFC3339Nano)
			dstPreviewFull := library.ResolveFullPath(st.mountPath, st.effectiveRel)

			_, _ = s.sqlDB.ExecContext(ctx, `
				INSERT INTO operation_journal (
					id, batch_id, review_item_id, step_sequence, operation_state,
					source_snapshot_path, destination_snapshot_path, last_successful_substep, started_at
				) VALUES (?, ?, ?, ?, 'in_progress', ?, ?, 'errorsbox_init', ?)
			`, journalID, batchID, st.itemID, seq, srcFull, dstPreviewFull, stepStart)

			ev, mvErr := s.errBoxSvc.MoveApprovedItem(ctx, errorsbox.MoveToErrorsRequest{
				Permit:          permit,
				DiskID:          st.diskID,
				RootID:          rootID,
				MountPath:       st.mountPath,
				RootRelPath:     rootRel,
				FileID:          st.fileID,
				JournalID:       journalID,
				EntryKind:       st.itemFileKind,
				Category:        errorsbox.CategoryCode(st.errorCatCode),
				OriginalRelPath: st.oldRel,
				ReasonAr:        st.reasonDesc,
				ReasonEn:        st.reasonDesc,
			})
			if mvErr != nil {
				_, _ = s.sqlDB.ExecContext(ctx, `
					UPDATE operation_journal SET operation_state = 'aborted_target_exists', error_message = ?, completed_at = ?
					WHERE id = ?
				`, mvErr.Error(), stepStart, journalID)
				_, _ = s.sqlDB.ExecContext(ctx, `UPDATE review_items SET decision_status = 'failed', conflict_details = ? WHERE id = ?`, mvErr.Error(), st.itemID)
				res.AbortedCount++
				res.Errors = append(res.Errors, fmt.Sprintf("%s -> %v", st.oldName, mvErr))
				continue
			}

			actualDstFull := library.ResolveFullPath(st.mountPath, ev.ErrorRelPath)
			doneStr := time.Now().UTC().Format(time.RFC3339Nano)
			var hashNullable sql.NullString
			if ev.SHA256Hash != "" {
				hashNullable = sql.NullString{String: ev.SHA256Hash, Valid: true}
			}

			_, _ = s.sqlDB.ExecContext(ctx, `
				UPDATE operation_journal
				SET operation_state = 'completed', destination_snapshot_path = ?, pre_sha256 = ?, post_sha256 = ?,
				    last_successful_substep = 'errorsbox_verified', completed_at = ?
				WHERE id = ?
			`, actualDstFull, hashNullable, hashNullable, doneStr, journalID)

			_, _ = s.sqlDB.ExecContext(ctx, `
				INSERT INTO rollback_steps (
					id, journal_id, batch_id, reverse_order, reverse_action,
					from_disk_id, from_rel_path, to_disk_id, to_rel_path, expected_sha256, error_entry_id, rollback_status
				) VALUES (?, ?, ?, ?, 'move_back', ?, ?, ?, ?, ?, ?, 'ready')
			`, domain.NewImmutableID(), journalID, batchID, seq, st.diskID, ev.ErrorRelPath, st.diskID, st.oldRel, hashNullable, ev.ID)

			_, _ = s.sqlDB.ExecContext(ctx, `UPDATE review_items SET decision_status = 'executed' WHERE id = ?`, st.itemID)
			res.ExecutedCount++

			if st.itemFileKind == "file" {
				cleanEmptyParentChain(filepath.Dir(srcFull), rootFull, scheduledEmptyDirs)
			}
			continue
		}

		newBase := filepath.Base(filepath.FromSlash(st.effectiveRel))
		if naming.IsShortsExtension(st.oldName) && !naming.IsShortsExtension(newBase) {
			res.AbortedCount++
			res.Errors = append(res.Errors, fmt.Sprintf("Blocked .shorts alteration on %s", st.oldName))
			continue
		}

		srcFull := library.ResolveFullPath(st.mountPath, st.oldRel)
		dstFull := library.ResolveFullPath(st.mountPath, st.effectiveRel)

		journalID := domain.NewImmutableID()
		stepStart := time.Now().UTC().Format(time.RFC3339Nano)

		if err := fsops.VerifySafeDestination(permit, srcFull, dstFull); err != nil {
			state := "aborted_target_exists"
			_, _ = s.sqlDB.ExecContext(ctx, `
				INSERT INTO operation_journal (
					id, batch_id, review_item_id, step_sequence, operation_state,
					source_snapshot_path, destination_snapshot_path, last_successful_substep, error_message, started_at, completed_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, 'precheck', ?, ?, ?)
			`, journalID, batchID, st.itemID, seq, state, srcFull, dstFull, err.Error(), stepStart, stepStart)
			_, _ = s.sqlDB.ExecContext(ctx, `UPDATE review_items SET decision_status = 'failed', conflict_detected = 1, conflict_details = ? WHERE id = ?`, err.Error(), st.itemID)
			res.AbortedCount++
			res.Errors = append(res.Errors, fmt.Sprintf("%s -> %v", st.oldName, err))
			continue
		}

		preHash, hErr := hashFile(srcFull)
		if hErr != nil {
			res.AbortedCount++
			res.Errors = append(res.Errors, fmt.Sprintf("pre-hash failed %s: %v", st.oldName, hErr))
			continue
		}

		_, _ = s.sqlDB.ExecContext(ctx, `
			INSERT INTO operation_journal (
				id, batch_id, review_item_id, step_sequence, operation_state, pre_sha256,
				source_snapshot_path, destination_snapshot_path, last_successful_substep, started_at
			) VALUES (?, ?, ?, ?, 'in_progress', ?, ?, ?, 'precheck_hashed', ?)
		`, journalID, batchID, st.itemID, seq, preHash, srcFull, dstFull, stepStart)

		if mkErr := os.MkdirAll(filepath.Dir(dstFull), 0o755); mkErr != nil {
			_, _ = s.sqlDB.ExecContext(ctx, `UPDATE operation_journal SET operation_state = 'interrupted_power_or_disk', error_message = ? WHERE id = ?`, mkErr.Error(), journalID)
			res.AbortedCount++
			res.Errors = append(res.Errors, mkErr.Error())
			continue
		}

		if mvErr := os.Rename(srcFull, dstFull); mvErr != nil {
			_, _ = s.sqlDB.ExecContext(ctx, `UPDATE operation_journal SET operation_state = 'interrupted_power_or_disk', error_message = ? WHERE id = ?`, mvErr.Error(), journalID)
			_, _ = s.sqlDB.ExecContext(ctx, `UPDATE review_items SET decision_status = 'failed', conflict_details = ? WHERE id = ?`, mvErr.Error(), st.itemID)
			res.AbortedCount++
			res.Errors = append(res.Errors, mvErr.Error())
			continue
		}

		postHash, phErr := hashFile(dstFull)
		if phErr != nil || postHash != preHash {
			_ = os.Rename(dstFull, srcFull)
			_, _ = s.sqlDB.ExecContext(ctx, `UPDATE operation_journal SET operation_state = 'interrupted_power_or_disk', error_message = 'SHA-256 mismatch' WHERE id = ?`, journalID)
			return res, ErrHashMismatchAfterMove
		}

		doneStr := time.Now().UTC().Format(time.RFC3339Nano)
		newParent := filepath.ToSlash(filepath.Dir(filepath.FromSlash(st.effectiveRel)))
		if newParent == "." {
			newParent = ""
		}

		_, _ = s.sqlDB.ExecContext(ctx, `
			UPDATE media_files
			SET relative_path = ?, parent_rel_path = ?, filename = ?, sha256_hash = ?, updated_at = ?
			WHERE id = ?
		`, st.effectiveRel, newParent, newBase, postHash, doneStr, st.fileID)

		_, _ = s.sqlDB.ExecContext(ctx, `
			UPDATE operation_journal
			SET operation_state = 'completed', post_sha256 = ?, last_successful_substep = 'verified_complete', completed_at = ?
			WHERE id = ?
		`, postHash, doneStr, journalID)

		revAction := "move_back"
		if st.actionKind == "rename" {
			revAction = "rename_back"
		}
		_, _ = s.sqlDB.ExecContext(ctx, `
			INSERT INTO rollback_steps (
				id, journal_id, batch_id, reverse_order, reverse_action,
				from_disk_id, from_rel_path, to_disk_id, to_rel_path, expected_sha256, error_entry_id, rollback_status
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', 'ready')
		`, domain.NewImmutableID(), journalID, batchID, seq, revAction, st.diskID, st.effectiveRel, st.diskID, st.oldRel, postHash)

		_, _ = s.sqlDB.ExecContext(ctx, `UPDATE review_items SET decision_status = 'executed' WHERE id = ?`, st.itemID)
		res.ExecutedCount++

		_, rootRel := s.resolveEnclosingRoot(ctx, st.diskID, st.oldRel)
		stopBoundary := library.ResolveFullPath(st.mountPath, rootRel)
		cleanEmptyParentChain(filepath.Dir(srcFull), stopBoundary, scheduledEmptyDirs)
	}

	finalStatus := "completed"
	if res.ExecutedCount == 0 && res.AbortedCount > 0 {
		finalStatus = "halted_on_interrupt"
	}
	res.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
	_, _ = s.sqlDB.ExecContext(ctx, `UPDATE review_batches SET batch_status = ?, updated_at = ? WHERE id = ?`, finalStatus, res.CompletedAt, batchID)

	_, _ = s.sqlDB.ExecContext(ctx, `
		INSERT INTO audit_logs (id, event_category, severity, actor, summary, details_json)
		VALUES (?, 'fsops', 'info', 'user', ?, ?)
	`, domain.NewImmutableID(),
		fmt.Sprintf("Executed approved batch %s (%d moved/renamed, %d skipped, %d aborted)", batchID[:8], res.ExecutedCount, res.SkippedCount, res.AbortedCount),
		fmt.Sprintf(`{"batchId":%q,"executed":%d,"skipped":%d,"aborted":%d}`, batchID, res.ExecutedCount, res.SkippedCount, res.AbortedCount),
	)

	return res, nil
}

func (s *Service) RollbackBatch(ctx context.Context, batchID string) (RollbackResult, error) {
	if !s.mu.TryLock() {
		return RollbackResult{}, ErrOrganizerBusy
	}
	defer s.mu.Unlock()

	res := RollbackResult{
		BatchID: batchID,
		Errors:  []string{},
	}

	rows, err := s.sqlDB.QueryContext(ctx, `
		SELECT rs.id, rs.journal_id, oj.review_item_id, COALESCE(ri.file_id, ''),
		       rs.from_disk_id, d.mount_path, rs.from_rel_path, rs.to_rel_path,
		       COALESCE(rs.expected_sha256, ''), rs.error_entry_id
		FROM rollback_steps rs
		JOIN operation_journal oj ON oj.id = rs.journal_id
		JOIN review_items ri ON ri.id = oj.review_item_id
		JOIN disks d ON d.id = rs.from_disk_id
		WHERE rs.batch_id = ? AND rs.rollback_status = 'ready'
		ORDER BY rs.reverse_order DESC
	`, batchID)
	if err != nil {
		return res, err
	}
	type rbRow struct {
		rbID, journalID, itemID, fileID, diskID, mountPath, fromRel, toRel, expHash, errEntryID string
	}
	var list []rbRow
	for rows.Next() {
		var r rbRow
		if err := rows.Scan(&r.rbID, &r.journalID, &r.itemID, &r.fileID, &r.diskID, &r.mountPath, &r.fromRel, &r.toRel, &r.expHash, &r.errEntryID); err == nil {
			list = append(list, r)
		}
	}
	rows.Close()

	if len(list) == 0 {
		return res, errors.New("organizer: no ready rollback steps found for this batch")
	}

	for _, r := range list {
		if r.errEntryID != "" {
			if _, restErr := s.errBoxSvc.RestoreEntry(ctx, r.errEntryID); restErr != nil {
				if errors.Is(restErr, errorsbox.ErrRestoreCollision) {
					_, _ = s.sqlDB.ExecContext(ctx, `UPDATE rollback_steps SET rollback_status = 'blocked_collision' WHERE id = ?`, r.rbID)
				}
				res.BlockedCount++
				res.Errors = append(res.Errors, fmt.Sprintf("ErrorsBox restore %s: %v", r.toRel, restErr))
				continue
			}
			nowStr := time.Now().UTC().Format(time.RFC3339Nano)
			_, _ = s.sqlDB.ExecContext(ctx, `UPDATE rollback_steps SET rollback_status = 'executed', executed_at = ? WHERE id = ?`, nowStr, r.rbID)
			_, _ = s.sqlDB.ExecContext(ctx, `UPDATE operation_journal SET operation_state = 'rolled_back' WHERE id = ?`, r.journalID)
			_, _ = s.sqlDB.ExecContext(ctx, `UPDATE review_items SET decision_status = 'rolled_back' WHERE id = ?`, r.itemID)
			res.RestoredCount++
			continue
		}

		curFull := library.ResolveFullPath(r.mountPath, r.fromRel)
		origFull := library.ResolveFullPath(r.mountPath, r.toRel)

		if _, err := os.Stat(origFull); err == nil {
			_, _ = s.sqlDB.ExecContext(ctx, `UPDATE rollback_steps SET rollback_status = 'blocked_collision' WHERE id = ?`, r.rbID)
			res.BlockedCount++
			res.Errors = append(res.Errors, fmt.Sprintf("Rollback collision: %s already exists", origFull))
			continue
		}

		if r.expHash != "" {
			actualHash, hErr := hashFile(curFull)
			if hErr != nil || actualHash != r.expHash {
				res.BlockedCount++
				res.Errors = append(res.Errors, fmt.Sprintf("Rollback hash check failed on %s", curFull))
				continue
			}
		}

		if err := os.MkdirAll(filepath.Dir(origFull), 0o755); err != nil {
			res.BlockedCount++
			res.Errors = append(res.Errors, err.Error())
			continue
		}
		if err := os.Rename(curFull, origFull); err != nil {
			res.BlockedCount++
			res.Errors = append(res.Errors, err.Error())
			continue
		}

		nowStr := time.Now().UTC().Format(time.RFC3339Nano)
		origParent := filepath.ToSlash(filepath.Dir(filepath.FromSlash(r.toRel)))
		if origParent == "." {
			origParent = ""
		}
		origName := filepath.Base(filepath.FromSlash(r.toRel))

		if r.fileID != "" {
			_, _ = s.sqlDB.ExecContext(ctx, `
				UPDATE media_files SET relative_path = ?, parent_rel_path = ?, filename = ?, presence_status = 'present', updated_at = ?
				WHERE id = ?
			`, r.toRel, origParent, origName, nowStr, r.fileID)
		}
		_, _ = s.sqlDB.ExecContext(ctx, `UPDATE rollback_steps SET rollback_status = 'executed', executed_at = ? WHERE id = ?`, nowStr, r.rbID)
		_, _ = s.sqlDB.ExecContext(ctx, `UPDATE operation_journal SET operation_state = 'rolled_back' WHERE id = ?`, r.journalID)
		_, _ = s.sqlDB.ExecContext(ctx, `UPDATE review_items SET decision_status = 'rolled_back' WHERE id = ?`, r.itemID)
		res.RestoredCount++

		_, rootRel := s.resolveEnclosingRoot(ctx, r.diskID, r.fromRel)
		stopBoundary := library.ResolveFullPath(r.mountPath, rootRel)
		cleanEmptyParentChain(filepath.Dir(curFull), stopBoundary, nil)
	}

	res.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if res.RestoredCount > 0 && res.BlockedCount == 0 {
		_, _ = s.sqlDB.ExecContext(ctx, `UPDATE review_batches SET batch_status = 'rolled_back', updated_at = ? WHERE id = ?`, res.CompletedAt, batchID)
	}

	_, _ = s.sqlDB.ExecContext(ctx, `
		INSERT INTO audit_logs (id, event_category, severity, actor, summary, details_json)
		VALUES (?, 'fsops', 'warning', 'user', ?, ?)
	`, domain.NewImmutableID(),
		fmt.Sprintf("Rolled back batch %s (%d restored)", batchID[:8], res.RestoredCount),
		fmt.Sprintf(`{"batchId":%q,"restored":%d}`, batchID, res.RestoredCount),
	)
	return res, nil
}

func (s *Service) resolveEnclosingRoot(ctx context.Context, diskID, itemRel string) (string, string) {
	normItem := strings.Trim(filepath.ToSlash(itemRel), "/")
	rows, err := s.sqlDB.QueryContext(ctx, `
		SELECT id, relative_root_path FROM library_roots WHERE disk_id = ?
	`, diskID)
	if err != nil {
		return "", ""
	}
	defer rows.Close()
	bestID, bestRel := "", ""
	for rows.Next() {
		var rid, rrel string
		if err := rows.Scan(&rid, &rrel); err == nil {
			cleanR := strings.Trim(filepath.ToSlash(rrel), "/")
			if cleanR == "." {
				cleanR = ""
			}
			if cleanR == "" || normItem == cleanR || strings.HasPrefix(normItem, cleanR+"/") {
				if bestID == "" || len(cleanR) > len(bestRel) {
					bestID = rid
					bestRel = cleanR
				}
			}
		}
	}
	return bestID, bestRel
}

func (s *Service) ListJournalEntries(ctx context.Context, batchID string) ([]JournalEntryView, error) {
	q := `
		SELECT id, batch_id, step_sequence, operation_state, source_snapshot_path, destination_snapshot_path,
		       COALESCE(pre_sha256, ''), COALESCE(post_sha256, ''), error_message, COALESCE(completed_at, '')
		FROM operation_journal
	`
	args := []any{}
	if batchID != "" {
		q += ` WHERE batch_id = ?`
		args = append(args, batchID)
	}
	q += ` ORDER BY started_at DESC, step_sequence DESC LIMIT 200`

	rows, err := s.sqlDB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []JournalEntryView{}
	for rows.Next() {
		var j JournalEntryView
		if err := rows.Scan(&j.ID, &j.BatchID, &j.StepSequence, &j.OperationState, &j.SourcePath, &j.DestinationPath, &j.PreSHA256, &j.PostSHA256, &j.ErrorMessage, &j.CompletedAt); err == nil {
			out = append(out, j)
		}
	}
	return out, rows.Err()
}

func hashFile(path string) (string, error) {
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

func cleanEmptyParentChain(dirPath, stopBoundaryDir string, preserveScheduled map[string]bool) {
	cur := filepath.Clean(dirPath)
	boundary := filepath.Clean(stopBoundaryDir)
	for cur != "" && cur != "." && cur != boundary && len(cur) > len(boundary) {
		if preserveScheduled != nil && preserveScheduled[cur] {
			return
		}
		entries, err := os.ReadDir(cur)
		if err != nil || len(entries) > 0 {
			return
		}
		if rmErr := os.Remove(cur); rmErr != nil {
			return
		}
		cur = filepath.Dir(cur)
	}
}

func pruneEmptyChildSubdirs(topDir string) {
	cleanTop := filepath.Clean(topDir)
	entries, err := os.ReadDir(cleanTop)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			child := filepath.Join(cleanTop, e.Name())
			pruneEmptyChildSubdirs(child)
			if subEntries, sErr := os.ReadDir(child); sErr == nil && len(subEntries) == 0 {
				_ = os.Remove(child)
			}
		}
	}
}
