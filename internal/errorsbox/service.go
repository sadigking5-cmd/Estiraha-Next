package errorsbox

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
	"strings"
	"sync"
	"time"

	"github.com/sadigking5-cmd/Estiraha-Next/internal/domain"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/fsops"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/library"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/naming"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/safetygate"
)

// OfficialRootFolderName is the managed error root directory inside a library root.
// Scanner automatically skips this folder so isolated anomalies are never re-indexed as movies.
const OfficialRootFolderName = "الأخطاء"

// CategoryCode enumerates the 6 official managed error folder categories.
type CategoryCode string

const (
	CategoryEmptyFolders      CategoryCode = "empty_folders"
	CategoryZeroByteFiles     CategoryCode = "zero_byte_files"
	CategoryUnlinkedSubtitles CategoryCode = "unlinked_subtitles"
	CategoryAuxiliaryUnknown  CategoryCode = "auxiliary_or_unknown"
	CategorySamplesShortClips CategoryCode = "samples_and_short_clips"
	CategoryConflictsReview   CategoryCode = "conflicts_needing_review"
)

// CategorySpec maps an internal CategoryCode to its exact official Arabic folder name and labels.
type CategorySpec struct {
	Code        CategoryCode `json:"code"`
	FolderAr    string       `json:"folderAr"`
	TitleEn     string       `json:"titleEn"`
	Description string       `json:"description"`
}

var (
	ErrUnsupportedCategory = errors.New("errorsbox: unsupported error folder category")
	ErrShortsProtected     = errors.New("errorsbox safety law: .shorts companion files cannot be routed to samples or error folders")
	ErrEntryNotFound       = errors.New("errorsbox: error folder entry not found")
	ErrRestoreCollision    = errors.New("errorsbox safety violation: original path already exists; overwrite forbidden")
)

var officialCategories = []CategorySpec{
	{
		Code:        CategoryEmptyFolders,
		FolderAr:    "مجلدات فارغة",
		TitleEn:     "Empty Folders",
		Description: "المجلدات الفارغة التي تم إخلاؤها أو كانت بلا ملفات صالحة",
	},
	{
		Code:        CategoryZeroByteFiles,
		FolderAr:    "ملفات صفرية",
		TitleEn:     "Zero-Byte Files",
		Description: "الملفات التالفة ذات الحجم 0 بايت",
	},
	{
		Code:        CategoryUnlinkedSubtitles,
		FolderAr:    "ترجمات غير مرتبطة",
		TitleEn:     "Unlinked Subtitles",
		Description: "ملفات الترجمة التي لا يوجد لها فيلم مطابق في المسار",
	},
	{
		Code:        CategoryAuxiliaryUnknown,
		FolderAr:    "ملفات مساعدة وغير معروفة",
		TitleEn:     "Auxiliary & System Artifacts",
		Description: "ملفات النظام المصنفة ومخلفات التورنت المؤكدة (AppleDouble ._*، Thumbs.db، desktop.ini، الاختصارات)",
	},
	{
		Code:        CategorySamplesShortClips,
		FolderAr:    "عينات ومقاطع قصيرة",
		TitleEn:     "Samples & Short Clips",
		Description: "مقاطع العينات المؤكدة بالتحليل الفني (Sample / مصغرات الفيديو)",
	},
	{
		Code:        CategoryConflictsReview,
		FolderAr:    "تعارضات تحتاج مراجعة",
		TitleEn:     "Conflicts Needing Review",
		Description: "الملفات المتعارضة أو التي تعذر حسم تحليلها الفني وتحتاج مراجعة يدوية",
	},
}

// AllCategories returns a copy of all 6 official managed error categories in canonical order.
func AllCategories() []CategorySpec {
	out := make([]CategorySpec, len(officialCategories))
	copy(out, officialCategories)
	return out
}

