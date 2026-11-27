package library

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/sadigking5-cmd/Estiraha-Next/internal/domain"
)

var (
	ErrDuplicateRootPath = errors.New("library: root path is already registered or overlaps an existing root")
	ErrInvalidCategory   = errors.New("library: unsupported content category")
	ErrEmptyRootPath     = errors.New("library: folder path cannot be empty")
)

type DiskView struct {
	ID              string `json:"id"`
	VolumeSerial    string `json:"volumeSerial"`
	Label           string `json:"label"`
	MountPath       string `json:"mountPath"`
	FilesystemType  string `json:"filesystemType"`
	TotalBytes      int64  `json:"totalBytes"`
	FreeBytes       int64  `json:"freeBytes"`
	ConnectionState string `json:"connectionState"`
	RootCount       int    `json:"rootCount"`
	MediaFileCount  int    `json:"mediaFileCount"`
	LastSeenAt      string `json:"lastSeenAt"`
	LastScanAt      string `json:"lastScanAt"`
}

type RootView struct {
	ID                 string `json:"id"`
	DiskID             string `json:"diskId"`
	DiskLabel          string `json:"diskLabel"`
	MountPath          string `json:"mountPath"`
	RelativeRootPath   string `json:"relativeRootPath"`
	FullPath           string `json:"fullPath"`
	ContentCategory    string `json:"contentCategory"`
	DisplayName        string `json:"displayName"`
	ScanEnabled        bool   `json:"scanEnabled"`
	WatchEnabled       bool   `json:"watchEnabled"`
	IsAccessible       bool   `json:"isAccessible"`
	PassiveChangeAlert bool   `json:"passiveChangeAlert"`
	IndexedFileCount   int    `json:"indexedFileCount"`
	LastScanAt         string `json:"lastScanAt"`
}

type AddRootInput struct {
	FullPath        string `json:"fullPath"`
	ContentCategory string `json:"contentCategory"`
	DisplayName     string `json:"displayName"`
	DiskLabel       string `json:"diskLabel"`
}

type Service struct {
	sqlDB *sql.DB
}

func NewService(sqlDB *sql.DB) *Service {
	return &Service{sqlDB: sqlDB}
}

func validCategory(cat string) bool {
	switch cat {
	case "movies", "series", "programs", "documentaries", "anime", "cartoons", "celebrities", "franchises", "mixed", "errors":
		return true
	default:
		return false
	}
}

