package duplicates

import (
	"context"
	"database/sql"
	"sort"
	"time"

	"github.com/sadigking5-cmd/Estiraha-Next/internal/domain"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/library"
)

type DuplicateMemberView struct {
	FileID          string  `json:"fileId"`
	DiskID          string  `json:"diskId"`
	DiskLabel       string  `json:"diskLabel"`
	FullPath        string  `json:"fullPath"`
	Filename        string  `json:"filename"`
	SizeBytes       int64   `json:"sizeBytes"`
	ResolutionLabel string  `json:"resolutionLabel"`
	VideoCodec      string  `json:"videoCodec"`
	IntegrityStatus string  `json:"integrityStatus"`
	QualityScore    float64 `json:"qualityScore"`
	MemberReason    string  `json:"memberReason"`
	ProposedAction  string  `json:"proposedAction"`
	IsSelectedBest  bool    `json:"isSelectedBest"`
}

type DuplicateGroupView struct {
	GroupID         string                `json:"groupId"`
	DuplicateKind   string                `json:"duplicateKind"`
	WorkID          string                `json:"workId"`
	EpisodeID       string                `json:"episodeId"`
	WorkTitle       string                `json:"workTitle"`
	SelectedFileID  string                `json:"selectedFileId"`
	ResolutionState string                `json:"resolutionState"`
	CrossDiskKeep   bool                  `json:"crossDiskKeep"`
	Members         []DuplicateMemberView `json:"members"`
}

type AnalysisSummary struct {
	GroupsDetected       int `json:"groupsDetected"`
	CrossDiskKeptGroups  int `json:"crossDiskKeptGroups"`
	SameDiskReviewGroups int `json:"sameDiskReviewGroups"`
}

type Service struct {
	sqlDB *sql.DB
}

func NewService(sqlDB *sql.DB) *Service {
	return &Service{sqlDB: sqlDB}
}

type candidateRow struct {
	fileID    string
	diskID    string
	workID    string
	epID      string
	sha256    string
	sizeBytes int64
	integrity string
	score     float64
}