// LookupCategory resolves a CategoryCode to its official Arabic folder specification.
func LookupCategory(code CategoryCode) (CategorySpec, error) {
	for _, c := range officialCategories {
		if c.Code == code {
			return c, nil
		}
	}
	return CategorySpec{}, fmt.Errorf("%w: %q", ErrUnsupportedCategory, code)
}

// IsManagedErrorDirName returns true if dirName matches the official managed error root folder ("الأخطاء").
func IsManagedErrorDirName(dirName string) bool {
	return strings.TrimSpace(dirName) == OfficialRootFolderName
}

// IsInsideManagedErrorPath returns true if any path segment in relOrFullPath is "الأخطاء".
func IsInsideManagedErrorPath(relOrFullPath string) bool {
	norm := filepath.ToSlash(strings.TrimSpace(relOrFullPath))
	for _, seg := range strings.Split(norm, "/") {
		if strings.TrimSpace(seg) == OfficialRootFolderName {
			return true
		}
	}
	return false
}

// EntryView represents a tracked item inside `الأخطاء/<التصنيف>/` with restore provenance.
type EntryView struct {
	ID               string       `json:"id"`
	DiskID           string       `json:"diskId"`
	DiskLabel        string       `json:"diskLabel"`
	RootID           string       `json:"rootId"`
	FileID           string       `json:"fileId"`
	BatchID          string       `json:"batchId"`
	BatchTitle       string       `json:"batchTitle"`
	JournalID        string       `json:"journalId"`
	EntryKind        string       `json:"entryKind"`
	CategoryCode     CategoryCode `json:"categoryCode"`
	CategoryFolderAr string       `json:"categoryFolderAr"`
	OriginalRelPath  string       `json:"originalRelPath"`
	OriginalFullPath string       `json:"originalFullPath"`
	ErrorRelPath     string       `json:"errorRelPath"`
	ErrorFullPath    string       `json:"errorFullPath"`
	ItemName         string       `json:"itemName"`
	SizeBytes        int64        `json:"sizeBytes"`
	SHA256Hash       string       `json:"sha256Hash"`
	ReasonAr         string       `json:"reasonAr"`
	ReasonEn         string       `json:"reasonEn"`
	EntryStatus      string       `json:"entryStatus"`
	MovedAt          string       `json:"movedAt"`
	RestoredAt       string       `json:"restoredAt"`
}

// BatchRestoreSummary reports the result of restoring multiple selected error entries or a reversible batch.
type BatchRestoreSummary struct {
	RestoredCount int         `json:"restoredCount"`
	BlockedCount  int         `json:"blockedCount"`
	RestoredItems []EntryView `json:"restoredItems"`
	Errors        []string    `json:"errors"`
	CompletedAt   string      `json:"completedAt"`
}

// MoveToErrorsRequest encapsulates a permit-gated move into `الأخطاء/<التصنيف>/`.
type MoveToErrorsRequest struct {
	Permit          safetygate.ExecutionPermit
	DiskID          string
	RootID          string
	MountPath       string
	RootRelPath     string
	FileID          string
	JournalID       string
	EntryKind       string // "file" or "directory"
	Category        CategoryCode
	OriginalRelPath string
	ReasonAr        string
	ReasonEn        string
}

// Service manages the 6-category `الأخطاء/` directory hierarchy, collision-free destination
// planning, SHA-256 verified quarantine moves, and bit-for-bit restoration.
type Service struct {
	sqlDB *sql.DB
	mu    sync.Mutex
}

func NewService(sqlDB *sql.DB) *Service {
	return &Service{sqlDB: sqlDB}
}

