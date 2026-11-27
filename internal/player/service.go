package player

import (
	"bufio"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/sadigking5-cmd/Estiraha-Next/internal/domain"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/library"
)

var srtTimestampRegex = regexp.MustCompile(`(\d{2}:\d{2}:\d{2}),(\d{3})\s*-->\s*(\d{2}:\d{2}:\d{2}),(\d{3})`)

type SubtitleOption struct {
	TrackID      string `json:"trackId"`
	FileID       string `json:"fileId"`
	Label        string `json:"label"`
	LanguageCode string `json:"languageCode"`
	Format       string `json:"format"`
	WebVTTURL    string `json:"webVttUrl"`
}

type PlaybackSession struct {
	SessionID        string           `json:"sessionId"`
	WorkID           string           `json:"workId"`
	EpisodeID        string           `json:"episodeId"`
	FileID           string           `json:"fileId"`
	Title            string           `json:"title"`
	Filename         string           `json:"filename"`
	FullPath         string           `json:"fullPath"`
	StreamURL        string           `json:"streamUrl"`
	MimeType         string           `json:"mimeType"`
	ResumePositionMs int64            `json:"resumePositionMs"`
	DurationMs       int64            `json:"durationMs"`
	WatchStatus      string           `json:"watchStatus"`
	NextEpisodeID    string           `json:"nextEpisodeId"`
	PrevEpisodeID    string           `json:"prevEpisodeId"`
	Subtitles        []SubtitleOption `json:"subtitles"`
}

type ProgressUpdateInput struct {
	WorkID      string `json:"workId"`
	EpisodeID   string `json:"episodeId"`
	FileID      string `json:"fileId"`
	PositionMs  int64  `json:"positionMs"`
	DurationMs  int64  `json:"durationMs"`
	WatchStatus string `json:"watchStatus"`
}

type ContinueWatchingItem struct {
	WorkID           string `json:"workId"`
	EpisodeID        string `json:"episodeId"`
	FileID           string `json:"fileId"`
	Title            string `json:"title"`
	MediaType        string `json:"mediaType"`
	WatchStatus      string `json:"watchStatus"`
	ResumePositionMs int64  `json:"resumePositionMs"`
	DurationMs       int64  `json:"durationMs"`
	LastPlayedAt     string `json:"lastPlayedAt"`
	PosterURL        string `json:"posterUrl"`
}

type Service struct {
	sqlDB    *sql.DB
	mu       sync.Mutex
	listener net.Listener
	server   *http.Server
	baseURL  string
	token    string
}

func NewService(sqlDB *sql.DB) *Service {
	tokBytes := make([]byte, 16)
	_, _ = rand.Read(tokBytes)
	return &Service{
		sqlDB: sqlDB,
		token: hex.EncodeToString(tokBytes),
	}
}

func (s *Service) StartLoopbackServer() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.baseURL != "" {
		return s.baseURL, nil
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/media/", s.handleMediaStream)
	mux.HandleFunc("/subtitle/", s.handleSubtitleVTT)
	mux.HandleFunc("/artwork/", s.handleArtworkImage)

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		_ = srv.Serve(ln)
	}()

	s.listener = ln
	s.server = srv
	s.baseURL = "http://" + ln.Addr().String()
	return s.baseURL, nil
}

func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.server.Shutdown(ctx)
		s.server = nil
		s.baseURL = ""
	}
	return nil
}

func (s *Service) ArtworkURLForFile(fileID string) string {
	if fileID == "" {
		return ""
	}
	base, err := s.StartLoopbackServer()
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%s/artwork/%s?t=%s", base, fileID, s.token)
}

