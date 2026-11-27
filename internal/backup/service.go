// Package backup implements Estiraha Next's verified "Both" Backup Policy:
//  1. Verified SQLite snapshot via `VACUUM INTO` + read-only `PRAGMA integrity_check` + `SHA-256`
//  2. Reversible operation/plan manifest (`.manifest.json`) storing library roots, approved review
//     batches, and before/after journal entries so file paths can be rolled back safely.
//
// Explicit Scope Boundary: Database backups protect index/metadata/watch-states/journals;
// they are NOT a substitute for original media file storage and NEVER mass-copy media without consent.
package backup

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
	"time"

	"github.com/sadigking5-cmd/Estiraha-Next/internal/domain"
)

type BackupRecord struct {
	ID                 string `json:"id"`
	BackupKind         string `json:"backupKind"`
	ArchivePath        string `json:"archivePath"`
	ManifestPath       string `json:"manifestPath"`
	SizeBytes          int64  `json:"sizeBytes"`
	SHA256Checksum     string `json:"sha256Checksum"`
	SchemaVersion      int    `json:"schemaVersion"`
	VerificationStatus string `json:"verificationStatus"`
	ScopeNoticeAr      string `json:"scopeNoticeAr"`
	ScopeNoticeEn      string `json:"scopeNoticeEn"`
	CreatedAt          string `json:"createdAt"`
	VerifiedAt         string `json:"verifiedAt"`
}

type PortableManifest struct {
	BackupID       string            `json:"backupId"`
	CreatedAtUTC   string            `json:"createdAtUtc"`
	SchemaVersion  int               `json:"schemaVersion"`
	DBSHA256       string            `json:"dbSha256"`
	ScopeBoundary  string            `json:"scopeBoundary"`
	LibraryRoots   []ManifestRoot    `json:"libraryRoots"`
	RecentJournals []ManifestJournal `json:"recentJournals"`
}

type ManifestRoot struct {
	RootID       string `json:"rootId"`
	DiskID       string `json:"diskId"`
	MountPath    string `json:"mountPath"`
	RelativePath string `json:"relativePath"`
	Category     string `json:"category"`
}

type ManifestJournal struct {
	JournalID  string `json:"journalId"`
	BatchID    string `json:"batchId"`
	State      string `json:"state"`
	SourcePath string `json:"sourcePath"`
	DestPath   string `json:"destPath"`
	PreSHA256  string `json:"preSha256"`
	PostSHA256 string `json:"postSha256"`
}

type Service struct {
	sqlDB     *sql.DB
	backupDir string
}

func NewService(sqlDB *sql.DB, backupDir string) *Service {
	return &Service{
		sqlDB:     sqlDB,
		backupDir: backupDir,
	}
}

func (s *Service) BackupDir() string {
	return s.backupDir
}