// EnsureErrorStructure creates `<RootFullPath>/الأخطاء/` and all 6 official Arabic category subfolders.
func (s *Service) EnsureErrorStructure(rootFullPath string) (string, map[CategoryCode]string, error) {
	cleanRoot := filepath.Clean(strings.TrimSpace(rootFullPath))
	if cleanRoot == "" || cleanRoot == "." {
		return "", nil, errors.New("errorsbox: rootFullPath cannot be empty")
	}
	errRoot := filepath.Join(cleanRoot, OfficialRootFolderName)
	if err := os.MkdirAll(errRoot, 0o755); err != nil {
		return "", nil, fmt.Errorf("errorsbox: create root %s: %w", errRoot, err)
	}
	paths := make(map[CategoryCode]string, len(officialCategories))
	for _, cat := range officialCategories {
		catDir := filepath.Join(errRoot, cat.FolderAr)
		if err := os.MkdirAll(catDir, 0o755); err != nil {
			return "", nil, fmt.Errorf("errorsbox: create category %s: %w", cat.FolderAr, err)
		}
		paths[cat.Code] = catDir
	}
	return errRoot, paths, nil
}

// PlanCollisionFreeRelPath computes a guaranteed collision-free relative path (relative to disk mount_path)
// inside `<rootRelPath>/الأخطاء/<التصنيف>/`.
// If two items from different folders share the same basename (e.g., `Thumbs.db` or `WWW.YIFY-TORRENTS.COM.jpg`),
// the parent folder context and/or numeric suffix ` (2)`, ` (3)` is disambiguated so no file is EVER overwritten.
func (s *Service) PlanCollisionFreeRelPath(
	ctx context.Context,
	diskID string,
	mountPath string,
	rootRelPath string,
	category CategoryCode,
	originalRelPath string,
	reservedInBatch map[string]bool,
) (string, error) {
	spec, err := LookupCategory(category)
	if err != nil {
		return "", err
	}

	origSlash := strings.Trim(filepath.ToSlash(originalRelPath), "/")
	itemBase := filepath.Base(filepath.FromSlash(origSlash))
	if naming.IsShortsExtension(itemBase) && category == CategorySamplesShortClips {
		return "", ErrShortsProtected
	}

	rootSlash := strings.Trim(filepath.ToSlash(rootRelPath), "/")
	if rootSlash == "." {
		rootSlash = ""
	}

	var catBaseRel string
	if rootSlash == "" {
		catBaseRel = OfficialRootFolderName + "/" + spec.FolderAr
	} else {
		catBaseRel = rootSlash + "/" + OfficialRootFolderName + "/" + spec.FolderAr
	}

	relInsideRoot := origSlash
	if rootSlash != "" && strings.HasPrefix(origSlash, rootSlash+"/") {
		relInsideRoot = strings.TrimPrefix(origSlash, rootSlash+"/")
	}
	parentInsideRoot := filepath.ToSlash(filepath.Dir(filepath.FromSlash(relInsideRoot)))
	if parentInsideRoot == "." {
		parentInsideRoot = ""
	}

	candidateName := sanitizeSegmentPreservingExt(itemBase)
	candidateRel := catBaseRel + "/" + candidateName

	if !s.isPathTaken(ctx, diskID, mountPath, candidateRel, reservedInBatch) {
		if reservedInBatch != nil {
			reservedInBatch[strings.ToLower(candidateRel)] = true
		}
		return candidateRel, nil
	}

	// Disambiguate first with parent folder context (e.g., `Blow (2001)__Thumbs.db`)
	if parentInsideRoot != "" {
		flatParent := strings.ReplaceAll(parentInsideRoot, "/", "__")
		flatParent = sanitizeSegmentPreservingExt(flatParent)
		prefixedName := flatParent + "__" + candidateName
		prefixedRel := catBaseRel + "/" + prefixedName
		if !s.isPathTaken(ctx, diskID, mountPath, prefixedRel, reservedInBatch) {
			if reservedInBatch != nil {
				reservedInBatch[strings.ToLower(prefixedRel)] = true
			}
			return prefixedRel, nil
		}
		candidateName = prefixedName
	}

	ext := filepath.Ext(candidateName)
	stem := strings.TrimSuffix(candidateName, ext)
	if naming.IsShortsExtension(candidateName) {
		ext = ".shorts"
		stem = strings.TrimSuffix(candidateName, ".shorts")
	}

	for n := 2; n < 10000; n++ {
		numbered := fmt.Sprintf("%s (%d)%s", stem, n, ext)
		numberedRel := catBaseRel + "/" + numbered
		if !s.isPathTaken(ctx, diskID, mountPath, numberedRel, reservedInBatch) {
			if reservedInBatch != nil {
				reservedInBatch[strings.ToLower(numberedRel)] = true
			}
			return numberedRel, nil
		}
	}

	return "", errors.New("errorsbox: exhausted 10,000 collision-free candidate names")
}