func (s *Service) AnalyzeDuplicates(ctx context.Context) (AnalysisSummary, error) {
	var sum AnalysisSummary
	_, _ = s.sqlDB.ExecContext(ctx, `DELETE FROM duplicate_groups WHERE resolution_state IN ('unresolved', 'kept_intentionally')`)

	rows, err := s.sqlDB.QueryContext(ctx, `
		SELECT mf.id, mf.disk_id, COALESCE(mf.work_id, ''), COALESCE(mf.episode_id, ''),
		       COALESCE(mf.sha256_hash, ''), mf.size_bytes, mf.integrity_status,
		       COALESCE(mv.computed_quality_score, 0)
		FROM media_files mf
		LEFT JOIN media_versions mv ON mv.file_id = mf.id
		WHERE mf.file_kind = 'video' AND mf.presence_status = 'present'
	`)
	if err != nil {
		return sum, err
	}
	var all []candidateRow
	for rows.Next() {
		var c candidateRow
		if err := rows.Scan(&c.fileID, &c.diskID, &c.workID, &c.epID, &c.sha256, &c.sizeBytes, &c.integrity, &c.score); err == nil {
			all = append(all, c)
		}
	}
	rows.Close()

	byTarget := map[string][]candidateRow{}
	for _, c := range all {
		if c.epID != "" {
			byTarget["ep:"+c.epID] = append(byTarget["ep:"+c.epID], c)
		} else if c.workID != "" {
			byTarget["work:"+c.workID] = append(byTarget["work:"+c.workID], c)
		}
	}

	nowStr := time.Now().UTC().Format(time.RFC3339Nano)
	for key, members := range byTarget {
		if len(members) < 2 {
			continue
		}
		sort.Slice(members, func(i, j int) bool {
			if members[i].score != members[j].score {
				return members[i].score > members[j].score
			}
			return members[i].sizeBytes > members[j].sizeBytes
		})

		best := members[0]
		kind := "same_work_multi_version"
		var workPtr, epPtr any = nil, nil
		if members[0].workID != "" {
			workPtr = members[0].workID
		}
		if len(key) > 3 && key[:3] == "ep:" {
			kind = "same_episode_multi_version"
			epPtr = members[0].epID
		}

		diskCounts := map[string]int{}
		for _, m := range members {
			diskCounts[m.diskID]++
		}
		hasSameDiskDup := false
		for _, cnt := range diskCounts {
			if cnt > 1 {
				hasSameDiskDup = true
				break
			}
		}

		resState := "unresolved"
		if !hasSameDiskDup {
			resState = "kept_intentionally"
			sum.CrossDiskKeptGroups++
		} else {
			sum.SameDiskReviewGroups++
		}
		sum.GroupsDetected++

		groupID := domain.NewImmutableID()
		_, err := s.sqlDB.ExecContext(ctx, `
			INSERT INTO duplicate_groups (
				id, duplicate_kind, work_id, episode_id, selected_file_id, resolution_state, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, groupID, kind, workPtr, epPtr, best.fileID, resState, nowStr, nowStr)
		if err != nil {
			continue
		}

		seenDiskBest := map[string]bool{}
		for idx, m := range members {
			action := "propose_move_to_errors"
			reason := "Lower quality duplicate on same disk (requires user review)"
			keepDec := "undecided"
			isAutoPref := 0

			if idx == 0 {
				action = "keep_selected"
				reason = "Best quality version (resolution + codec + integrity)"
				keepDec = "keep"
				isAutoPref = 1
				seenDiskBest[m.diskID] = true
			} else if !seenDiskBest[m.diskID] {
				action = "keep_other_disk"
				reason = "Cross-disk copy preserved per 'Cross-Disk Copies = Keep' policy"
				keepDec = "keep_across_disks"
				seenDiskBest[m.diskID] = true
			}

			_, _ = s.sqlDB.ExecContext(ctx, `
				INSERT INTO duplicate_group_members (group_id, file_id, member_reason, proposed_action)
				VALUES (?, ?, ?, ?)
			`, groupID, m.fileID, reason, action)

			_, _ = s.sqlDB.ExecContext(ctx, `UPDATE media_files SET keep_decision = ? WHERE id = ?`, keepDec, m.fileID)
			_, _ = s.sqlDB.ExecContext(ctx, `UPDATE media_versions SET is_preferred_auto = ? WHERE file_id = ?`, isAutoPref, m.fileID)
		}
	}

	return sum, nil
}

func (s *Service) ListDuplicateGroups(ctx context.Context) ([]DuplicateGroupView, error) {
	rows, err := s.sqlDB.QueryContext(ctx, `
		SELECT dg.id, dg.duplicate_kind, COALESCE(dg.work_id, ''), COALESCE(dg.episode_id, ''),
		       COALESCE(w.canonical_folder_name, w.title_original, 'Unknown Work'),
		       COALESCE(dg.selected_file_id, ''), dg.resolution_state
		FROM duplicate_groups dg
		LEFT JOIN works w ON w.id = dg.work_id
		ORDER BY dg.created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	var groups []DuplicateGroupView
	for rows.Next() {
		var g DuplicateGroupView
		if err := rows.Scan(&g.GroupID, &g.DuplicateKind, &g.WorkID, &g.EpisodeID, &g.WorkTitle, &g.SelectedFileID, &g.ResolutionState); err == nil {
			g.CrossDiskKeep = g.ResolutionState == "kept_intentionally"
			groups = append(groups, g)
		}
	}
	rows.Close()

	for i := range groups {
		g := &groups[i]
		mRows, err := s.sqlDB.QueryContext(ctx, `
			SELECT dgm.file_id, mf.disk_id, d.label, d.mount_path, mf.relative_path, mf.filename,
			       mf.size_bytes, mf.integrity_status,
			       COALESCE(mv.resolution_label, '1080p'), COALESCE(mv.video_codec, 'H.264'), COALESCE(mv.computed_quality_score, 0),
			       dgm.member_reason, dgm.proposed_action
			FROM duplicate_group_members dgm
			JOIN media_files mf ON mf.id = dgm.file_id
			JOIN disks d ON d.id = mf.disk_id
			LEFT JOIN media_versions mv ON mv.file_id = mf.id
			WHERE dgm.group_id = ?
			ORDER BY COALESCE(mv.computed_quality_score, 0) DESC
		`, g.GroupID)
		if err != nil {
			continue
		}
		g.Members = []DuplicateMemberView{}
		for mRows.Next() {
			var mv DuplicateMemberView
			var mp, rel string
			if err := mRows.Scan(
				&mv.FileID, &mv.DiskID, &mv.DiskLabel, &mp, &rel, &mv.Filename,
				&mv.SizeBytes, &mv.IntegrityStatus,
				&mv.ResolutionLabel, &mv.VideoCodec, &mv.QualityScore,
				&mv.MemberReason, &mv.ProposedAction,
			); err == nil {
				mv.FullPath = library.ResolveFullPath(mp, rel)
				mv.IsSelectedBest = mv.FileID == g.SelectedFileID
				g.Members = append(g.Members, mv)
			}
		}
		mRows.Close()
	}
	return groups, nil
}
