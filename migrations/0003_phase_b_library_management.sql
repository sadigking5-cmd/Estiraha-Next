-- ==============================================================================
-- ESTIRAHA NEXT — MIGRATION 0003: PHASE B FULL MEDIA LIBRARY MANAGEMENT SYSTEM
-- Extends works & review_items, adds library_issues for B9 missing/health reports,
-- seeds bilingual countries/genres, and persists all 17 official Phase B policies.
-- ==============================================================================

ALTER TABLE works ADD COLUMN confidence_score REAL NOT NULL DEFAULT 0.0 CHECK (confidence_score >= 0.0 AND confidence_score <= 1.0);
ALTER TABLE works ADD COLUMN tmdb_id TEXT NOT NULL DEFAULT '';
ALTER TABLE works ADD COLUMN elcinema_id TEXT NOT NULL DEFAULT '';
ALTER TABLE works ADD COLUMN poster_rel_path TEXT NOT NULL DEFAULT '';

ALTER TABLE review_items ADD COLUMN confidence_score REAL NOT NULL DEFAULT 0.0 CHECK (confidence_score >= 0.0 AND confidence_score <= 1.0);
ALTER TABLE review_items ADD COLUMN linked_files_json TEXT NOT NULL DEFAULT '[]';
ALTER TABLE review_items ADD COLUMN conflict_details TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS library_issues (
    id TEXT PRIMARY KEY,
    issue_kind TEXT NOT NULL
        CHECK (issue_kind IN (
            'missing_episode',
            'missing_season',
            'missing_subtitle',
            'missing_poster',
            'missing_metadata',
            'corrupted_file',
            'unreadable_file',
            'unidentified_file',
            'duplicate_conflict',
            'offline_disk'
        )),
    severity TEXT NOT NULL DEFAULT 'warning'
        CHECK (severity IN ('info', 'warning', 'error', 'critical')),
    work_id TEXT REFERENCES works(id) ON DELETE CASCADE,
    season_id TEXT REFERENCES seasons(id) ON DELETE CASCADE,
    episode_id TEXT REFERENCES episodes(id) ON DELETE CASCADE,
    file_id TEXT REFERENCES media_files(id) ON DELETE CASCADE,
    disk_id TEXT REFERENCES disks(id) ON DELETE CASCADE,
    title_ar TEXT NOT NULL,
    title_en TEXT NOT NULL,
    details_ar TEXT NOT NULL DEFAULT '',
    details_en TEXT NOT NULL DEFAULT '',
    suggested_fix_ar TEXT NOT NULL DEFAULT '',
    suggested_fix_en TEXT NOT NULL DEFAULT '',
    resolved INTEGER NOT NULL DEFAULT 0 CHECK (resolved IN (0, 1)),
    detected_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    resolved_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_library_issues_kind_resolved ON library_issues(issue_kind, resolved);
CREATE INDEX IF NOT EXISTS idx_library_issues_work ON library_issues(work_id);
CREATE INDEX IF NOT EXISTS idx_library_issues_disk ON library_issues(disk_id);

INSERT OR IGNORE INTO countries (iso_code, name_ar, name_en) VALUES
    ('EG', 'مصر', 'Egypt'),
    ('SA', 'السعودية', 'Saudi Arabia'),
    ('SY', 'سوريا', 'Syria'),
    ('LB', 'لبنان', 'Lebanon'),
    ('KW', 'الكويت', 'Kuwait'),
    ('AE', 'الإمارات', 'United Arab Emirates'),
    ('JO', 'الأردن', 'Jordan'),
    ('MA', 'المغرب', 'Morocco'),
    ('TN', 'تونس', 'Tunisia'),
    ('US', 'الولايات المتحدة', 'United States'),
    ('GB', 'المملكة المتحدة', 'United Kingdom'),
    ('FR', 'فرنسا', 'France'),
    ('DE', 'ألمانيا', 'Germany'),
    ('JP', 'اليابان', 'Japan'),
    ('KR', 'كوريا الجنوبية', 'South Korea'),
    ('IN', 'الهند', 'India'),
    ('TR', 'تركيا', 'Turkey'),
    ('ES', 'إسبانيا', 'Spain'),
    ('IT', 'إيطاليا', 'Italy');

INSERT OR IGNORE INTO genres (id, slug, name_ar, name_en) VALUES
    ('genre-action', 'action', 'أكشن', 'Action'),
    ('genre-drama', 'drama', 'دراما', 'Drama'),
    ('genre-comedy', 'comedy', 'كوميديا', 'Comedy'),
    ('genre-thriller', 'thriller', 'إثارة وتشويق', 'Thriller'),
    ('genre-scifi', 'sci-fi', 'خيال علمي', 'Sci-Fi'),
    ('genre-crime', 'crime', 'جريمة', 'Crime'),
    ('genre-history', 'history', 'تاريخي', 'History'),
    ('genre-documentary', 'documentary', 'وثائقي', 'Documentary'),
    ('genre-animation', 'animation', 'رسوم متحركة', 'Animation'),
    ('genre-family', 'family', 'عائلي', 'Family'),
    ('genre-mystery', 'mystery', 'غموض', 'Mystery'),
    ('genre-biography', 'biography', 'سيرة ذاتية', 'Biography');

INSERT OR IGNORE INTO app_settings (setting_key, setting_value_json, category) VALUES
    ('classification_system', '"Hybrid"', 'organization'),
    ('multi_disk_organization', '"By Content Type"', 'storage'),
    ('cross_disk_duplicates', '"Keep"', 'duplicates'),
    ('automation_level', '"Review"', 'safety'),
    ('library_organization_mode', '"Organize Existing Library"', 'organization'),
    ('unidentified_files_policy', '"Review"', 'safety'),
    ('series_classification_policy', '"Same"', 'organization'),
    ('episode_version_selection', '"Best"', 'duplicates'),
    ('missing_episodes_detection', '"Enabled"', 'reports'),
    ('notifications_and_monitoring', '"Both"', 'monitoring'),
    ('background_behavior', '"System Tray"', 'desktop'),
    ('backup_policy', '"Both"', 'backup'),
    ('scan_trigger_policy', '"Manual"', 'scanner'),
    ('media_player_strategy', '"Integrated"', 'player'),
    ('design_direction', '"Hybrid Cinematic + Administrative"', 'ui'),
    ('ui_density', '"Balanced"', 'ui'),
    ('sidebar_direction_ar', '"right-rtl"', 'ui'),
    ('sidebar_direction_en', '"left-ltr"', 'ui'),
    ('protect_shorts_extension', 'true', 'naming');