func (s *Service) isPathTaken(ctx context.Context, diskID, mountPath, candidateRel string, reserved map[string]bool) bool {
	lowerKey := strings.ToLower(strings.Trim(filepath.ToSlash(candidateRel), "/"))
	if reserved != nil && reserved[lowerKey] {
		return true
	}
	if mountPath != "" {
		full := library.ResolveFullPath(mountPath, candidateRel)
		if _, err := os.Stat(full); err == nil {
			return true
		}
	}
	if s.sqlDB != nil && diskID != "" {
		var existing string
		err := s.sqlDB.QueryRowContext(ctx, `
			SELECT id FROM error_folder_entries
			WHERE disk_id = ? AND lower(error_rel_path) = lower(?) AND entry_status = 'active'
			LIMIT 1
		`, diskID, candidateRel).Scan(&existing)
		if err == nil && existing != "" {
			return true
		}
		err = s.sqlDB.QueryRowContext(ctx, `
			SELECT id FROM media_files
			WHERE disk_id = ? AND lower(relative_path) = lower(?) AND presence_status IN ('present', 'moved_to_errors')
			LIMIT 1
		`, diskID, candidateRel).Scan(&existing)
		if err == nil && existing != "" {
			return true
		}
	}
	return false
}

// MoveApprovedItem relocates an approved file or empty directory into `الأخطاء/<التصنيف>/`
// with strict permit verification, collision-free destination naming, SHA-256 verification (for files),
// and full provenance persistence in `error_folder_entries`.
func (s *Service) MoveApprovedItem(ctx context.Context, req MoveToErrorsRequest) (EntryView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := safetygate.ValidatePermit(req.Permit); err != nil {
		return EntryView{}, err
	}
	spec, err := LookupCategory(req.Category)
	if err != nil {
		return EntryView{}, err
	}

	entryKind := req.EntryKind
	if entryKind == "" {
		entryKind = "file"
	}
	if entryKind != "file" && entryKind != "directory" {
		return EntryView{}, fmt.Errorf("errorsbox: invalid entryKind %q", entryKind)
	}

	origBase := filepath.Base(filepath.FromSlash(req.OriginalRelPath))
	if naming.IsShortsExtension(origBase) {
		return EntryView{}, ErrShortsProtected
	}

	targetRel, err := s.PlanCollisionFreeRelPath(ctx, req.DiskID, req.MountPath, req.RootRelPath, req.Category, req.OriginalRelPath, nil)
	if err != nil {
		return EntryView{}, err
	}

	srcFull := library.ResolveFullPath(req.MountPath, req.OriginalRelPath)
	dstFull := library.ResolveFullPath(req.MountPath, targetRel)

	st, statErr := os.Stat(srcFull)
	if statErr != nil {
		return EntryView{}, fmt.Errorf("errorsbox: stat source %s: %w", srcFull, statErr)
	}
	if entryKind == "directory" {
		if !st.IsDir() {
			return EntryView{}, errors.New("errorsbox: expected directory for directory move")
		}
		entries, rErr := os.ReadDir(srcFull)
		if rErr != nil {
			return EntryView{}, rErr
		}
		if len(entries) > 0 {
			return EntryView{}, errors.New("errorsbox safety violation: cannot move non-empty directory as empty_folders")
		}
	}

	if err := fsops.VerifySafeDestination(req.Permit, srcFull, dstFull); err != nil {
		return EntryView{}, err
	}

	var sizeBytes int64
	var preHash string
	if entryKind == "file" {
		sizeBytes = st.Size()
		h, hErr := computeFileSHA256(srcFull)
		if hErr != nil {
			return EntryView{}, fmt.Errorf("errorsbox: pre-move sha256 failed: %w", hErr)
		}
		preHash = h
	}

	if err := os.MkdirAll(filepath.Dir(dstFull), 0o755); err != nil {
		return EntryView{}, err
	}
	if err := os.Rename(srcFull, dstFull); err != nil {
		return EntryView{}, fmt.Errorf("errorsbox: rename into error folder failed: %w", err)
	}

	if entryKind == "file" {
		postHash, phErr := computeFileSHA256(dstFull)
		if phErr != nil || postHash != preHash {
			_ = os.Rename(dstFull, srcFull)
			return EntryView{}, errors.New("errorsbox critical safety halt: post-move SHA-256 mismatch")
		}
	}

	nowStr := time.Now().UTC().Format(time.RFC3339Nano)
	entryID := domain.NewImmutableID()
	var rootNullable, fileNullable, batchNullable, journalNullable, hashNullable sql.NullString
	if req.RootID != "" {
		rootNullable = sql.NullString{String: req.RootID, Valid: true}
	}
	if req.FileID != "" {
		fileNullable = sql.NullString{String: req.FileID, Valid: true}
	}
	if req.Permit.BatchID != "" {
		batchNullable = sql.NullString{String: req.Permit.BatchID, Valid: true}
	}
	if req.JournalID != "" {
		journalNullable = sql.NullString{String: req.JournalID, Valid: true}
	}
	if preHash != "" {
		hashNullable = sql.NullString{String: preHash, Valid: true}
	}

	if _, err := s.sqlDB.ExecContext(ctx, `
		INSERT INTO error_folder_entries (
			id, disk_id, root_id, file_id, batch_id, journal_id, entry_kind,
			category_code, category_folder_ar, original_rel_path, error_rel_path,
			item_name, size_bytes, sha256_hash, reason_ar, reason_en, entry_status, moved_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'active', ?)
	`, entryID, req.DiskID, rootNullable, fileNullable, batchNullable, journalNullable, entryKind,
		string(spec.Code), spec.FolderAr, filepath.ToSlash(req.OriginalRelPath), targetRel,
		origBase, sizeBytes, hashNullable, req.ReasonAr, req.ReasonEn, nowStr); err != nil {
		// Roll back physical rename if DB record fails so disk & DB never diverge
		_ = os.Rename(dstFull, srcFull)
		return EntryView{}, fmt.Errorf("errorsbox: record entry failed: %w", err)
	}

	if req.FileID != "" && entryKind == "file" {
		newParent := filepath.ToSlash(filepath.Dir(filepath.FromSlash(targetRel)))
		newBase := filepath.Base(filepath.FromSlash(targetRel))
		_, _ = s.sqlDB.ExecContext(ctx, `
			UPDATE media_files
			SET relative_path = ?, parent_rel_path = ?, filename = ?,
			    presence_status = 'moved_to_errors', sha256_hash = COALESCE(?, sha256_hash), updated_at = ?
			WHERE id = ?
		`, targetRel, newParent, newBase, hashNullable, nowStr, req.FileID)
	}

		return EntryView{
			ID:               entryID,
			DiskID:           req.DiskID,
			RootID:           req.RootID,
			FileID:           req.FileID,
			BatchID:          req.Permit.BatchID,
			JournalID:        req.JournalID,
			EntryKind:        entryKind,
			CategoryCode:     spec.Code,
			CategoryFolderAr: spec.FolderAr,
			OriginalRelPath:  filepath.ToSlash(req.OriginalRelPath),
			OriginalFullPath: srcFull,
			ErrorRelPath:     targetRel,
			ErrorFullPath:    dstFull,
			ItemName:         origBase,
			SizeBytes:        sizeBytes,
			SHA256Hash:       preHash,
			ReasonAr:         req.ReasonAr,
			ReasonEn:         req.ReasonEn,
			EntryStatus:      "active",
			MovedAt:          nowStr,
		}, nil
	}

