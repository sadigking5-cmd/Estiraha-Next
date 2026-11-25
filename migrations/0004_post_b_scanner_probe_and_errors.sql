-- ==============================================================================
-- ESTIRAHA NEXT — MIGRATION 0004: BATCH 1 SCANNER INTELLIGENCE, MEDIA PROBING
-- & MANAGED ERROR FOLDER FOUNDATION (6 OFFICIAL CATEGORIES)
-- Preserves 100% of existing UUIDs, works, media_files, and user settings.
-- ==============================================================================

ALTER TABLE media_files ADD COLUMN duration_seconds REAL NOT NULL DEFAULT 0.0 CHECK (duration_seconds >= 0.0);
ALTER TABLE media_files ADD COLUMN video_codec TEXT NOT NULL DEFAULT '';
ALTER TABLE media_files ADD COLUMN video_width INTEGER NOT NULL DEFAULT 0 CHECK (video_width >= 0);
ALTER TABLE media_files ADD COLUMN video_height INTEGER NOT NULL DEFAULT 0 CHECK (video_height >= 0);
ALTER TABLE media_files ADD COLUMN has_video_stream INTEGER NOT NULL DEFAULT 0 CHECK (has_video_stream IN (0, 1));
ALTER TABLE media_files ADD COLUMN has_audio_stream INTEGER NOT NULL DEFAULT 0 CHECK (has_audio_stream IN (0, 1));
ALTER TABLE media_files ADD COLUMN probe_status TEXT NOT NULL DEFAULT 'not_applicable'
    CHECK (probe_status IN (
        'not_applicable',
        'probed_ok',
        'probe_unavailable_needs_review',
        'probe_failed_needs_review'
    ));
ALTER TABLE media_files ADD COLUMN anomaly_category TEXT NOT NULL DEFAULT ''
    CHECK (anomaly_category IN (
        '',
        'empty_folders',
        'zero_byte_files',
        'unlinked_subtitles',
        'auxiliary_or_unknown',
        'samples_and_short_clips',
        'conflicts_needing_review'
    ));
ALTER TABLE media_files ADD COLUMN anomaly_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE media_files ADD COLUMN is_apple_double INTEGER NOT NULL DEFAULT 0 CHECK (is_apple_double IN (0, 1));
ALTER TABLE media_files ADD COLUMN is_sample_clip INTEGER NOT NULL DEFAULT 0 CHECK (is_sample_clip IN (0, 1));
ALTER TABLE media_files ADD COLUMN is_system_artifact INTEGER NOT NULL DEFAULT 0 CHECK (is_system_artifact IN (0, 1));

CREATE INDEX IF NOT EXISTS idx_media_files_anomaly ON media_files(anomaly_category) WHERE anomaly_category != '';
CREATE INDEX IF NOT EXISTS idx_media_files_probe_status ON media_files(probe_status);
CREATE INDEX IF NOT EXISTS idx_media_files_sample_apple ON media_files(is_sample_clip, is_apple_double, is_system_artifact);

-- Official Managed Error Folder Registry (الأخطاء/)
-- Tracks all 6 official categories with full provenance, SHA-256 verification, and 1-click restore.
CREATE TABLE IF NOT EXISTS error_folder_entries (
    id TEXT PRIMARY KEY,
    disk_id TEXT NOT NULL REFERENCES disks(id) ON DELETE RESTRICT,
    root_id TEXT REFERENCES library_roots(id) ON DELETE SET NULL,
    file_id TEXT REFERENCES media_files(id) ON DELETE SET NULL,
    batch_id TEXT REFERENCES review_batches(id) ON DELETE SET NULL,
    journal_id TEXT REFERENCES operation_journal(id) ON DELETE SET NULL,
    entry_kind TEXT NOT NULL CHECK (entry_kind IN ('file', 'directory')),
    category_code TEXT NOT NULL
        CHECK (category_code IN (
            'empty_folders',
            'zero_byte_files',
            'unlinked_subtitles',
            'auxiliary_or_unknown',
            'samples_and_short_clips',
            'conflicts_needing_review'
        )),
    category_folder_ar TEXT NOT NULL
        CHECK (category_folder_ar IN (
            'مجلدات فارغة',
            'ملفات صفرية',
            'ترجمات غير مرتبطة',
            'ملفات مساعدة وغير معروفة',
            'عينات ومقاطع قصيرة',
            'تعارضات تحتاج مراجعة'
        )),
    original_rel_path TEXT NOT NULL,
    error_rel_path TEXT NOT NULL,
    item_name TEXT NOT NULL,
    size_bytes INTEGER NOT NULL DEFAULT 0 CHECK (size_bytes >= 0),
    sha256_hash TEXT CHECK (sha256_hash IS NULL OR length(sha256_hash) = 64),
    reason_ar TEXT NOT NULL DEFAULT '',
    reason_en TEXT NOT NULL DEFAULT '',
    entry_status TEXT NOT NULL DEFAULT 'active'
        CHECK (entry_status IN ('planned', 'active', 'restored')),
    moved_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    restored_at TEXT,
    UNIQUE (disk_id, error_rel_path)
);

CREATE INDEX IF NOT EXISTS idx_error_folder_entries_status_cat ON error_folder_entries(entry_status, category_code);
CREATE INDEX IF NOT EXISTS idx_error_folder_entries_root ON error_folder_entries(root_id, entry_status);
CREATE INDEX IF NOT EXISTS idx_error_folder_entries_file ON error_folder_entries(file_id);