func (s *Service) CreateVerifiedBackup(ctx context.Context, kind string) (BackupRecord, error) {
	if kind == "" {
		kind = "manual_user"
	}
	switch kind {
	case "auto_scheduled", "pre_migration", "pre_batch", "manual_user":
	default:
		return BackupRecord{}, fmt.Errorf("backup: invalid backup kind %q", kind)
	}
	if s.backupDir == "" {
		return BackupRecord{}, errors.New("backup: backup directory is not configured")
	}
	if err := os.MkdirAll(s.backupDir, 0o755); err != nil {
		return BackupRecord{}, fmt.Errorf("backup: mkdir %s: %w", s.backupDir, err)
	}

	id := domain.NewImmutableID()
	ts := time.Now().UTC().Format("20060102-150405.000")
	idSuffix := id
	if len(id) >= 12 {
		idSuffix = id[len(id)-12:]
	}
	snapName := fmt.Sprintf("estiraha-%s-%s-%s.sqlite", kind, ts, idSuffix)
	snapPath := filepath.Join(s.backupDir, snapName)
	escapedSnap := strings.ReplaceAll(snapPath, "'", "''")

	if _, err := s.sqlDB.ExecContext(ctx, fmt.Sprintf("VACUUM INTO '%s';", escapedSnap)); err != nil {
		return BackupRecord{}, fmt.Errorf("backup: vacuum into failed: %w", err)
	}

	if err := VerifySQLiteSnapshot(ctx, snapPath); err != nil {
		_ = os.Remove(snapPath)
		return BackupRecord{}, err
	}

	hashHex, sizeBytes, err := computeFileSHA256(snapPath)
	if err != nil {
		_ = os.Remove(snapPath)
		return BackupRecord{}, err
	}

	var schemaVer int
	_ = s.sqlDB.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&schemaVer)

	manifest, err := s.buildManifest(ctx, id, schemaVer, hashHex)
	if err != nil {
		return BackupRecord{}, err
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return BackupRecord{}, err
	}
	manifestPath := snapPath + ".manifest.json"
	if err := os.WriteFile(manifestPath, manifestBytes, 0o600); err != nil {
		return BackupRecord{}, fmt.Errorf("backup: write manifest failed: %w", err)
	}

	nowStr := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.sqlDB.ExecContext(ctx, `
		INSERT INTO backup_records (
			id, backup_kind, archive_path, size_bytes, sha256_checksum, schema_version,
			includes_settings, includes_watch_state, includes_audit_journal,
			verification_status, portable_manifest_json, created_at, verified_at
		) VALUES (?, ?, ?, ?, ?, ?, 1, 1, 1, 'verified', ?, ?, ?)
	`, id, kind, snapPath, sizeBytes, hashHex, schemaVer, string(manifestBytes), nowStr, nowStr)
	if err != nil {
		return BackupRecord{}, fmt.Errorf("backup: insert record failed: %w", err)
	}

	_, _ = s.sqlDB.ExecContext(ctx, `
		INSERT INTO audit_logs (id, event_category, severity, actor, summary, details_json)
		VALUES (?, 'backup', 'info', 'user', ?, ?)
	`, domain.NewImmutableID(), fmt.Sprintf("Verified %s backup created (%d bytes)", kind, sizeBytes), string(manifestBytes))

	return BackupRecord{
		ID:                 id,
		BackupKind:         kind,
		ArchivePath:        snapPath,
		ManifestPath:       manifestPath,
		SizeBytes:          sizeBytes,
		SHA256Checksum:     hashHex,
		SchemaVersion:      schemaVer,
		VerificationStatus: "verified",
		ScopeNoticeAr:      "النسخة الاحتياطية تحمي قاعدة البيانات وسجل التراجع وحالات المشاهدة، وليست بديلاً عن حفظ ملفات الفيديو الأصلية ولا تنسخ الوسائط بدون إذنك.",
		ScopeNoticeEn:      "This backup protects the SQLite index, watch states, and reversible operation journal; it does NOT copy original media files without consent.",
		CreatedAt:          nowStr,
		VerifiedAt:         nowStr,
	}, nil
}

func (s *Service) VerifyBackupRecord(ctx context.Context, backupID string) (BackupRecord, error) {
	var rec BackupRecord
	var manifestJSON string
	err := s.sqlDB.QueryRowContext(ctx, `
		SELECT id, backup_kind, archive_path, size_bytes, sha256_checksum, schema_version,
		       verification_status, portable_manifest_json, created_at, COALESCE(verified_at, '')
		FROM backup_records WHERE id = ?
	`, backupID).Scan(
		&rec.ID, &rec.BackupKind, &rec.ArchivePath, &rec.SizeBytes, &rec.SHA256Checksum,
		&rec.SchemaVersion, &rec.VerificationStatus, &manifestJSON, &rec.CreatedAt, &rec.VerifiedAt,
	)
	if err != nil {
		return BackupRecord{}, err
	}
	rec.ManifestPath = rec.ArchivePath + ".manifest.json"
	rec.ScopeNoticeAr = "النسخة الاحتياطية تحمي قاعدة البيانات وسجل التراجع وحالات المشاهدة، وليست بديلاً عن حفظ ملفات الفيديو الأصلية."
	rec.ScopeNoticeEn = "Backup protects SQLite index, watch states, and rollback journal; original media files remain untouched."

	status := "verified"
	nowStr := time.Now().UTC().Format(time.RFC3339Nano)
	if _, statErr := os.Stat(rec.ArchivePath); statErr != nil {
		status = "missing_file"
	} else {
		actualHash, _, hashErr := computeFileSHA256(rec.ArchivePath)
		if hashErr != nil || !strings.EqualFold(actualHash, rec.SHA256Checksum) {
			status = "corrupted"
		} else if integErr := VerifySQLiteSnapshot(ctx, rec.ArchivePath); integErr != nil {
			status = "corrupted"
		}
	}

	rec.VerificationStatus = status
	rec.VerifiedAt = nowStr
	_, _ = s.sqlDB.ExecContext(ctx, `
		UPDATE backup_records SET verification_status = ?, verified_at = ? WHERE id = ?
	`, status, nowStr, backupID)

	if status != "verified" {
		return rec, fmt.Errorf("backup verification failed with status=%s", status)
	}
	return rec, nil
}