func (s *Service) BuildPlaybackSession(ctx context.Context, workID, episodeID, explicitFileID string) (PlaybackSession, error) {
	base, err := s.StartLoopbackServer()
	if err != nil {
		return PlaybackSession{}, err
	}

	fileID := strings.TrimSpace(explicitFileID)
	if fileID == "" {
		if episodeID != "" {
			_ = s.sqlDB.QueryRowContext(ctx, `
				SELECT mf.id FROM media_files mf
				LEFT JOIN media_versions mv ON mv.file_id = mf.id
				JOIN disks d ON d.id = mf.disk_id
				WHERE mf.episode_id = ? AND mf.file_kind = 'video' AND mf.presence_status = 'present' AND d.connection_state = 'online'
				ORDER BY COALESCE(mv.computed_quality_score, 0) DESC LIMIT 1
			`, episodeID).Scan(&fileID)
		} else if workID != "" {
			_ = s.sqlDB.QueryRowContext(ctx, `
				SELECT mf.id FROM media_files mf
				LEFT JOIN media_versions mv ON mv.file_id = mf.id
				JOIN disks d ON d.id = mf.disk_id
				WHERE mf.work_id = ? AND mf.file_kind = 'video' AND mf.presence_status = 'present' AND d.connection_state = 'online'
				ORDER BY COALESCE(mv.computed_quality_score, 0) DESC LIMIT 1
			`, workID).Scan(&fileID)
		}
	}
	if fileID == "" {
		return PlaybackSession{}, errors.New("player: no playable online video file found for this item")
	}

	var mountPath, relPath, filename, ext, resolvedWorkID, resolvedEpID, title string
	err = s.sqlDB.QueryRowContext(ctx, `
		SELECT d.mount_path, mf.relative_path, mf.filename, mf.extension,
		       COALESCE(mf.work_id, ''), COALESCE(mf.episode_id, ''),
		       COALESCE(w.canonical_folder_name, mf.filename)
		FROM media_files mf
		JOIN disks d ON d.id = mf.disk_id
		LEFT JOIN works w ON w.id = mf.work_id
		WHERE mf.id = ?
	`, fileID).Scan(&mountPath, &relPath, &filename, &ext, &resolvedWorkID, &resolvedEpID, &title)
	if err != nil {
		return PlaybackSession{}, err
	}

	fullPath := library.ResolveFullPath(mountPath, relPath)
	if _, statErr := os.Stat(fullPath); statErr != nil {
		return PlaybackSession{}, fmt.Errorf("player: media file unavailable on disk (%s): %w", fullPath, statErr)
	}

	var resumeMs, durMs int64
	watchStatus := "unwatched"
	if resolvedEpID != "" {
		_ = s.sqlDB.QueryRowContext(ctx, `
			SELECT watch_status, resume_position_ms, duration_ms FROM watch_states WHERE work_id = ? AND episode_id = ?
		`, resolvedWorkID, resolvedEpID).Scan(&watchStatus, &resumeMs, &durMs)
	} else if resolvedWorkID != "" {
		_ = s.sqlDB.QueryRowContext(ctx, `
			SELECT watch_status, resume_position_ms, duration_ms FROM watch_states WHERE work_id = ? AND episode_id IS NULL
		`, resolvedWorkID).Scan(&watchStatus, &resumeMs, &durMs)
	}

	subs := s.loadSubtitles(ctx, base, resolvedWorkID, resolvedEpID)
	prevEp, nextEp := s.findAdjacentEpisodes(ctx, resolvedWorkID, resolvedEpID)

	sessID := domain.NewImmutableID()
	var epPtr any = nil
	if resolvedEpID != "" {
		epPtr = resolvedEpID
		var epNum, sNum int
		if eErr := s.sqlDB.QueryRowContext(ctx, `
			SELECT ep.episode_number, sn.season_number FROM episodes ep JOIN seasons sn ON sn.id = ep.season_id WHERE ep.id = ?
		`, resolvedEpID).Scan(&epNum, &sNum); eErr == nil {
			title = fmt.Sprintf("%s — S%02dE%02d", title, sNum, epNum)
		}
	}
	if resolvedWorkID != "" {
		_, _ = s.sqlDB.ExecContext(ctx, `
			INSERT INTO playback_history (id, work_id, episode_id, file_id, player_engine, start_position_ms)
			VALUES (?, ?, ?, ?, 'internal_mpv', ?)
		`, sessID, resolvedWorkID, epPtr, fileID, resumeMs)
	}

	return PlaybackSession{
		SessionID:        sessID,
		WorkID:           resolvedWorkID,
		EpisodeID:        resolvedEpID,
		FileID:           fileID,
		Title:            title,
		Filename:         filename,
		FullPath:         fullPath,
		StreamURL:        fmt.Sprintf("%s/media/%s?t=%s", base, fileID, s.token),
		MimeType:         videoMimeType(ext),
		ResumePositionMs: resumeMs,
		DurationMs:       durMs,
		WatchStatus:      watchStatus,
		PrevEpisodeID:    prevEp,
		NextEpisodeID:    nextEp,
		Subtitles:        subs,
	}, nil
}

