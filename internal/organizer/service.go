package organizer

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sadigking5-cmd/Estiraha-Next/internal/backup"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/domain"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/errorsbox"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/fsops"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/library"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/naming"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/review"
)

var (
	ErrNoApprovedItems       = errors.New("organizer: batch has zero user-approved items to execute")
	ErrOrganizerBusy         = errors.New("organizer: another file organization or rollback operation is currently running")
	ErrHashMismatchAfterMove = errors.New("organizer critical safety halt: post-move SHA-256 did not match pre-move SHA-256")
)

type ExecutionResult struct {
	BatchID          string   `json:"batchId"`
	ManifestID       string   `json:"manifestId"`
	ManifestSHA256   string   `json:"manifestSha256"`
	PreBatchBackupID string   `json:"preBatchBackupId"`
	ExecutedCount    int      `json:"executedCount"`
	SkippedCount     int      `json:"skippedCount"`
	AbortedCount     int      `json:"abortedCount"`
	Cancelled        bool     `json:"cancelled"`
	RemainingCount   int      `json:"remainingCount"`
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

type RecoveryResult struct {
	BatchID             string   `json:"batchId"`
	InspectedJournalIDs []string `json:"inspectedJournalIds"`
	FinalizedCount      int      `json:"finalizedCount"`
	RequeuedCount       int      `json:"requeuedCount"`
	RestoredCleanCount  int      `json:"restoredCleanCount"`
	Notes               []string `json:"notes"`
	CompletedAt         string   `json:"completedAt"`
}

type ExecutionProgressView struct {
	BatchID            string `json:"batchId"`
	Running            bool   `json:"running"`
	Paused             bool   `json:"paused"`
	CancelRequested    bool   `json:"cancelRequested"`
	ManifestID         string `json:"manifestId"`
	ManifestSHA256     string `json:"manifestSha256"`
	TotalSteps         int    `json:"totalSteps"`
	CurrentStep        int    `json:"currentStep"`
	CurrentItemName    string `json:"currentItemName"`
	ExecutedCount      int    `json:"executedCount"`
	InterruptedEntries int    `json:"interruptedEntries"`
}

type JournalEntryView struct {
	ID              string `json:"id"`
	BatchID         string `json:"batchId"`
	ReviewItemID    string `json:"reviewItemId"`
	StepSequence    int    `json:"stepSequence"`
	OperationState  string `json:"operationState"`
	SourcePath      string `json:"sourcePath"`
	DestinationPath string `json:"destinationPath"`
	PreSHA256       string `json:"preSha256"`
	PostSHA256      string `json:"postSha256"`
	ErrorMessage    string `json:"errorMessage"`
	CompletedAt     string `json:"completedAt"`
}

// StepHookFunc is an optional instrumentation hook invoked right before step index `stepIdx` (0-based)
// runs. Returning a non-nil error simulates a deterministic interruption before the step mutates disk.
type StepHookFunc func(stepIdx int, step review.ManifestStepSpec) error

type Service struct {
	sqlDB     *sql.DB
	backupSvc *backup.Service
	errBoxSvc *errorsbox.Service
	reviewSvc *review.Service
	mu        sync.Mutex

	ctrlMu          sync.Mutex
	ctrlCond        *sync.Cond
	running         bool
	activeBatchID   string
	activeManifest  string
	activeHash      string
	paused          bool
	cancelRequested bool
	totalSteps      int
	currentStep     int
	currentItemName string
	executedSoFar   int
	stepHook        StepHookFunc
}

func NewService(sqlDB *sql.DB, backupSvc *backup.Service) *Service {
	svc := &Service{
		sqlDB:     sqlDB,
		backupSvc: backupSvc,
		errBoxSvc: errorsbox.NewService(sqlDB),
		reviewSvc: review.NewService(sqlDB),
	}
	svc.ctrlCond = sync.NewCond(&svc.ctrlMu)
	return svc
}

func (s *Service) SetStepHook(fn StepHookFunc) {
	s.ctrlMu.Lock()
	defer s.ctrlMu.Unlock()
	s.stepHook = fn
}

func (s *Service) PauseExecution() bool {
	s.ctrlMu.Lock()
	defer s.ctrlMu.Unlock()
	if !s.running || s.cancelRequested {
		return false
	}
	s.paused = true
	if s.activeManifest != "" {
		_, _ = s.sqlDB.Exec(`UPDATE execution_manifests SET manifest_status = 'paused' WHERE id = ?`, s.activeManifest)
	}
	return true
}

func (s *Service) ResumeExecution() bool {
	s.ctrlMu.Lock()
	defer s.ctrlMu.Unlock()
	if !s.running || !s.paused {
		return false
	}
	s.paused = false
	if s.activeManifest != "" {
		_, _ = s.sqlDB.Exec(`UPDATE execution_manifests SET manifest_status = 'executing' WHERE id = ?`, s.activeManifest)
	}
	s.ctrlCond.Broadcast()
	return true
}

func (s *Service) CancelExecution() bool {
	s.ctrlMu.Lock()
	defer s.ctrlMu.Unlock()
	if !s.running {
		return false
	}
	s.cancelRequested = true
	s.paused = false
	s.ctrlCond.Broadcast()
	return true
}

func (s *Service) GetExecutionProgress(ctx context.Context, batchID string) (ExecutionProgressView, error) {
	s.ctrlMu.Lock()
	view := ExecutionProgressView{
		BatchID:         batchID,
		Running:         s.running && (batchID == "" || s.activeBatchID == batchID),
		Paused:          s.paused && (batchID == "" || s.activeBatchID == batchID),
		CancelRequested: s.cancelRequested && (batchID == "" || s.activeBatchID == batchID),
		ManifestID:      s.activeManifest,
		ManifestSHA256:  s.activeHash,
		TotalSteps:      s.totalSteps,
		CurrentStep:     s.currentStep,
		CurrentItemName: s.currentItemName,
		ExecutedCount:   s.executedSoFar,
	}
	s.ctrlMu.Unlock()

	if batchID != "" {
		_ = s.sqlDB.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM operation_journal
			WHERE batch_id = ? AND operation_state IN ('in_progress', 'interrupted_power_or_disk')
		`, batchID).Scan(&view.InterruptedEntries)

		if view.ManifestID == "" {
			_ = s.sqlDB.QueryRowContext(ctx, `
				SELECT id, manifest_sha256, total_approved_steps, executed_steps
				FROM execution_manifests
				WHERE batch_id = ?
				ORDER BY created_at DESC LIMIT 1
			`, batchID).Scan(&view.ManifestID, &view.ManifestSHA256, &view.TotalSteps, &view.ExecutedCount)
		}
	}
	return view, nil
}

func (s *Service) waitIfPausedOrCheckCancel(stepIdx int, itemName string) (cancelled bool, hook func(int, review.ManifestStepSpec) error) {
	s.ctrlMu.Lock()
	defer s.ctrlMu.Unlock()
	s.currentStep = stepIdx + 1
	s.currentItemName = itemName
	for s.paused && !s.cancelRequested {
		s.ctrlCond.Wait()
	}
	return s.cancelRequested, s.stepHook
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

	manifestSummary, mErr := s.reviewSvc.BuildExecutionManifestSummary(ctx, batchID)
	if mErr != nil {
		return res, mErr
	}
	steps := manifestSummary.Steps
	if len(steps) == 0 {
		return res, ErrNoApprovedItems
	}

	if s.backupSvc != nil && s.backupSvc.BackupDir() != "" {
		bk, bkErr := s.backupSvc.CreateVerifiedBackup(ctx, "pre_batch")
		if bkErr != nil {
			return res, fmt.Errorf("organizer safety gate: pre-batch verified backup failed: %w", bkErr)
		}
		res.PreBatchBackupID = bk.ID
	}

	manifestID := domain.NewImmutableID()
	stepsJSON, _ := json.Marshal(steps)
	summaryJSON, _ := json.Marshal(manifestSummary)
	nowStr := time.Now().UTC().Format(time.RFC3339Nano)

	if _, err := s.sqlDB.ExecContext(ctx, `
		INSERT INTO execution_manifests (
			id, batch_id, manifest_sha256, manifest_json, summary_json,
			total_approved_steps, executed_steps, manifest_status, pre_batch_backup_id, created_at
		) VALUES (?, ?, ?, ?, ?, ?, 0, 'executing', ?, ?)
	`, manifestID, batchID, manifestSummary.ManifestSHA256, string(stepsJSON), string(summaryJSON),
		len(steps), res.PreBatchBackupID, nowStr); err != nil {
		return res, fmt.Errorf("organizer: persist immutable execution manifest: %w", err)
	}
	res.ManifestID = manifestID
	res.ManifestSHA256 = manifestSummary.ManifestSHA256

	s.ctrlMu.Lock()
	s.running = true
	s.activeBatchID = batchID
	s.activeManifest = manifestID
	s.activeHash = manifestSummary.ManifestSHA256
	s.paused = false
	s.cancelRequested = false
	s.totalSteps = len(steps)
	s.currentStep = 0
	s.currentItemName = ""
	s.executedSoFar = 0
	s.ctrlMu.Unlock()

	defer func() {
		s.ctrlMu.Lock()
		s.running = false
		s.paused = false
		s.cancelRequested = false
		s.currentItemName = ""
		s.ctrlMu.Unlock()
	}()

	scheduledEmptyDirs := map[string]bool{}
	for _, st := range steps {
		if st.ItemFileKind == "directory" {
			fullDir := filepath.Clean(library.ResolveFullPath(st.MountPath, st.OldRelPath))
			scheduledEmptyDirs[fullDir] = true
		}
	}

	_, _ = s.sqlDB.ExecContext(ctx, `UPDATE review_batches SET batch_status = 'executing', updated_at = ? WHERE id = ?`, nowStr, batchID)

	var maxSeq int
	_ = s.sqlDB.QueryRowContext(ctx, `SELECT COALESCE(MAX(step_sequence), 0) FROM operation_journal WHERE batch_id = ?`, batchID).Scan(&maxSeq)

	interruptedHalt := false

	for idx, st := range steps {
		select {
		case <-ctx.Done():
			res.Cancelled = true
			res.RemainingCount = len(steps) - idx
		default:
		}
		if res.Cancelled {
			break
		}

		cancelled, hook := s.waitIfPausedOrCheckCancel(idx, st.OldName)
		if cancelled {
			res.Cancelled = true
			res.RemainingCount = len(steps) - idx
			break
		}

		seq := maxSeq + idx + 1

		if hook != nil {
			if hErr := hook(idx, st); hErr != nil {
				journalID := domain.NewImmutableID()
				stepStart := time.Now().UTC().Format(time.RFC3339Nano)
				srcFull := library.ResolveFullPath(st.MountPath, st.OldRelPath)
				dstFull := library.ResolveFullPath(st.MountPath, st.EffectiveRelPath)
				_, _ = s.sqlDB.ExecContext(ctx, `
					INSERT INTO operation_journal (
						id, batch_id, review_item_id, step_sequence, operation_state,
						source_snapshot_path, destination_snapshot_path, last_successful_substep, error_message, started_at
					) VALUES (?, ?, ?, ?, 'interrupted_power_or_disk', ?, ?, 'interrupted_before_rename', ?, ?)
				`, journalID, batchID, st.ReviewItemID, seq, srcFull, dstFull, hErr.Error(), stepStart)
				res.AbortedCount++
				res.RemainingCount = len(steps) - idx
				res.Errors = append(res.Errors, fmt.Sprintf("Execution interrupted at step %d (%s): %v", seq, st.OldName, hErr))
				interruptedHalt = true
				break
			}
		}

		approvedTime, tErr := time.Parse(time.RFC3339Nano, st.ApprovedAt)
		if tErr != nil || approvedTime.IsZero() {
			approvedTime = time.Now().UTC()
		}
		permit := review.ExecutionPermit{
			BatchID:      batchID,
			ReviewItemID: st.ReviewItemID,
			Decision:     review.DecisionApproved,
			ApprovedAt:   approvedTime,
		}

		if st.ActionKind == "move_to_errors" {
			if naming.IsShortsExtension(st.OldName) {
				res.AbortedCount++
				res.Errors = append(res.Errors, fmt.Sprintf("Safety law blocked moving .shorts to الأخطاء: %s", st.OldName))
				continue
			}

			rootID, rootRel := st.RootID, st.RootRelPath
			if rootRel == "" && st.RootID == "" {
				rootID, rootRel = s.resolveEnclosingRoot(ctx, st.DiskID, st.OldRelPath)
			}
			rootFull := library.ResolveFullPath(st.MountPath, rootRel)
			if _, _, ensErr := s.errBoxSvc.EnsureErrorStructure(rootFull); ensErr != nil {
				res.AbortedCount++
				res.Errors = append(res.Errors, ensErr.Error())
				continue
			}

			srcFull := library.ResolveFullPath(st.MountPath, st.OldRelPath)
			if st.ItemFileKind == "directory" {
				pruneEmptyChildSubdirs(srcFull)
				entries, rdErr := os.ReadDir(srcFull)
				if rdErr != nil || len(entries) > 0 {
					res.SkippedCount++
					_, _ = s.sqlDB.ExecContext(ctx, `
						UPDATE review_items SET decision_status = 'deferred', conflict_details = 'Skipped empty-folder move because unmoved files remain inside folder'
						WHERE id = ?
					`, st.ReviewItemID)
					continue
				}
			}

			journalID := domain.NewImmutableID()
			stepStart := time.Now().UTC().Format(time.RFC3339Nano)
			dstPreviewFull := library.ResolveFullPath(st.MountPath, st.EffectiveRelPath)

			_, _ = s.sqlDB.ExecContext(ctx, `
				INSERT INTO operation_journal (
					id, batch_id, review_item_id, step_sequence, operation_state,
					source_snapshot_path, destination_snapshot_path, last_successful_substep, started_at
				) VALUES (?, ?, ?, ?, 'in_progress', ?, ?, 'errorsbox_init', ?)
			`, journalID, batchID, st.ReviewItemID, seq, srcFull, dstPreviewFull, stepStart)

			ev, mvErr := s.errBoxSvc.MoveApprovedItem(ctx, errorsbox.MoveToErrorsRequest{
				Permit:          permit,
				DiskID:          st.DiskID,
				RootID:          rootID,
				MountPath:       st.MountPath,
				RootRelPath:     rootRel,
				FileID:          st.FileID,
				JournalID:       journalID,
				EntryKind:       st.ItemFileKind,
				Category:        errorsbox.CategoryCode(st.ErrorCategoryCode),
				OriginalRelPath: st.OldRelPath,
				ReasonAr:        st.ReasonDescription,
				ReasonEn:        st.ReasonDescription,
			})
			if mvErr != nil {
				_, _ = s.sqlDB.ExecContext(ctx, `
					UPDATE operation_journal SET operation_state = 'aborted_target_exists', error_message = ?, completed_at = ?
					WHERE id = ?
				`, mvErr.Error(), stepStart, journalID)
				_, _ = s.sqlDB.ExecContext(ctx, `UPDATE review_items SET decision_status = 'failed', conflict_details = ? WHERE id = ?`, mvErr.Error(), st.ReviewItemID)
				res.AbortedCount++
				res.Errors = append(res.Errors, fmt.Sprintf("%s -> %v", st.OldName, mvErr))
				continue
			}

			actualDstFull := library.ResolveFullPath(st.MountPath, ev.ErrorRelPath)
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
			`, domain.NewImmutableID(), journalID, batchID, seq, st.DiskID, ev.ErrorRelPath, st.DiskID, st.OldRelPath, hashNullable, ev.ID)

			_, _ = s.sqlDB.ExecContext(ctx, `UPDATE review_items SET decision_status = 'executed' WHERE id = ?`, st.ReviewItemID)
			res.ExecutedCount++
			s.recordStepSuccess(manifestID, res.ExecutedCount)

			if st.ItemFileKind == "file" {
				cleanEmptyParentChain(filepath.Dir(srcFull), rootFull, scheduledEmptyDirs)
			}
			continue
		}

		newBase := filepath.Base(filepath.FromSlash(st.EffectiveRelPath))
		if naming.IsShortsExtension(st.OldName) && !naming.IsShortsExtension(newBase) {
			res.AbortedCount++
			res.Errors = append(res.Errors, fmt.Sprintf("Blocked .shorts alteration on %s", st.OldName))
			continue
		}

		srcFull := library.ResolveFullPath(st.MountPath, st.OldRelPath)
		dstFull := library.ResolveFullPath(st.MountPath, st.EffectiveRelPath)

		journalID := domain.NewImmutableID()
		stepStart := time.Now().UTC().Format(time.RFC3339Nano)

		if err := fsops.VerifySafeDestination(permit, srcFull, dstFull); err != nil {
			state := "aborted_target_exists"
			_, _ = s.sqlDB.ExecContext(ctx, `
				INSERT INTO operation_journal (
					id, batch_id, review_item_id, step_sequence, operation_state,
					source_snapshot_path, destination_snapshot_path, last_successful_substep, error_message, started_at, completed_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, 'precheck', ?, ?, ?)
			`, journalID, batchID, st.ReviewItemID, seq, state, srcFull, dstFull, err.Error(), stepStart, stepStart)
			_, _ = s.sqlDB.ExecContext(ctx, `UPDATE review_items SET decision_status = 'failed', conflict_detected = 1, conflict_details = ? WHERE id = ?`, err.Error(), st.ReviewItemID)
			res.AbortedCount++
			res.Errors = append(res.Errors, fmt.Sprintf("%s -> %v", st.OldName, err))
			continue
		}

		preHash, hErr := hashFile(srcFull)
		if hErr != nil {
			res.AbortedCount++
			res.Errors = append(res.Errors, fmt.Sprintf("pre-hash failed %s: %v", st.OldName, hErr))
			continue
		}

		_, _ = s.sqlDB.ExecContext(ctx, `
			INSERT INTO operation_journal (
				id, batch_id, review_item_id, step_sequence, operation_state, pre_sha256,
				source_snapshot_path, destination_snapshot_path, last_successful_substep, started_at
			) VALUES (?, ?, ?, ?, 'in_progress', ?, ?, ?, 'precheck_hashed', ?)
		`, journalID, batchID, st.ReviewItemID, seq, preHash, srcFull, dstFull, stepStart)

		if mkErr := os.MkdirAll(filepath.Dir(dstFull), 0o755); mkErr != nil {
			_, _ = s.sqlDB.ExecContext(ctx, `UPDATE operation_journal SET operation_state = 'interrupted_power_or_disk', error_message = ? WHERE id = ?`, mkErr.Error(), journalID)
			res.AbortedCount++
			res.Errors = append(res.Errors, mkErr.Error())
			continue
		}

		if mvErr := os.Rename(srcFull, dstFull); mvErr != nil {
			_, _ = s.sqlDB.ExecContext(ctx, `UPDATE operation_journal SET operation_state = 'interrupted_power_or_disk', error_message = ? WHERE id = ?`, mvErr.Error(), journalID)
			_, _ = s.sqlDB.ExecContext(ctx, `UPDATE review_items SET decision_status = 'failed', conflict_details = ? WHERE id = ?`, mvErr.Error(), st.ReviewItemID)
			res.AbortedCount++
			res.Errors = append(res.Errors, mvErr.Error())
			continue
		}

		postHash, phErr := hashFile(dstFull)
		if phErr != nil || postHash != preHash {
			_ = os.Rename(dstFull, srcFull)
			_, _ = s.sqlDB.ExecContext(ctx, `UPDATE operation_journal SET operation_state = 'interrupted_power_or_disk', error_message = 'SHA-256 mismatch' WHERE id = ?`, journalID)
			_, _ = s.sqlDB.ExecContext(ctx, `UPDATE execution_manifests SET manifest_status = 'halted_on_interrupt', completed_at = ? WHERE id = ?`, time.Now().UTC().Format(time.RFC3339Nano), manifestID)
			return res, ErrHashMismatchAfterMove
		}

		doneStr := time.Now().UTC().Format(time.RFC3339Nano)
		newParent := filepath.ToSlash(filepath.Dir(filepath.FromSlash(st.EffectiveRelPath)))
		if newParent == "." {
			newParent = ""
		}

		_, _ = s.sqlDB.ExecContext(ctx, `
			UPDATE media_files
			SET relative_path = ?, parent_rel_path = ?, filename = ?, sha256_hash = ?, updated_at = ?
			WHERE id = ?
		`, st.EffectiveRelPath, newParent, newBase, postHash, doneStr, st.FileID)

		_, _ = s.sqlDB.ExecContext(ctx, `
			UPDATE operation_journal
			SET operation_state = 'completed', post_sha256 = ?, last_successful_substep = 'verified_complete', completed_at = ?
			WHERE id = ?
		`, postHash, doneStr, journalID)

		revAction := "move_back"
		if st.ActionKind == "rename" {
			revAction = "rename_back"
		}
		_, _ = s.sqlDB.ExecContext(ctx, `
			INSERT INTO rollback_steps (
				id, journal_id, batch_id, reverse_order, reverse_action,
				from_disk_id, from_rel_path, to_disk_id, to_rel_path, expected_sha256, error_entry_id, rollback_status
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', 'ready')
		`, domain.NewImmutableID(), journalID, batchID, seq, revAction, st.DiskID, st.EffectiveRelPath, st.DiskID, st.OldRelPath, postHash)

		_, _ = s.sqlDB.ExecContext(ctx, `UPDATE review_items SET decision_status = 'executed' WHERE id = ?`, st.ReviewItemID)
		res.ExecutedCount++
		s.recordStepSuccess(manifestID, res.ExecutedCount)

		_, rootRel := s.resolveEnclosingRoot(ctx, st.DiskID, st.OldRelPath)
		stopBoundary := library.ResolveFullPath(st.MountPath, rootRel)
		cleanEmptyParentChain(filepath.Dir(srcFull), stopBoundary, scheduledEmptyDirs)
	}

	res.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)

	finalBatchStatus := "completed"
	finalManifestStatus := "completed"
	if interruptedHalt || (res.ExecutedCount == 0 && res.AbortedCount > 0) {
		finalBatchStatus = "halted_on_interrupt"
		finalManifestStatus = "halted_on_interrupt"
	} else if res.Cancelled {
		finalBatchStatus = "partially_approved"
		finalManifestStatus = "cancelled"
	}

	var remainingApproved int
	_ = s.sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM review_items WHERE batch_id = ? AND decision_status = 'approved'`, batchID).Scan(&remainingApproved)
	_, _ = s.sqlDB.ExecContext(ctx, `
		UPDATE review_batches SET batch_status = ?, approved_items = ?, updated_at = ? WHERE id = ?
	`, finalBatchStatus, remainingApproved, res.CompletedAt, batchID)
	_, _ = s.sqlDB.ExecContext(ctx, `
		UPDATE execution_manifests SET manifest_status = ?, executed_steps = ?, completed_at = ? WHERE id = ?
	`, finalManifestStatus, res.ExecutedCount, res.CompletedAt, manifestID)

	_, _ = s.sqlDB.ExecContext(ctx, `
		INSERT INTO audit_logs (id, event_category, severity, actor, summary, details_json)
		VALUES (?, 'fsops', 'info', 'user', ?, ?)
	`, domain.NewImmutableID(),
		fmt.Sprintf("Executed approved batch %s [manifest %s] (%d moved/renamed, %d skipped, %d aborted, cancelled=%v)",
			batchID[:min(8, len(batchID))], res.ManifestSHA256[:12], res.ExecutedCount, res.SkippedCount, res.AbortedCount, res.Cancelled),
		fmt.Sprintf(`{"batchId":%q,"manifestId":%q,"manifestSha256":%q,"executed":%d,"skipped":%d,"aborted":%d,"cancelled":%v}`,
			batchID, manifestID, res.ManifestSHA256, res.ExecutedCount, res.SkippedCount, res.AbortedCount, res.Cancelled),
	)

	if interruptedHalt {
		return res, errors.New(res.Errors[len(res.Errors)-1])
	}
	return res, nil
}

func (s *Service) recordStepSuccess(manifestID string, count int) {
	s.ctrlMu.Lock()
	s.executedSoFar = count
	s.ctrlMu.Unlock()
	_, _ = s.sqlDB.Exec(`UPDATE execution_manifests SET executed_steps = ? WHERE id = ?`, count, manifestID)
}

func (s *Service) RecoverInterruptedBatch(ctx context.Context, batchID string) (RecoveryResult, error) {
	if !s.mu.TryLock() {
		return RecoveryResult{}, ErrOrganizerBusy
	}
	defer s.mu.Unlock()

	res := RecoveryResult{
		BatchID:             batchID,
		InspectedJournalIDs: []string{},
		Notes:               []string{},
	}

	rows, err := s.sqlDB.QueryContext(ctx, `
		SELECT oj.id, oj.review_item_id, oj.step_sequence, COALESCE(oj.pre_sha256, ''),
		       oj.source_snapshot_path, oj.destination_snapshot_path,
		       COALESCE(ri.file_id, ''), ri.item_file_kind, ri.action_kind, ri.source_disk_id,
		       ri.old_rel_path, COALESCE(NULLIF(ri.user_edited_rel_path, ''), ri.proposed_rel_path)
		FROM operation_journal oj
		JOIN review_items ri ON ri.id = oj.review_item_id
		WHERE oj.batch_id = ? AND oj.operation_state IN ('in_progress', 'interrupted_power_or_disk')
		ORDER BY oj.step_sequence ASC
	`, batchID)
	if err != nil {
		return res, err
	}
	type intRow struct {
		journalID, itemID, preHash, srcFull, dstFull, fileID, itemKind, actionKind, diskID, oldRel, effRel string
		seq                                                                                                int
	}
	var list []intRow
	for rows.Next() {
		var r intRow
		if err := rows.Scan(
			&r.journalID, &r.itemID, &r.seq, &r.preHash,
			&r.srcFull, &r.dstFull, &r.fileID, &r.itemKind, &r.actionKind, &r.diskID, &r.oldRel, &r.effRel,
		); err == nil {
			list = append(list, r)
		}
	}
	rows.Close()

	nowStr := time.Now().UTC().Format(time.RFC3339Nano)

	for _, r := range list {
		res.InspectedJournalIDs = append(res.InspectedJournalIDs, r.journalID)
		_, srcErr := os.Stat(r.srcFull)
		_, dstErr := os.Stat(r.dstFull)
		srcExists := srcErr == nil
		dstExists := dstErr == nil

		if srcExists && !dstExists {
			_, _ = s.sqlDB.ExecContext(ctx, `
				UPDATE operation_journal
				SET operation_state = 'rolled_back', last_successful_substep = 'recovered_source_intact',
				    error_message = 'Recovered after interruption: source intact, ready to resume', completed_at = ?
				WHERE id = ?
			`, nowStr, r.journalID)
			_, _ = s.sqlDB.ExecContext(ctx, `
				UPDATE review_items SET decision_status = 'approved', conflict_details = '' WHERE id = ?
			`, r.itemID)
			res.RequeuedCount++
			res.Notes = append(res.Notes, fmt.Sprintf("Step #%d: source intact (%s) — requeued safely", r.seq, filepath.Base(r.srcFull)))
			continue
		}

		if !srcExists && dstExists {
			verifiedHash := r.preHash
			if r.itemKind == "file" {
				dstHash, hErr := hashFile(r.dstFull)
				if hErr != nil || (r.preHash != "" && dstHash != r.preHash) {
					_ = os.MkdirAll(filepath.Dir(r.srcFull), 0o755)
					_ = os.Rename(r.dstFull, r.srcFull)
					_, _ = s.sqlDB.ExecContext(ctx, `
						UPDATE operation_journal SET operation_state = 'rolled_back', error_message = 'Recovered by restoring source due to hash mismatch', completed_at = ? WHERE id = ?
					`, nowStr, r.journalID)
					_, _ = s.sqlDB.ExecContext(ctx, `UPDATE review_items SET decision_status = 'approved' WHERE id = ?`, r.itemID)
					res.RestoredCleanCount++
					continue
				}
				verifiedHash = dstHash
			}

			newBase := filepath.Base(filepath.FromSlash(r.effRel))
			newParent := filepath.ToSlash(filepath.Dir(filepath.FromSlash(r.effRel)))
			if newParent == "." {
				newParent = ""
			}
			if r.fileID != "" && r.itemKind == "file" {
				_, _ = s.sqlDB.ExecContext(ctx, `
					UPDATE media_files SET relative_path = ?, parent_rel_path = ?, filename = ?, sha256_hash = COALESCE(NULLIF(?, ''), sha256_hash), updated_at = ?
					WHERE id = ?
				`, r.effRel, newParent, newBase, verifiedHash, nowStr, r.fileID)
			}
			_, _ = s.sqlDB.ExecContext(ctx, `
				UPDATE operation_journal
				SET operation_state = 'completed', post_sha256 = ?, last_successful_substep = 'recovered_verified_complete', completed_at = ?
				WHERE id = ?
			`, verifiedHash, nowStr, r.journalID)
			revAction := "move_back"
			if r.actionKind == "rename" {
				revAction = "rename_back"
			}
			_, _ = s.sqlDB.ExecContext(ctx, `
				INSERT OR IGNORE INTO rollback_steps (
					id, journal_id, batch_id, reverse_order, reverse_action,
					from_disk_id, from_rel_path, to_disk_id, to_rel_path, expected_sha256, error_entry_id, rollback_status
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', 'ready')
			`, domain.NewImmutableID(), r.journalID, batchID, r.seq, revAction, r.diskID, r.effRel, r.diskID, r.oldRel, verifiedHash)
			_, _ = s.sqlDB.ExecContext(ctx, `UPDATE review_items SET decision_status = 'executed' WHERE id = ?`, r.itemID)
			res.FinalizedCount++
			res.Notes = append(res.Notes, fmt.Sprintf("Step #%d: destination verified via SHA-256 (%s) — finalized with rollback step", r.seq, newBase))
			continue
		}

		_, _ = s.sqlDB.ExecContext(ctx, `
			UPDATE operation_journal
			SET operation_state = 'aborted_target_exists', error_message = 'Recovery detected ambiguous state; preserved both paths untouched', completed_at = ?
			WHERE id = ?
		`, nowStr, r.journalID)
		_, _ = s.sqlDB.ExecContext(ctx, `
			UPDATE review_items SET decision_status = 'deferred', conflict_detected = 1, conflict_details = 'Interrupted with both source & target present; preserved untouched' WHERE id = ?
		`, r.itemID)
		res.Notes = append(res.Notes, fmt.Sprintf("Step #%d: preserved untouched for manual inspection", r.seq))
	}

	var approvedRemaining, executedTotal int
	_ = s.sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM review_items WHERE batch_id = ? AND decision_status = 'approved'`, batchID).Scan(&approvedRemaining)
	_ = s.sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM review_items WHERE batch_id = ? AND decision_status = 'executed'`, batchID).Scan(&executedTotal)

	nextStatus := "awaiting_approval"
	if approvedRemaining > 0 {
		nextStatus = "partially_approved"
	} else if executedTotal > 0 {
		nextStatus = "completed"
	}
	res.CompletedAt = nowStr
	_, _ = s.sqlDB.ExecContext(ctx, `UPDATE review_batches SET batch_status = ?, approved_items = ?, updated_at = ? WHERE id = ?`, nextStatus, approvedRemaining, nowStr, batchID)

	_, _ = s.sqlDB.ExecContext(ctx, `
		INSERT INTO audit_logs (id, event_category, severity, actor, summary, details_json)
		VALUES (?, 'safety', 'info', 'user', ?, ?)
	`, domain.NewImmutableID(),
		fmt.Sprintf("Recovered interrupted batch %s (finalized=%d, requeued=%d)", batchID[:min(8, len(batchID))], res.FinalizedCount, res.RequeuedCount),
		fmt.Sprintf(`{"batchId":%q,"finalized":%d,"requeued":%d}`, batchID, res.FinalizedCount, res.RequeuedCount),
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

		if r.expHash != "" {
			restoredHash, hErr := hashFile(origFull)
			if hErr != nil || restoredHash != r.expHash {
				_ = os.Rename(origFull, curFull)
				res.BlockedCount++
				res.Errors = append(res.Errors, fmt.Sprintf("Post-rollback SHA-256 verification failed on %s", origFull))
				continue
			}
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
		_, _ = s.sqlDB.ExecContext(ctx, `UPDATE execution_manifests SET manifest_status = 'rolled_back', completed_at = ? WHERE batch_id = ?`, res.CompletedAt, batchID)
	}

	_, _ = s.sqlDB.ExecContext(ctx, `
		INSERT INTO audit_logs (id, event_category, severity, actor, summary, details_json)
		VALUES (?, 'fsops', 'warning', 'user', ?, ?)
	`, domain.NewImmutableID(),
		fmt.Sprintf("Rolled back batch %s (%d restored)", batchID[:min(8, len(batchID))], res.RestoredCount),
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
		SELECT id, batch_id, review_item_id, step_sequence, operation_state, source_snapshot_path, destination_snapshot_path,
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
		if err := rows.Scan(&j.ID, &j.BatchID, &j.ReviewItemID, &j.StepSequence, &j.OperationState, &j.SourcePath, &j.DestinationPath, &j.PreSHA256, &j.PostSHA256, &j.ErrorMessage, &j.CompletedAt); err == nil {
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