// RestoreEntry restores a previously quarantined file or folder back to its exact original path
// while verifying SHA-256 integrity and forbidding overwrites.
func (s *Service) RestoreEntry(ctx context.Context, entryID string) (EntryView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.restoreEntryLocked(ctx, strings.TrimSpace(entryID))
}

func (s *Service) restoreEntryLocked(ctx context.Context, entryID string) (EntryView, error) {
	var ev EntryView
	var mountPath string
	var rootID, fileID, batchID, batchTitle, journalID, shaHash, restoredAt sql.NullString
	var catCode string

	err := s.sqlDB.QueryRowContext(ctx, `
		SELECT e.id, e.disk_id, d.label, d.mount_path, e.root_id, e.file_id, e.batch_id, COALESCE(rb.title, ''), e.journal_id,
		       e.entry_kind, e.category_code, e.category_folder_ar, e.original_rel_path, e.error_rel_path,
		       e.item_name, e.size_bytes, e.sha256_hash, e.reason_ar, e.reason_en, e.entry_status, e.moved_at, e.restored_at
		FROM error_folder_entries e
		JOIN disks d ON d.id = e.disk_id
		LEFT JOIN review_batches rb ON rb.id = e.batch_id
		WHERE e.id = ?
	`, entryID).Scan(
		&ev.ID, &ev.DiskID, &ev.DiskLabel, &mountPath, &rootID, &fileID, &batchID, &batchTitle, &journalID,
		&ev.EntryKind, &catCode, &ev.CategoryFolderAr, &ev.OriginalRelPath, &ev.ErrorRelPath,
		&ev.ItemName, &ev.SizeBytes, &shaHash, &ev.ReasonAr, &ev.ReasonEn, &ev.EntryStatus, &ev.MovedAt, &restoredAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return EntryView{}, ErrEntryNotFound
	}
	if err != nil {
		return EntryView{}, err
	}
	ev.CategoryCode = CategoryCode(catCode)
	ev.RootID = rootID.String
	ev.FileID = fileID.String
	ev.BatchID = batchID.String
	ev.BatchTitle = batchTitle.String
	ev.JournalID = journalID.String
	ev.SHA256Hash = shaHash.String
	ev.RestoredAt = restoredAt.String

	curFull := library.ResolveFullPath(mountPath, ev.ErrorRelPath)
	origFull := library.ResolveFullPath(mountPath, ev.OriginalRelPath)
	ev.ErrorFullPath = curFull
	ev.OriginalFullPath = origFull

	if ev.EntryStatus == "restored" {
		return ev, nil
	}

	alreadyRecreatedDir := false
	if st, err := os.Stat(origFull); err == nil {
		if ev.EntryKind == "directory" && st.IsDir() {
			if curEntries, rErr := os.ReadDir(curFull); rErr == nil && len(curEntries) == 0 {
				alreadyRecreatedDir = true
			}
		}
		if !alreadyRecreatedDir {
			return EntryView{}, fmt.Errorf("%w: %s", ErrRestoreCollision, origFull)
		}
	}

	if ev.EntryKind == "file" && ev.SHA256Hash != "" {
		curHash, hErr := computeFileSHA256(curFull)
		if hErr != nil {
			return EntryView{}, fmt.Errorf("errorsbox: verify source before restore: %w", hErr)
		}
		if curHash != ev.SHA256Hash {
			return EntryView{}, errors.New("errorsbox safety error: SHA-256 mismatch before restore")
		}
	}

	if alreadyRecreatedDir {
		if err := os.Remove(curFull); err != nil {
			return EntryView{}, fmt.Errorf("errorsbox: clean evacuated dir marker failed: %w", err)
		}
	} else {
		if err := os.MkdirAll(filepath.Dir(origFull), 0o755); err != nil {
			return EntryView{}, err
		}
		if err := os.Rename(curFull, origFull); err != nil {
			return EntryView{}, fmt.Errorf("errorsbox: restore rename failed: %w", err)
		}
	}

	if ev.EntryKind == "file" && ev.SHA256Hash != "" {
		restoredHash, hErr := computeFileSHA256(origFull)
		if hErr != nil || restoredHash != ev.SHA256Hash {
			_ = os.Rename(origFull, curFull)
			return EntryView{}, errors.New("errorsbox safety error: SHA-256 mismatch after restore")
		}
	}

	nowStr := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.sqlDB.ExecContext(ctx, `
		UPDATE error_folder_entries SET entry_status = 'restored', restored_at = ? WHERE id = ?
	`, nowStr, ev.ID); err != nil {
		return EntryView{}, err
	}

	if ev.FileID != "" && ev.EntryKind == "file" {
		origParent := filepath.ToSlash(filepath.Dir(filepath.FromSlash(ev.OriginalRelPath)))
		if origParent == "." {
			origParent = ""
		}
		origBase := filepath.Base(filepath.FromSlash(ev.OriginalRelPath))
		_, _ = s.sqlDB.ExecContext(ctx, `
			UPDATE media_files
			SET relative_path = ?, parent_rel_path = ?, filename = ?, presence_status = 'present', updated_at = ?
			WHERE id = ?
		`, ev.OriginalRelPath, origParent, origBase, nowStr, ev.FileID)
	}

	if ev.JournalID != "" {
		_, _ = s.sqlDB.ExecContext(ctx, `UPDATE operation_journal SET operation_state = 'rolled_back' WHERE id = ?`, ev.JournalID)
		_, _ = s.sqlDB.ExecContext(ctx, `UPDATE rollback_steps SET rollback_status = 'executed', executed_at = ? WHERE error_entry_id = ? AND rollback_status = 'ready'`, nowStr, ev.ID)
	}

	ev.EntryStatus = "restored"
	ev.RestoredAt = nowStr
	return ev, nil
}

