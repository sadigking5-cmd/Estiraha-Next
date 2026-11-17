// Package db provides the local SQLite database engine, PRAGMA hardening,
// pre-migration SHA-256 backup safety, atomic migration runner, and FTS5 verification.
package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sadigking5-cmd/Estiraha-Next/migrations"
	_ "github.com/mattn/go-sqlite3"
)

// Engine wraps the SQLite connection pool and enforces Estiraha Next persistence invariants.
type Engine struct {
	sqlDB  *sql.DB
	dbPath string
}

// HealthReport summarizes verified runtime SQLite capabilities.
type HealthReport struct {
	JournalMode     string
	ForeignKeys     bool
	FTS5Enabled     bool
	SchemaVersion   int
	RegisteredTables int
}

// Open initializes a hardened SQLite connection (WAL mode, Foreign Keys ON, busy timeout).
// Pass ":memory:" for isolated unit tests or an absolute AppData file path for local storage.
func Open(ctx context.Context, dbPath string) (*Engine, error) {
	if dbPath != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(dbPath), 0o750); err != nil {
			return nil, fmt.Errorf("create db directory: %w", err)
		}
	}

	dsn := dbPath
	if dbPath != ":memory:" {
		dsn = fmt.Sprintf("file:%s?_busy_timeout=10000&_foreign_keys=on&_journal_mode=WAL&_sync=NORMAL", filepath.ToSlash(dbPath))
	}

	conn, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if dbPath == ":memory:" {
		// Single connection for in-memory DB so PRAGMAs and tables persist across queries in tests.
		conn.SetMaxOpenConns(1)
	} else {
		conn.SetMaxOpenConns(4)
	}

	pragmas := []string{
		"PRAGMA foreign_keys = ON;",
		"PRAGMA busy_timeout = 10000;",
		"PRAGMA synchronous = NORMAL;",
	}
	if dbPath != ":memory:" {
		pragmas = append([]string{"PRAGMA journal_mode = WAL;"}, pragmas...)
	}

	for _, p := range pragmas {
		if _, err := conn.ExecContext(ctx, p); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("apply pragma %q: %w", p, err)
		}
	}

	return &Engine{sqlDB: conn, dbPath: dbPath}, nil
}

// SQL returns the underlying *sql.DB handle for repositories and transactions.
func (e *Engine) SQL() *sql.DB {
	return e.sqlDB
}

// Close cleanly closes the SQLite database connection.
func (e *Engine) Close() error {
	if e == nil || e.sqlDB == nil {
		return nil
	}
	return e.sqlDB.Close()
}