func (s *Service) SavePlaybackProgress(ctx context.Context, in ProgressUpdateInput) (string, error) {
	if in.WorkID == "" {
		return "", errors.New("player: workId is required")
	}
	status := in.WatchStatus
	source := "auto_player"
	if status != "" {
		source = "manual"
	} else {
		if in.DurationMs > 0 && float64(in.PositionMs)/float64(in.DurationMs) >= 0.90 {
			status = "watched"
		} else if in.PositionMs >= 5000 {
			status = "watching"
		} else {
			status = "unwatched"
		}
	}

	nowStr := time.Now().UTC().Format(time.RFC3339Nano)
	var fPtr any = nil
	if in.FileID != "" {
		fPtr = in.FileID
	}

	if in.EpisodeID != "" {
		var existingID string
		err := s.sqlDB.QueryRowContext(ctx, `SELECT id FROM watch_states WHERE work_id = ? AND episode_id = ?`, in.WorkID, in.EpisodeID).Scan(&existingID)
		if errors.Is(err, sql.ErrNoRows) {
			_, err = s.sqlDB.ExecContext(ctx, `
				INSERT INTO watch_states (
					id, work_id, episode_id, watch_status, resume_position_ms, duration_ms,
					play_count, preferred_file_id, updated_source, last_played_at, updated_at
				) VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?, ?, ?)
			`, domain.NewImmutableID(), in.WorkID, in.EpisodeID, status, in.PositionMs, in.DurationMs, fPtr, source, nowStr, nowStr)
		} else if err == nil {
			_, err = s.sqlDB.ExecContext(ctx, `
				UPDATE watch_states
				SET watch_status = ?, resume_position_ms = ?, duration_ms = CASE WHEN ? > 0 THEN ? ELSE duration_ms END,
				    preferred_file_id = COALESCE(?, preferred_file_id), updated_source = ?, last_played_at = ?, updated_at = ?
				WHERE id = ?
			`, status, in.PositionMs, in.DurationMs, in.DurationMs, fPtr, source, nowStr, nowStr, existingID)
		}
		if err != nil {
			return "", err
		}
		var parentWS string
		if pErr := s.sqlDB.QueryRowContext(ctx, `SELECT id FROM watch_states WHERE work_id = ? AND episode_id IS NULL`, in.WorkID).Scan(&parentWS); errors.Is(pErr, sql.ErrNoRows) {
			_, _ = s.sqlDB.ExecContext(ctx, `
				INSERT INTO watch_states (id, work_id, watch_status, last_watched_episode_id, last_played_at, updated_at)
				VALUES (?, ?, 'watching', ?, ?, ?)
			`, domain.NewImmutableID(), in.WorkID, in.EpisodeID, nowStr, nowStr)
		} else if pErr == nil {
			_, _ = s.sqlDB.ExecContext(ctx, `
				UPDATE watch_states SET watch_status = 'watching', last_watched_episode_id = ?, last_played_at = ?, updated_at = ?
				WHERE id = ?
			`, in.EpisodeID, nowStr, nowStr, parentWS)
		}
		return status, nil
	}

	var existingID string
	err := s.sqlDB.QueryRowContext(ctx, `SELECT id FROM watch_states WHERE work_id = ? AND episode_id IS NULL`, in.WorkID).Scan(&existingID)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = s.sqlDB.ExecContext(ctx, `
			INSERT INTO watch_states (
				id, work_id, watch_status, resume_position_ms, duration_ms,
				play_count, preferred_file_id, updated_source, last_played_at, updated_at
			) VALUES (?, ?, ?, ?, ?, 1, ?, ?, ?, ?)
		`, domain.NewImmutableID(), in.WorkID, status, in.PositionMs, in.DurationMs, fPtr, source, nowStr, nowStr)
	} else if err == nil {
		_, err = s.sqlDB.ExecContext(ctx, `
			UPDATE watch_states
			SET watch_status = ?, resume_position_ms = ?, duration_ms = CASE WHEN ? > 0 THEN ? ELSE duration_ms END,
			    preferred_file_id = COALESCE(?, preferred_file_id), updated_source = ?, last_played_at = ?, updated_at = ?
			WHERE id = ?
		`, status, in.PositionMs, in.DurationMs, in.DurationMs, fPtr, source, nowStr, nowStr, existingID)
	}
	return status, err
}