// GetEntry returns full inspection metadata for a single Error Center item.
func (s *Service) GetEntry(ctx context.Context, entryID string) (EntryView, error) {
	entries, err := s.ListEntries(ctx, "", "")
	if err != nil {
		return EntryView{}, err
	}
	target := strings.TrimSpace(entryID)
	for _, ev := range entries {
		if ev.ID == target {
			return ev, nil
		}
	}
	return EntryView{}, ErrEntryNotFound
}

// RestoreSelectedEntries restores multiple user-selected Error Center entries safely in order
// (empty directories first so any original parent directories exist cleanly, then files).
func (s *Service) RestoreSelectedEntries(ctx context.Context, entryIDs []string) (BatchRestoreSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	summary := BatchRestoreSummary{
		RestoredItems: []EntryView{},
		Errors:        []string{},
	}
	seen := map[string]bool{}
	var cleanIDs []string
	for _, id := range entryIDs {
		trimmed := strings.TrimSpace(id)
		if trimmed != "" && !seen[trimmed] {
			seen[trimmed] = true
			cleanIDs = append(cleanIDs, trimmed)
		}
	}
	if len(cleanIDs) == 0 {
		summary.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
		return summary, nil
	}

	placeholders := make([]string, len(cleanIDs))
	args := make([]any, len(cleanIDs))
	for i, id := range cleanIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	rows, err := s.sqlDB.QueryContext(ctx, fmt.Sprintf(`
		SELECT id FROM error_folder_entries
		WHERE id IN (%s)
		ORDER BY CASE entry_kind WHEN 'directory' THEN 0 ELSE 1 END ASC, moved_at DESC
	`, strings.Join(placeholders, ",")), args...)
	var orderedIDs []string
	if err == nil {
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil {
				orderedIDs = append(orderedIDs, id)
			}
		}
		rows.Close()
	}
	if len(orderedIDs) == 0 {
		orderedIDs = cleanIDs
	}

	for _, id := range orderedIDs {
		ev, rErr := s.restoreEntryLocked(ctx, id)
		if rErr != nil {
			summary.BlockedCount++
			summary.Errors = append(summary.Errors, fmt.Sprintf("%s: %v", id, rErr))
			continue
		}
		summary.RestoredCount++
		summary.RestoredItems = append(summary.RestoredItems, ev)
	}
	summary.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return summary, nil
}