// Migrate applies all pending embedded SQL migrations atomically.
// If backupDir is non-empty and a non-empty database file already exists with prior migrations,
// a verified SHA-256 pre-migration backup copy is created before applying new migrations.
func (e *Engine) Migrate(ctx context.Context, backupDir string) (int, error) {
	const createMigrationsTable = `
	CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		filename TEXT NOT NULL UNIQUE,
		checksum_sha256 TEXT NOT NULL,
		applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
	);`

	if _, err := e.sqlDB.ExecContext(ctx, createMigrationsTable); err != nil {
		return 0, fmt.Errorf("init schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		return 0, fmt.Errorf("read embedded migrations: %w", err)
	}

	type migrationFile struct {
		version  int
		filename string
		content  string
		checksum string
	}

	var files []migrationFile
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		parts := strings.SplitN(entry.Name(), "_", 2)
		ver, err := strconv.Atoi(parts[0])
		if err != nil {
			return 0, fmt.Errorf("invalid migration filename %q: %w", entry.Name(), err)
		}
		raw, err := fs.ReadFile(migrations.FS, entry.Name())
		if err != nil {
			return 0, fmt.Errorf("read migration %q: %w", entry.Name(), err)
		}
		sum := sha256.Sum256(raw)
		files = append(files, migrationFile{
			version:  ver,
			filename: entry.Name(),
			content:  string(raw),
			checksum: hex.EncodeToString(sum[:]),
		})
	}

	sort.Slice(files, func(i, j int) bool { return files[i].version < files[j].version })

	applied := make(map[int]string)
	rows, err := e.sqlDB.QueryContext(ctx, `SELECT version, checksum_sha256 FROM schema_migrations`)
	if err != nil {
		return 0, fmt.Errorf("query applied migrations: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var v int
		var sum string
		if err := rows.Scan(&v, &sum); err != nil {
			return 0, err
		}
		applied[v] = sum
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	pendingCount := 0
	for _, mf := range files {
		if existingSum, ok := applied[mf.version]; ok {
			if existingSum != mf.checksum {
				return 0, fmt.Errorf("migration %d (%s) checksum mismatch: applied=%s current=%s", mf.version, mf.filename, existingSum, mf.checksum)
			}
			continue
		}
		pendingCount++
	}

	// Pre-migration automatic backup if upgrading an existing populated file DB
	if pendingCount > 0 && len(applied) > 0 && e.dbPath != ":memory:" && backupDir != "" {
		if _, _, err := e.createFileBackup(backupDir); err != nil {
			return 0, fmt.Errorf("pre-migration backup failed (aborting migration for safety): %w", err)
		}
	}

	latestVersion := 0
	for _, mf := range files {
		if mf.version > latestVersion {
			latestVersion = mf.version
		}
		if _, already := applied[mf.version]; already {
			continue
		}

		tx, err := e.sqlDB.BeginTx(ctx, nil)
		if err != nil {
			return 0, fmt.Errorf("begin tx for migration %s: %w", mf.filename, err)
		}
		if _, err := tx.ExecContext(ctx, mf.content); err != nil {
			_ = tx.Rollback()
			return 0, fmt.Errorf("execute migration %s: %w", mf.filename, err)
		}
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO schema_migrations (version, filename, checksum_sha256) VALUES (?, ?, ?)`,
			mf.version, mf.filename, mf.checksum,
		); err != nil {
			_ = tx.Rollback()
			return 0, fmt.Errorf("record migration %s: %w", mf.filename, err)
		}
		if err := tx.Commit(); err != nil {
			return 0, fmt.Errorf("commit migration %s: %w", mf.filename, err)
		}
	}

	return latestVersion, nil
}

func (e *Engine) createFileBackup(backupDir string) (string, string, error) {
	if err := os.MkdirAll(backupDir, 0o750); err != nil {
		return "", "", err
	}
	src, err := os.Open(e.dbPath)
	if err != nil {
		return "", "", err
	}
	defer src.Close()

	stamp := time.Now().UTC().Format("20060102T150405Z")
	dstPath := filepath.Join(backupDir, fmt.Sprintf("estiraha_premig_%s.db.bak", stamp))
	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return "", "", err
	}
	defer dst.Close()

	hasher := sha256.New()
	if _, err := io.Copy(io.MultiWriter(dst, hasher), src); err != nil {
		return "", "", err
	}
	if err := dst.Sync(); err != nil {
		return "", "", err
	}
	return dstPath, hex.EncodeToString(hasher.Sum(nil)), nil
}

// HealthCheck verifies PRAGMA enforcement, schema version, and FTS5 operational readiness.
func (e *Engine) HealthCheck(ctx context.Context) (HealthReport, error) {
	var report HealthReport
	if err := e.sqlDB.QueryRowContext(ctx, `PRAGMA journal_mode;`).Scan(&report.JournalMode); err != nil {
		return report, err
	}
	var fk int
	if err := e.sqlDB.QueryRowContext(ctx, `PRAGMA foreign_keys;`).Scan(&fk); err != nil {
		return report, err
	}
	report.ForeignKeys = fk == 1

	_ = e.sqlDB.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations;`).Scan(&report.SchemaVersion)
	if err := e.sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%';`).Scan(&report.RegisteredTables); err != nil {
		return report, err
	}

	// Probe FTS5 virtual table availability
	var dummy int
	if err := e.sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM works_fts;`).Scan(&dummy); err == nil {
		report.FTS5Enabled = true
	}
	return report, nil
}