func splitMountAndRelative(cleanFull string) (mountPath string, relPath string, volKey string) {
	vol := filepath.VolumeName(cleanFull)
	if vol != "" {
		mountPath = vol + `\`
		trimmed := strings.TrimPrefix(cleanFull, vol)
		trimmed = strings.TrimLeft(trimmed, `\/`)
		relPath = filepath.ToSlash(trimmed)
		if relPath == "" {
			relPath = "."
		}
		volKey = "WINVOL-" + strings.ToUpper(strings.TrimSuffix(vol, ":"))
		return mountPath, relPath, volKey
	}
	parent := filepath.Dir(cleanFull)
	base := filepath.Base(cleanFull)
	if parent == "" || parent == "." {
		parent = "/"
	}
	sum := sha1.Sum([]byte(parent))
	volKey = "VVOL-" + strings.ToUpper(hex.EncodeToString(sum[:6]))
	return parent, filepath.ToSlash(base), volKey
}

func ResolveFullPath(mountPath, relativePath string) string {
	if relativePath == "" || relativePath == "." {
		return filepath.Clean(mountPath)
	}
	return filepath.Join(mountPath, filepath.FromSlash(relativePath))
}

func RelativeFromMount(mountPath, fullPath string) (string, error) {
	rel, err := filepath.Rel(filepath.Clean(mountPath), filepath.Clean(fullPath))
	if err != nil {
		return "", err
	}
	slash := strings.Trim(filepath.ToSlash(rel), "/")
	if slash == "." {
		return "", nil
	}
	return slash, nil
}

func (s *Service) AddRoot(ctx context.Context, in AddRootInput) (RootView, error) {
	rawPath := strings.TrimSpace(in.FullPath)
	if rawPath == "" {
		return RootView{}, ErrEmptyRootPath
	}
	cat := strings.ToLower(strings.TrimSpace(in.ContentCategory))
	if cat == "" {
		cat = "mixed"
	}
	if !validCategory(cat) {
		return RootView{}, ErrInvalidCategory
	}

	absPath, err := filepath.Abs(rawPath)
	if err != nil {
		return RootView{}, fmt.Errorf("library: invalid path: %w", err)
	}
	cleanFull := filepath.Clean(absPath)

	st, statErr := os.Stat(cleanFull)
	if statErr != nil {
		return RootView{}, fmt.Errorf("library: path is not accessible (%s): %w", cleanFull, statErr)
	}
	if !st.IsDir() {
		return RootView{}, errors.New("library: root path must be a directory, not a file")
	}

	existing, err := s.ListRoots(ctx)
	if err != nil {
		return RootView{}, err
	}
	normCandidate := normalizeComparePath(cleanFull)
	for _, r := range existing {
		normExisting := normalizeComparePath(r.FullPath)
		if normCandidate == normExisting {
			return RootView{}, ErrDuplicateRootPath
		}
		if strings.HasPrefix(normCandidate, normExisting+"/") || strings.HasPrefix(normExisting, normCandidate+"/") {
			return RootView{}, ErrDuplicateRootPath
		}
	}

	mountPath, relPath, volSerial := s.matchOrDeriveMount(ctx, cleanFull)
	diskLabel := strings.TrimSpace(in.DiskLabel)
	if diskLabel == "" {
		if vol := filepath.VolumeName(cleanFull); vol != "" {
			diskLabel = fmt.Sprintf("Drive %s (%s)", vol, cat)
		} else {
			diskLabel = fmt.Sprintf("Storage Volume (%s)", filepath.Base(mountPath))
		}
	}

	totalB, freeB := queryDiskSpace(mountPath)
	nowStr := time.Now().UTC().Format(time.RFC3339Nano)

	var diskID string
	err = s.sqlDB.QueryRowContext(ctx, `SELECT id FROM disks WHERE volume_serial = ?`, volSerial).Scan(&diskID)
	if errors.Is(err, sql.ErrNoRows) {
		diskID = domain.NewImmutableID()
		fsType := "NTFS"
		if runtime.GOOS != "windows" {
			fsType = "EXT4/POSIX"
		}
		if _, err := s.sqlDB.ExecContext(ctx, `
			INSERT INTO disks (
				id, volume_serial, label, mount_path, filesystem_type,
				total_bytes, free_bytes, connection_state, is_monitored, last_seen_at, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, 'online', 1, ?, ?, ?)
		`, diskID, volSerial, diskLabel, mountPath, fsType, totalB, freeB, nowStr, nowStr, nowStr); err != nil {
			return RootView{}, fmt.Errorf("library: insert disk: %w", err)
		}
	} else if err != nil {
		return RootView{}, err
	} else {
		_, _ = s.sqlDB.ExecContext(ctx, `
			UPDATE disks SET mount_path = ?, total_bytes = CASE WHEN ? > 0 THEN ? ELSE total_bytes END,
			free_bytes = CASE WHEN ? > 0 THEN ? ELSE free_bytes END,
			connection_state = 'online', last_seen_at = ?, updated_at = ?
			WHERE id = ?
		`, mountPath, totalB, totalB, freeB, freeB, nowStr, nowStr, diskID)
	}

	displayName := strings.TrimSpace(in.DisplayName)
	if displayName == "" {
		displayName = filepath.Base(cleanFull)
	}

	rootID := domain.NewImmutableID()
	if _, err := s.sqlDB.ExecContext(ctx, `
		INSERT INTO library_roots (
			id, disk_id, relative_root_path, content_category, display_name,
			scan_enabled, watch_enabled, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, 1, 1, ?, ?)
	`, rootID, diskID, relPath, cat, displayName, nowStr, nowStr); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return RootView{}, ErrDuplicateRootPath
		}
		return RootView{}, fmt.Errorf("library: insert root: %w", err)
	}

	_, _ = s.sqlDB.ExecContext(ctx, `
		INSERT INTO audit_logs (id, event_category, severity, actor, summary, details_json)
		VALUES (?, 'disk', 'info', 'user', ?, ?)
	`, domain.NewImmutableID(), fmt.Sprintf("Registered library root: %s (%s)", displayName, cat), fmt.Sprintf(`{"rootId":%q,"fullPath":%q}`, rootID, cleanFull))

	return s.GetRoot(ctx, rootID)
}

func (s *Service) matchOrDeriveMount(ctx context.Context, cleanFull string) (string, string, string) {
	if filepath.VolumeName(cleanFull) == "" {
		rows, err := s.sqlDB.QueryContext(ctx, `SELECT mount_path, volume_serial FROM disks`)
		if err == nil {
			defer rows.Close()
			normFull := filepath.ToSlash(cleanFull)
			for rows.Next() {
				var mp, vs string
				if err := rows.Scan(&mp, &vs); err == nil {
					normMP := strings.TrimRight(filepath.ToSlash(mp), "/")
					if normMP != "" && strings.HasPrefix(normFull, normMP+"/") {
						rel := strings.TrimPrefix(normFull, normMP+"/")
						return mp, rel, vs
					}
				}
			}
		}
	}
	return splitMountAndRelative(cleanFull)
}

func (s *Service) RemoveRoot(ctx context.Context, rootID string) error {
	res, err := s.sqlDB.ExecContext(ctx, `DELETE FROM library_roots WHERE id = ?`, rootID)
	if err != nil {
		return err
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return sql.ErrNoRows
	}
	_, _ = s.sqlDB.ExecContext(ctx, `
		INSERT INTO audit_logs (id, event_category, severity, actor, summary, details_json)
		VALUES (?, 'disk', 'warning', 'user', 'Removed library root reference (physical files untouched)', ?)
	`, domain.NewImmutableID(), fmt.Sprintf(`{"rootId":%q}`, rootID))
	return nil
}

func (s *Service) RefreshConnectivity(ctx context.Context) ([]DiskView, []RootView, error) {
	roots, err := s.ListRoots(ctx)
	if err != nil {
		return nil, nil, err
	}
	nowStr := time.Now().UTC().Format(time.RFC3339Nano)
	diskOnlineMap := map[string]bool{}

	for i := range roots {
		r := &roots[i]
		st, err := os.Stat(r.FullPath)
		if err == nil && st.IsDir() {
			r.IsAccessible = true
			diskOnlineMap[r.DiskID] = true
			if r.WatchEnabled && r.LastScanAt != "" {
				if parsedScan, pErr := time.Parse(time.RFC3339Nano, r.LastScanAt); pErr == nil {
					if st.ModTime().After(parsedScan.Add(2 * time.Second)) {
						r.PassiveChangeAlert = true
					}
				}
			}
		} else {
			r.IsAccessible = false
		}
	}

	dRows, err := s.sqlDB.QueryContext(ctx, `SELECT id, mount_path FROM disks`)
	if err == nil {
		type dPair struct{ id, mp string }
		var pairs []dPair
		for dRows.Next() {
			var p dPair
			if err := dRows.Scan(&p.id, &p.mp); err == nil {
				pairs = append(pairs, p)
			}
		}
		dRows.Close()

		for _, p := range pairs {
			online := diskOnlineMap[p.id]
			if !online && len(roots) == 0 {
				if st, sErr := os.Stat(p.mp); sErr == nil && st.IsDir() {
					online = true
				}
			}
			state := "offline"
			if online {
				state = "online"
			}
			_, _ = s.sqlDB.ExecContext(ctx, `
				UPDATE disks SET connection_state = ?,
				last_seen_at = CASE WHEN ? = 'online' THEN ? ELSE last_seen_at END,
				updated_at = ?
				WHERE id = ?
			`, state, state, nowStr, nowStr, p.id)
		}
	}

	disks, err := s.ListDisks(ctx)
	return disks, roots, err
}

func (s *Service) GetRoot(ctx context.Context, rootID string) (RootView, error) {
	roots, err := s.ListRoots(ctx)
	if err != nil {
		return RootView{}, err
	}
	for _, r := range roots {
		if r.ID == rootID {
			return r, nil
		}
	}
	return RootView{}, sql.ErrNoRows
}

func (s *Service) ListRoots(ctx context.Context) ([]RootView, error) {
	rows, err := s.sqlDB.QueryContext(ctx, `
		SELECT r.id, r.disk_id, d.label, d.mount_path, r.relative_root_path,
		       r.content_category, r.display_name, r.scan_enabled, r.watch_enabled,
		       COALESCE(d.last_scan_at, ''),
		       (SELECT COUNT(*) FROM media_files mf WHERE mf.root_id = r.id AND mf.presence_status = 'present')
		FROM library_roots r
		JOIN disks d ON d.id = r.disk_id
		ORDER BY r.created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []RootView{}
	for rows.Next() {
		var r RootView
		var scanInt, watchInt int
		if err := rows.Scan(
			&r.ID, &r.DiskID, &r.DiskLabel, &r.MountPath, &r.RelativeRootPath,
			&r.ContentCategory, &r.DisplayName, &scanInt, &watchInt,
			&r.LastScanAt, &r.IndexedFileCount,
		); err != nil {
			return nil, err
		}
		r.ScanEnabled = scanInt == 1
		r.WatchEnabled = watchInt == 1
		r.FullPath = ResolveFullPath(r.MountPath, r.RelativeRootPath)
		if st, sErr := os.Stat(r.FullPath); sErr == nil && st.IsDir() {
			r.IsAccessible = true
			if r.WatchEnabled && r.LastScanAt != "" {
				if parsed, pErr := time.Parse(time.RFC3339Nano, r.LastScanAt); pErr == nil && st.ModTime().After(parsed.Add(2*time.Second)) {
					r.PassiveChangeAlert = true
				}
			}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Service) ListDisks(ctx context.Context) ([]DiskView, error) {
	rows, err := s.sqlDB.QueryContext(ctx, `
		SELECT d.id, d.volume_serial, d.label, d.mount_path, d.filesystem_type,
		       d.total_bytes, d.free_bytes, d.connection_state, d.last_seen_at, COALESCE(d.last_scan_at, ''),
		       (SELECT COUNT(*) FROM library_roots lr WHERE lr.disk_id = d.id),
		       (SELECT COUNT(*) FROM media_files mf WHERE mf.disk_id = d.id AND mf.presence_status = 'present')
		FROM disks d
		ORDER BY d.created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []DiskView{}
	for rows.Next() {
		var d DiskView
		if err := rows.Scan(
			&d.ID, &d.VolumeSerial, &d.Label, &d.MountPath, &d.FilesystemType,
			&d.TotalBytes, &d.FreeBytes, &d.ConnectionState, &d.LastSeenAt, &d.LastScanAt,
			&d.RootCount, &d.MediaFileCount,
		); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func normalizeComparePath(p string) string {
	s := strings.TrimRight(filepath.ToSlash(filepath.Clean(p)), "/")
	if runtime.GOOS == "windows" {
		s = strings.ToLower(s)
	}
	return s
}