// RestoreBatchEntries restores all active quarantined Error Center entries belonging to a reversible batch.
func (s *Service) RestoreBatchEntries(ctx context.Context, batchID string) (BatchRestoreSummary, error) {
	rows, err := s.sqlDB.QueryContext(ctx, `
		SELECT id FROM error_folder_entries
		WHERE batch_id = ? AND entry_status = 'active'
		ORDER BY CASE entry_kind WHEN 'directory' THEN 0 ELSE 1 END ASC, moved_at DESC
	`, strings.TrimSpace(batchID))
	if err != nil {
		return BatchRestoreSummary{}, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	if len(ids) == 0 {
		return BatchRestoreSummary{
			RestoredItems: []EntryView{},
			Errors:        []string{},
			CompletedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		}, nil
	}
	return s.RestoreSelectedEntries(ctx, ids)
}

// ListEntries returns tracked items in `الأخطاء/`, optionally filtered by category and status ("active", "restored", or "" for all).
func (s *Service) ListEntries(ctx context.Context, category CategoryCode, status string) ([]EntryView, error) {
	q := `
		SELECT e.id, e.disk_id, d.label, d.mount_path, COALESCE(e.root_id, ''), COALESCE(e.file_id, ''),
		       COALESCE(e.batch_id, ''), COALESCE(rb.title, ''), COALESCE(e.journal_id, ''),
		       e.entry_kind, e.category_code, e.category_folder_ar, e.original_rel_path, e.error_rel_path,
		       e.item_name, e.size_bytes, COALESCE(e.sha256_hash, ''), e.reason_ar, e.reason_en, e.entry_status, e.moved_at, COALESCE(e.restored_at, '')
		FROM error_folder_entries e
		JOIN disks d ON d.id = e.disk_id
		LEFT JOIN review_batches rb ON rb.id = e.batch_id
		WHERE 1=1
	`
	args := []any{}
	if category != "" {
		q += ` AND e.category_code = ?`
		args = append(args, string(category))
	}
	if status != "" {
		q += ` AND e.entry_status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY e.moved_at DESC`

	rows, err := s.sqlDB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []EntryView{}
	for rows.Next() {
		var ev EntryView
		var mountPath, catStr string
		if err := rows.Scan(
			&ev.ID, &ev.DiskID, &ev.DiskLabel, &mountPath, &ev.RootID, &ev.FileID,
			&ev.BatchID, &ev.BatchTitle, &ev.JournalID,
			&ev.EntryKind, &catStr, &ev.CategoryFolderAr, &ev.OriginalRelPath, &ev.ErrorRelPath,
			&ev.ItemName, &ev.SizeBytes, &ev.SHA256Hash, &ev.ReasonAr, &ev.ReasonEn, &ev.EntryStatus, &ev.MovedAt, &ev.RestoredAt,
		); err != nil {
			return nil, err
		}
		ev.CategoryCode = CategoryCode(catStr)
		ev.OriginalFullPath = library.ResolveFullPath(mountPath, ev.OriginalRelPath)
		ev.ErrorFullPath = library.ResolveFullPath(mountPath, ev.ErrorRelPath)
		out = append(out, ev)
	}
	return out, rows.Err()
}

func sanitizeSegmentPreservingExt(name string) string {
	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case '<', '>', ':', '"', '/', '\\', '|', '?', '*':
			return '_'
		default:
			if r < 32 {
				return '_'
			}
			return r
		}
	}, strings.TrimSpace(name))
	if cleaned == "" {
		return "unnamed_item"
	}
	return cleaned
}

func computeFileSHA256(path string) (string, error) {
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