func (s *Service) ListContinueWatching(ctx context.Context) ([]ContinueWatchingItem, error) {
	rows, err := s.sqlDB.QueryContext(ctx, `
		SELECT ws.work_id, COALESCE(ws.episode_id, ''), COALESCE(ws.preferred_file_id, ''),
		       w.canonical_folder_name, w.media_type, ws.watch_status,
		       ws.resume_position_ms, ws.duration_ms, COALESCE(ws.last_played_at, ws.updated_at),
		       w.poster_rel_path
		FROM watch_states ws
		JOIN works w ON w.id = ws.work_id
		WHERE ws.watch_status IN ('watching', 'watch_later')
		ORDER BY COALESCE(ws.last_played_at, ws.updated_at) DESC
		LIMIT 20
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ContinueWatchingItem{}
	for rows.Next() {
		var it ContinueWatchingItem
		var posterFileID string
		if err := rows.Scan(
			&it.WorkID, &it.EpisodeID, &it.FileID, &it.Title, &it.MediaType, &it.WatchStatus,
			&it.ResumePositionMs, &it.DurationMs, &it.LastPlayedAt, &posterFileID,
		); err == nil {
			it.PosterURL = s.ArtworkURLForFile(posterFileID)
			out = append(out, it)
		}
	}
	return out, rows.Err()
}

func (s *Service) LaunchInSystemPlayer(ctx context.Context, fileID string) error {
	var mountPath, relPath string
	if err := s.sqlDB.QueryRowContext(ctx, `
		SELECT d.mount_path, mf.relative_path FROM media_files mf JOIN disks d ON d.id = mf.disk_id WHERE mf.id = ?
	`, fileID).Scan(&mountPath, &relPath); err != nil {
		return err
	}
	fullPath := library.ResolveFullPath(mountPath, relPath)
	if _, err := os.Stat(fullPath); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		return exec.Command("cmd", "/c", "start", "", fullPath).Start()
	}
	return exec.Command("xdg-open", fullPath).Start()
}

func (s *Service) handleMediaStream(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("t") != s.token {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	fileID := strings.TrimPrefix(r.URL.Path, "/media/")
	var mountPath, relPath, ext string
	if err := s.sqlDB.QueryRowContext(r.Context(), `
		SELECT d.mount_path, mf.relative_path, mf.extension
		FROM media_files mf JOIN disks d ON d.id = mf.disk_id
		WHERE mf.id = ? AND mf.file_kind = 'video'
	`, fileID).Scan(&mountPath, &relPath, &ext); err != nil {
		http.NotFound(w, r)
		return
	}
	fullPath := library.ResolveFullPath(mountPath, relPath)
	f, err := os.Open(fullPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", videoMimeType(ext))
	http.ServeContent(w, r, st.Name(), st.ModTime(), f)
}

func (s *Service) handleSubtitleVTT(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("t") != s.token {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	fileID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/subtitle/"), ".vtt")
	var mountPath, relPath, ext string
	if err := s.sqlDB.QueryRowContext(r.Context(), `
		SELECT d.mount_path, mf.relative_path, mf.extension
		FROM media_files mf JOIN disks d ON d.id = mf.disk_id
		WHERE mf.id = ? AND mf.file_kind = 'subtitle'
	`, fileID).Scan(&mountPath, &relPath, &ext); err != nil {
		http.NotFound(w, r)
		return
	}
	raw, err := os.ReadFile(library.ResolveFullPath(mountPath, relPath))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	vtt := ConvertSubtitleToWebVTT(string(raw), ext)
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	_, _ = w.Write([]byte(vtt))
}

func (s *Service) handleArtworkImage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("t") != s.token {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	fileID := strings.TrimPrefix(r.URL.Path, "/artwork/")
	var mountPath, relPath, ext string
	if err := s.sqlDB.QueryRowContext(r.Context(), `
		SELECT d.mount_path, mf.relative_path, mf.extension
		FROM media_files mf JOIN disks d ON d.id = mf.disk_id
		WHERE mf.id = ? AND mf.file_kind = 'image'
		  AND mf.is_apple_double = 0 AND mf.is_system_artifact = 0 AND mf.size_bytes > 0
	`, fileID).Scan(&mountPath, &relPath, &ext); err != nil {
		http.NotFound(w, r)
		return
	}
	lowerExt := strings.ToLower(ext)
	if lowerExt != ".jpg" && lowerExt != ".jpeg" && lowerExt != ".png" && lowerExt != ".webp" {
		http.Error(w, "unsupported image format", http.StatusUnsupportedMediaType)
		return
	}
	fullImgPath := library.ResolveFullPath(mountPath, relPath)
	st, statErr := os.Stat(fullImgPath)
	if statErr != nil || st.IsDir() || st.Size() <= 0 {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	http.ServeFile(w, r, fullImgPath)
}

func ConvertSubtitleToWebVTT(content, ext string) string {
	trimmed := strings.TrimPrefix(content, "\ufeff")
	lowerExt := strings.ToLower(ext)
	if lowerExt == ".vtt" && strings.HasPrefix(strings.TrimSpace(trimmed), "WEBVTT") {
		return trimmed
	}
	if lowerExt == ".ass" || lowerExt == ".ssa" {
		var sb strings.Builder
		sb.WriteString("WEBVTT\n\n")
		sc := bufio.NewScanner(strings.NewReader(trimmed))
		for sc.Scan() {
			line := sc.Text()
			if strings.HasPrefix(line, "Dialogue:") {
				parts := strings.SplitN(strings.TrimPrefix(line, "Dialogue:"), ",", 10)
				if len(parts) == 10 {
					start := assTimeToVTT(strings.TrimSpace(parts[1]))
					end := assTimeToVTT(strings.TrimSpace(parts[2]))
					txt := strings.ReplaceAll(parts[9], `\N`, "\n")
					txt = regexp.MustCompile(`\{[^}]*\}`).ReplaceAllString(txt, "")
					sb.WriteString(fmt.Sprintf("%s --> %s\n%s\n\n", start, end, strings.TrimSpace(txt)))
				}
			}
		}
		return sb.String()
	}

	converted := srtTimestampRegex.ReplaceAllString(trimmed, "$1.$2 --> $3.$4")
	return "WEBVTT\n\n" + converted + "\n"
}

func assTimeToVTT(t string) string {
	if len(t) > 1 && t[1] == ':' {
		t = "0" + t
	}
	if strings.Count(t, ".") == 1 {
		parts := strings.Split(t, ".")
		if len(parts[1]) == 2 {
			return parts[0] + "." + parts[1] + "0"
		}
	}
	return t
}

func (s *Service) loadSubtitles(ctx context.Context, base, workID, epID string) []SubtitleOption {
	q := `
		SELECT mf.id, mf.filename, mf.extension
		FROM media_files mf
		WHERE mf.work_id = ? AND mf.file_kind = 'subtitle' AND mf.presence_status = 'present'
	`
	args := []any{workID}
	if epID != "" {
		q += ` AND mf.episode_id = ?`
		args = append(args, epID)
	}
	rows, err := s.sqlDB.QueryContext(ctx, q, args...)
	if err != nil {
		return []SubtitleOption{}
	}
	defer rows.Close()

	out := []SubtitleOption{}
	for rows.Next() {
		var fID, fname, ext string
		if err := rows.Scan(&fID, &fname, &ext); err == nil {
			lang := "ar"
			lower := strings.ToLower(fname)
			if strings.Contains(lower, ".en.") || strings.Contains(lower, "english") {
				lang = "en"
			}
			out = append(out, SubtitleOption{
				TrackID:      fID,
				FileID:       fID,
				Label:        fmt.Sprintf("%s (%s)", filepath.Base(fname), strings.ToUpper(lang)),
				LanguageCode: lang,
				Format:       strings.TrimPrefix(ext, "."),
				WebVTTURL:    fmt.Sprintf("%s/subtitle/%s.vtt?t=%s", base, fID, s.token),
			})
		}
	}
	return out
}

func (s *Service) findAdjacentEpisodes(ctx context.Context, workID, epID string) (string, string) {
	if workID == "" || epID == "" {
		return "", ""
	}
	rows, err := s.sqlDB.QueryContext(ctx, `
		SELECT ep.id
		FROM episodes ep
		JOIN seasons sn ON sn.id = ep.season_id
		WHERE ep.work_id = ? AND ep.is_missing = 0
		ORDER BY sn.season_number ASC, ep.episode_number ASC
	`, workID)
	if err != nil {
		return "", ""
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	for i, id := range ids {
		if id == epID {
			prev, next := "", ""
			if i > 0 {
				prev = ids[i-1]
			}
			if i+1 < len(ids) {
				next = ids[i+1]
			}
			return prev, next
		}
	}
	return "", ""
}

func videoMimeType(ext string) string {
	switch strings.ToLower(ext) {
	case ".mp4", ".m4v":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".mkv":
		return "video/x-matroska"
	case ".avi":
		return "video/x-msvideo"
	case ".mov":
		return "video/quicktime"
	default:
		return "video/mp4"
	}
}