func (s *Service) ListBackups(ctx context.Context) ([]BackupRecord, error) {
	rows, err := s.sqlDB.QueryContext(ctx, `
		SELECT id, backup_kind, archive_path, size_bytes, sha256_checksum, schema_version,
		       verification_status, created_at, COALESCE(verified_at, '')
		FROM backup_records
		ORDER BY created_at DESC
		LIMIT 100
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []BackupRecord{}
	for rows.Next() {
		var r BackupRecord
		if err := rows.Scan(
			&r.ID, &r.BackupKind, &r.ArchivePath, &r.SizeBytes, &r.SHA256Checksum,
			&r.SchemaVersion, &r.VerificationStatus, &r.CreatedAt, &r.VerifiedAt,
		); err != nil {
			return nil, err
		}
		r.ManifestPath = r.ArchivePath + ".manifest.json"
		r.ScopeNoticeAr = "نسخة موثقة لقاعدة البيانات وسجل العمليات القابلة للتراجع (لا تشمل نسخ ملفات الفيديو الضخمة)."
		r.ScopeNoticeEn = "Verified database & reversible operation journal snapshot (does not duplicate raw video files)."
		out = append(out, r)
	}
	return out, rows.Err()
}

func VerifySQLiteSnapshot(ctx context.Context, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("backup: stat snapshot: %w", err)
	}
	if info.Size() < 4096 {
		return errors.New("backup: snapshot is unexpectedly small or truncated")
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	header := make([]byte, 16)
	_, readErr := io.ReadFull(f, header)
	_ = f.Close()
	if readErr != nil || string(header[:15]) != "SQLite format 3" {
		return errors.New("backup: invalid SQLite format 3 header")
	}

	roDB, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?mode=ro", filepath.ToSlash(path)))
	if err != nil {
		return err
	}
	defer roDB.Close()

	var checkResult string
	if err := roDB.QueryRowContext(ctx, "PRAGMA integrity_check;").Scan(&checkResult); err != nil {
		return fmt.Errorf("backup: integrity_check query failed: %w", err)
	}
	if strings.ToLower(strings.TrimSpace(checkResult)) != "ok" {
		return fmt.Errorf("backup: integrity_check failed: %s", checkResult)
	}
	return nil
}

func (s *Service) buildManifest(ctx context.Context, backupID string, schemaVer int, dbHash string) (PortableManifest, error) {
	m := PortableManifest{
		BackupID:       backupID,
		CreatedAtUTC:   time.Now().UTC().Format(time.RFC3339),
		SchemaVersion:  schemaVer,
		DBSHA256:       dbHash,
		ScopeBoundary:  "Protects SQLite library catalog, watch progress, settings, and reversible file operation journal. Does NOT copy user media files.",
		LibraryRoots:   []ManifestRoot{},
		RecentJournals: []ManifestJournal{},
	}

	rRows, err := s.sqlDB.QueryContext(ctx, `
		SELECT r.id, r.disk_id, d.mount_path, r.relative_root_path, r.content_category
		FROM library_roots r
		JOIN disks d ON d.id = r.disk_id
		ORDER BY r.created_at ASC
	`)
	if err == nil {
		defer rRows.Close()
		for rRows.Next() {
			var mr ManifestRoot
			if err := rRows.Scan(&mr.RootID, &mr.DiskID, &mr.MountPath, &mr.RelativePath, &mr.Category); err == nil {
				m.LibraryRoots = append(m.LibraryRoots, mr)
			}
		}
	}

	jRows, err := s.sqlDB.QueryContext(ctx, `
		SELECT id, batch_id, operation_state, source_snapshot_path, destination_snapshot_path,
		       COALESCE(pre_sha256, ''), COALESCE(post_sha256, '')
		FROM operation_journal
		ORDER BY started_at DESC
		LIMIT 250
	`)
	if err == nil {
		defer jRows.Close()
		for jRows.Next() {
			var mj ManifestJournal
			if err := jRows.Scan(&mj.JournalID, &mj.BatchID, &mj.State, &mj.SourcePath, &mj.DestPath, &mj.PreSHA256, &mj.PostSHA256); err == nil {
				m.RecentJournals = append(m.RecentJournals, mj)
			}
		}
	}
	return m, nil
}

func computeFileSHA256(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}
