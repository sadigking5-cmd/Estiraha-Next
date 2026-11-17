package integration_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/a73971054-lab/Estiraha-Next/internal/db"
	"github.com/a73971054-lab/Estiraha-Next/internal/domain"
	"github.com/a73971054-lab/Estiraha-Next/internal/naming"
)

func TestSQLiteWALMigrationsForeignKeysAndBilingualFTS5(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	tmpDir := t.TempDir()
	dbFile := filepath.Join(tmpDir, "estiraha_test.db")
	backupDir := filepath.Join(tmpDir, "backups")

	engine, err := db.Open(ctx, dbFile)
	if err != nil {
		t.Fatalf("open file sqlite failed: %v", err)
	}
	defer engine.Close()

	ver, err := engine.Migrate(ctx, backupDir)
	if err != nil {
		t.Fatalf("migrate failed: %v", err)
	}
	if ver != 2 {
		t.Fatalf("expected schema version 2, got %d", ver)
	}

	// Re-running Migrate must be strictly idempotent
	ver2, err := engine.Migrate(ctx, backupDir)
	if err != nil || ver2 != 2 {
		t.Fatalf("idempotent re-migrate failed: ver=%d err=%v", ver2, err)
	}

	health, err := engine.HealthCheck(ctx)
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	if health.JournalMode != "wal" {
		t.Fatalf("expected WAL journal mode, got %q", health.JournalMode)
	}
	if !health.ForeignKeys {
		t.Fatal("expected PRAGMA foreign_keys = ON")
	}
	if !health.FTS5Enabled {
		t.Fatal("expected SQLite FTS5 virtual tables to be operational")
	}

	sqlDB := engine.SQL()

	// Verify 9 default API Hub providers seeded in safe state
	var providerCount int
	if err := sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM data_providers`).Scan(&providerCount); err != nil || providerCount != 9 {
		t.Fatalf("expected 9 seeded providers, got %d (err=%v)", providerCount, err)
	}

	// Verify Foreign Key enforcement: inserting library_root for non-existent disk MUST fail
	_, fkErr := sqlDB.ExecContext(ctx, `
		INSERT INTO library_roots (id, disk_id, relative_root_path, content_category, display_name)
		VALUES ('root-bad', 'non-existent-disk', 'Movies', 'movies', 'Movies')
	`)
	if fkErr == nil {
		t.Fatal("expected foreign key constraint failure for invalid disk_id, got nil")
	}

	// Verify Immutable ID persistence across file rename & bilingual FTS5 search
	diskID := domain.NewImmutableID()
	workArID := domain.NewImmutableID()
	workEnID := domain.NewImmutableID()
	personID := domain.NewImmutableID()
	fileID := domain.NewImmutableID()

	if _, err := sqlDB.ExecContext(ctx, `
		INSERT INTO disks (id, volume_serial, label, mount_path, total_bytes, free_bytes)
		VALUES (?, 'VOL-TEST-0001', 'CinemaVault1', 'D:\', 8000000000000, 2500000000000)
	`, diskID); err != nil {
		t.Fatalf("insert disk failed: %v", err)
	}

	movieFolderAr, _ := naming.FormatMovieName("الناظر", 2000)
	if _, err := sqlDB.ExecContext(ctx, `
		INSERT INTO works (id, media_type, sub_format, title_original, title_ar, title_en, is_arabic_origin, production_year, canonical_folder_name, identification_status)
		VALUES (?, 'movie', 'standalone', 'الناظر', 'الناظر', 'El Nazer', 1, 2000, ?, 'identified')
	`, workArID, movieFolderAr); err != nil {
		t.Fatalf("insert Arabic movie failed: %v", err)
	}

	movieFolderEn, _ := naming.FormatMovieName("The Dark Knight", 2008)
	if _, err := sqlDB.ExecContext(ctx, `
		INSERT INTO works (id, media_type, sub_format, title_original, title_ar, title_en, is_arabic_origin, production_year, canonical_folder_name, identification_status)
		VALUES (?, 'movie', 'standalone', 'The Dark Knight', 'فارس الظلام', 'The Dark Knight', 0, 2008, ?, 'identified')
	`, workEnID, movieFolderEn); err != nil {
		t.Fatalf("insert English movie failed: %v", err)
	}

	celebCanonical, _ := naming.FormatCelebrityFolder("أحمد زكي", "Ahmed Zaki")
	if _, err := sqlDB.ExecContext(ctx, `
		INSERT INTO people (id, name_ar, name_en, name_original, canonical_display_name)
		VALUES (?, 'أحمد زكي', 'Ahmed Zaki', 'أحمد زكي', ?)
	`, personID, celebCanonical); err != nil {
		t.Fatalf("insert celebrity failed: %v", err)
	}

	// Insert physical file and simulate approved rename: verify file UUID never changes
	if _, err := sqlDB.ExecContext(ctx, `
		INSERT INTO media_files (id, disk_id, work_id, relative_path, parent_rel_path, filename, extension, file_kind, size_bytes, mtime_unix, sha256_hash)
		VALUES (?, ?, ?, 'Movies/raw.dark.knight.mkv', 'Movies', 'raw.dark.knight.mkv', '.mkv', 'video', 4294967296, 1700000000, 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855')
	`, fileID, diskID, workEnID); err != nil {
		t.Fatalf("insert media_file failed: %v", err)
	}

	if _, err := sqlDB.ExecContext(ctx, `
		UPDATE media_files
		SET relative_path = 'Movies/The Dark Knight 2008/The Dark Knight 2008.mkv',
		    parent_rel_path = 'Movies/The Dark Knight 2008',
		    filename = 'The Dark Knight 2008.mkv'
		WHERE id = ?
	`, fileID); err != nil {
		t.Fatalf("update renamed path failed: %v", err)
	}

	var persistedID string
	if err := sqlDB.QueryRowContext(ctx, `SELECT id FROM media_files WHERE filename = 'The Dark Knight 2008.mkv'`).Scan(&persistedID); err != nil || persistedID != fileID {
		t.Fatalf("immutable file ID changed or missing: got %q want %q (err=%v)", persistedID, fileID, err)
	}

	// Test FTS5 Arabic search (including diacritic-normalized query 'احمد' matching 'أحمد زكي')
	var matchedWorkID string
	if err := sqlDB.QueryRowContext(ctx, `SELECT work_id FROM works_fts WHERE works_fts MATCH 'الناظر'`).Scan(&matchedWorkID); err != nil || matchedWorkID != workArID {
		t.Fatalf("FTS5 Arabic work query failed: got=%q want=%q err=%v", matchedWorkID, workArID, err)
	}

	var matchedEnByArabicTitle string
	if err := sqlDB.QueryRowContext(ctx, `SELECT work_id FROM works_fts WHERE works_fts MATCH '"فارس الظلام"'`).Scan(&matchedEnByArabicTitle); err != nil || matchedEnByArabicTitle != workEnID {
		t.Fatalf("FTS5 bilingual Arabic-to-foreign movie search failed: got=%q want=%q err=%v", matchedEnByArabicTitle, workEnID, err)
	}

	var matchedPersonID string
	if err := sqlDB.QueryRowContext(ctx, `SELECT person_id FROM people_fts WHERE people_fts MATCH 'احمد'`).Scan(&matchedPersonID); err != nil || matchedPersonID != personID {
		t.Fatalf("FTS5 Arabic diacritic-normalized celebrity query failed: got=%q want=%q err=%v", matchedPersonID, personID, err)
	}
}
