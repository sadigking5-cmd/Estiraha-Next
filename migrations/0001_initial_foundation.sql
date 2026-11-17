-- ==============================================================================
-- ESTIRAHA NEXT — MIGRATION 0001: CORE RELATIONAL SCHEMA FOUNDATION
-- Engine: SQLite 3 (WAL Mode, Foreign Keys Enforced, 200,000+ Files Ready)
-- Immutable ID Principle: Every work, person, and physical file holds a stable
-- UUID text primary key that NEVER changes across renames or disk moves.
-- ==============================================================================

-- 1. DISKS & STORAGE VOLUMES
CREATE TABLE IF NOT EXISTS disks (
    id TEXT PRIMARY KEY,
    volume_serial TEXT NOT NULL UNIQUE,
    volume_guid TEXT UNIQUE,
    label TEXT NOT NULL DEFAULT '',
    mount_path TEXT NOT NULL,
    filesystem_type TEXT NOT NULL DEFAULT 'NTFS',
    total_bytes INTEGER NOT NULL DEFAULT 0 CHECK (total_bytes >= 0),
    free_bytes INTEGER NOT NULL DEFAULT 0 CHECK (free_bytes >= 0),
    connection_state TEXT NOT NULL DEFAULT 'online'
        CHECK (connection_state IN ('online', 'offline', 'degraded', 'ejected')),
    is_monitored INTEGER NOT NULL DEFAULT 1 CHECK (is_monitored IN (0, 1)),
    last_seen_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    last_scan_at TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_disks_connection_state ON disks(connection_state);

-- 2. LIBRARY ROOT PATHS
CREATE TABLE IF NOT EXISTS library_roots (
    id TEXT PRIMARY KEY,
    disk_id TEXT NOT NULL REFERENCES disks(id) ON DELETE RESTRICT,
    relative_root_path TEXT NOT NULL,
    content_category TEXT NOT NULL
        CHECK (content_category IN (
            'movies', 'series', 'programs', 'documentaries',
            'anime', 'cartoons', 'celebrities', 'franchises', 'mixed', 'errors'
        )),
    display_name TEXT NOT NULL,
    scan_enabled INTEGER NOT NULL DEFAULT 1 CHECK (scan_enabled IN (0, 1)),
    watch_enabled INTEGER NOT NULL DEFAULT 1 CHECK (watch_enabled IN (0, 1)),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE (disk_id, relative_root_path)
);

CREATE INDEX IF NOT EXISTS idx_library_roots_disk ON library_roots(disk_id);

-- 3. COUNTRIES & GENRES
CREATE TABLE IF NOT EXISTS countries (
    iso_code TEXT PRIMARY KEY CHECK (length(iso_code) BETWEEN 2 AND 3),
    name_ar TEXT NOT NULL,
    name_en TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS genres (
    id TEXT PRIMARY KEY,
    slug TEXT NOT NULL UNIQUE,
    name_ar TEXT NOT NULL,
    name_en TEXT NOT NULL
);

-- 4. CINEMATIC FRANCHISES (No year in main franchise folder name)
CREATE TABLE IF NOT EXISTS franchises (
    id TEXT PRIMARY KEY,
    title_original TEXT NOT NULL,
    title_ar TEXT NOT NULL DEFAULT '',
    title_en TEXT NOT NULL DEFAULT '',
    canonical_folder_name TEXT NOT NULL,
    overview_ar TEXT NOT NULL DEFAULT '',
    overview_en TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_franchises_canonical_folder ON franchises(canonical_folder_name);

-- 5. MEDIA WORKS (All 12 Supported Media Types)
-- Naming Enforcement Constraints:
--   - Separate fields for original, Arabic, and English titles (no speculative translation)
--   - Standalone movies & documentary_movie require production_year when identified
CREATE TABLE IF NOT EXISTS works (
    id TEXT PRIMARY KEY,
    media_type TEXT NOT NULL
        CHECK (media_type IN (
            'movie',
            'series',
            'tv_program',
            'talk_show',
            'game_show',
            'sports_program',
            'entertainment_program',
            'documentary_movie',
            'documentary_series',
            'anime',
            'cartoon',
            'franchise_entry'
        )),
    sub_format TEXT NOT NULL DEFAULT 'standalone'
        CHECK (sub_format IN ('standalone', 'episodic')),
    title_original TEXT NOT NULL,
    title_ar TEXT NOT NULL DEFAULT '',
    title_en TEXT NOT NULL DEFAULT '',
    sort_title TEXT NOT NULL DEFAULT '',
    original_language TEXT NOT NULL DEFAULT '',
    is_arabic_origin INTEGER NOT NULL DEFAULT 0 CHECK (is_arabic_origin IN (0, 1)),
    production_year INTEGER CHECK (production_year IS NULL OR (production_year BETWEEN 1888 AND 2100)),
    end_year INTEGER CHECK (end_year IS NULL OR (end_year BETWEEN 1888 AND 2100)),
    canonical_folder_name TEXT NOT NULL DEFAULT '',
    identification_status TEXT NOT NULL DEFAULT 'unidentified'
        CHECK (identification_status IN ('unidentified', 'identified', 'unknown_program', 'needs_review', 'locked_by_user')),
    franchise_id TEXT REFERENCES franchises(id) ON DELETE SET NULL,
    franchise_order INTEGER,
    synopsis_ar TEXT NOT NULL DEFAULT '',
    synopsis_en TEXT NOT NULL DEFAULT '',
    age_rating TEXT NOT NULL DEFAULT '',
    runtime_minutes INTEGER CHECK (runtime_minutes IS NULL OR runtime_minutes >= 0),
    is_local_present INTEGER NOT NULL DEFAULT 1 CHECK (is_local_present IN (0, 1)),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_works_media_type ON works(media_type);
CREATE INDEX IF NOT EXISTS idx_works_year ON works(production_year);
CREATE INDEX IF NOT EXISTS idx_works_franchise ON works(franchise_id);
CREATE INDEX IF NOT EXISTS idx_works_identification ON works(identification_status);
CREATE INDEX IF NOT EXISTS idx_works_local_present ON works(is_local_present);

CREATE TABLE IF NOT EXISTS work_countries (
    work_id TEXT NOT NULL REFERENCES works(id) ON DELETE CASCADE,
    country_code TEXT NOT NULL REFERENCES countries(iso_code) ON DELETE RESTRICT,
    PRIMARY KEY (work_id, country_code)
);

CREATE TABLE IF NOT EXISTS work_genres (
    work_id TEXT NOT NULL REFERENCES works(id) ON DELETE CASCADE,
    genre_id TEXT NOT NULL REFERENCES genres(id) ON DELETE RESTRICT,
    PRIMARY KEY (work_id, genre_id)
);

-- 6. SEASONS & EPISODES (Series, Episodic Documentaries, Known Programs, Anime, Cartoons)
CREATE TABLE IF NOT EXISTS seasons (
    id TEXT PRIMARY KEY,
    work_id TEXT NOT NULL REFERENCES works(id) ON DELETE CASCADE,
    season_number INTEGER NOT NULL CHECK (season_number >= 0),
    canonical_folder_code TEXT NOT NULL, -- e.g., S01, S02
    title_ar TEXT NOT NULL DEFAULT '',
    title_en TEXT NOT NULL DEFAULT '',
    air_year INTEGER CHECK (air_year IS NULL OR (air_year BETWEEN 1888 AND 2100)),
    expected_episode_count INTEGER CHECK (expected_episode_count IS NULL OR expected_episode_count >= 0),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE (work_id, season_number)
);

CREATE INDEX IF NOT EXISTS idx_seasons_work ON seasons(work_id);

CREATE TABLE IF NOT EXISTS episodes (
    id TEXT PRIMARY KEY,
    work_id TEXT NOT NULL REFERENCES works(id) ON DELETE CASCADE,
    season_id TEXT NOT NULL REFERENCES seasons(id) ON DELETE CASCADE,
    episode_number INTEGER NOT NULL CHECK (episode_number >= 0),
    canonical_file_stem TEXT NOT NULL, -- e.g., 01, 02
    title_original TEXT NOT NULL DEFAULT '',
    title_ar TEXT NOT NULL DEFAULT '',
    title_en TEXT NOT NULL DEFAULT '',
    air_date TEXT,
    runtime_minutes INTEGER CHECK (runtime_minutes IS NULL OR runtime_minutes >= 0),
    is_missing INTEGER NOT NULL DEFAULT 0 CHECK (is_missing IN (0, 1)),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE (season_id, episode_number)
);

CREATE INDEX IF NOT EXISTS idx_episodes_work ON episodes(work_id);
CREATE INDEX IF NOT EXISTS idx_episodes_missing ON episodes(work_id, is_missing);

-- 7. PEOPLE / CELEBRITIES & CREDITS
-- Canonical display format: الاسم بالعربية (English Name)
-- Linked virtually to movies & series WITHOUT duplicating physical video files.
CREATE TABLE IF NOT EXISTS people (
    id TEXT PRIMARY KEY,
    name_ar TEXT NOT NULL DEFAULT '',
    name_en TEXT NOT NULL DEFAULT '',
    name_original TEXT NOT NULL DEFAULT '',
    canonical_display_name TEXT NOT NULL,
    biography_ar TEXT NOT NULL DEFAULT '',
    biography_en TEXT NOT NULL DEFAULT '',
    birth_date TEXT,
    death_date TEXT,
    country_code TEXT REFERENCES countries(iso_code) ON DELETE SET NULL,
    primary_profession TEXT NOT NULL DEFAULT 'actor',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_people_canonical_name ON people(canonical_display_name);

CREATE TABLE IF NOT EXISTS work_credits (
    id TEXT PRIMARY KEY,
    work_id TEXT NOT NULL REFERENCES works(id) ON DELETE CASCADE,
    person_id TEXT NOT NULL REFERENCES people(id) ON DELETE CASCADE,
    role_type TEXT NOT NULL
        CHECK (role_type IN ('actor', 'director', 'writer', 'producer', 'composer', 'host', 'narrator', 'voice_actor')),
    character_name_ar TEXT NOT NULL DEFAULT '',
    character_name_en TEXT NOT NULL DEFAULT '',
    billing_order INTEGER NOT NULL DEFAULT 999,
    UNIQUE (work_id, person_id, role_type, character_name_en)
);

CREATE INDEX IF NOT EXISTS idx_work_credits_person ON work_credits(person_id, role_type);
CREATE INDEX IF NOT EXISTS idx_work_credits_work ON work_credits(work_id, role_type);

-- 8. COLLECTIONS (Custom & Thematic Groups)
CREATE TABLE IF NOT EXISTS collections (
    id TEXT PRIMARY KEY,
    name_ar TEXT NOT NULL,
    name_en TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    is_smart_collection INTEGER NOT NULL DEFAULT 0 CHECK (is_smart_collection IN (0, 1)),
    smart_filter_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE IF NOT EXISTS collection_items (
    collection_id TEXT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    work_id TEXT NOT NULL REFERENCES works(id) ON DELETE CASCADE,
    sort_order INTEGER NOT NULL DEFAULT 0,
    added_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    PRIMARY KEY (collection_id, work_id)
);

-- 9. PHYSICAL MEDIA FILES (200,000+ Scale Target)
-- Immutable `id` persists across renames/moves.
-- Archives (ZIP/RAR) are inspected read-only; never auto-extracted.
CREATE TABLE IF NOT EXISTS media_files (
    id TEXT PRIMARY KEY,
    disk_id TEXT NOT NULL REFERENCES disks(id) ON DELETE RESTRICT,
    root_id TEXT REFERENCES library_roots(id) ON DELETE SET NULL,
    work_id TEXT REFERENCES works(id) ON DELETE SET NULL,
    episode_id TEXT REFERENCES episodes(id) ON DELETE SET NULL,
    relative_path TEXT NOT NULL,
    parent_rel_path TEXT NOT NULL,
    filename TEXT NOT NULL,
    extension TEXT NOT NULL,
    file_kind TEXT NOT NULL
        CHECK (file_kind IN (
            'video', 'subtitle', 'image', 'nfo', 'shorts',
            'archive_zip', 'archive_rar', 'audio_ext', 'unknown_extra'
        )),
    size_bytes INTEGER NOT NULL CHECK (size_bytes >= 0),
    mtime_unix INTEGER NOT NULL,
    partial_fingerprint TEXT, -- Fast head/tail hash pre-filter before full SHA-256
    sha256_hash TEXT CHECK (sha256_hash IS NULL OR length(sha256_hash) = 64),
    nesting_depth INTEGER NOT NULL DEFAULT 0 CHECK (nesting_depth >= 0),
    archive_entries_json TEXT, -- Read-only manifest for ZIP/RAR without extraction
    integrity_status TEXT NOT NULL DEFAULT 'unchecked'
        CHECK (integrity_status IN ('unchecked', 'healthy', 'corrupted', 'unreadable', 'truncated')),
    presence_status TEXT NOT NULL DEFAULT 'present'
        CHECK (presence_status IN ('present', 'missing', 'moved_to_errors', 'trashed')),
    keep_decision TEXT NOT NULL DEFAULT 'undecided'
        CHECK (keep_decision IN ('undecided', 'keep', 'keep_across_disks', 'candidate_error_move', 'candidate_delete')),
    classification_reason TEXT NOT NULL DEFAULT '',
    discovered_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    verified_at TEXT,
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE (disk_id, relative_path)
);

CREATE INDEX IF NOT EXISTS idx_media_files_sha256 ON media_files(sha256_hash) WHERE sha256_hash IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_media_files_work ON media_files(work_id);
CREATE INDEX IF NOT EXISTS idx_media_files_episode ON media_files(episode_id);
CREATE INDEX IF NOT EXISTS idx_media_files_kind_presence ON media_files(file_kind, presence_status);
CREATE INDEX IF NOT EXISTS idx_media_files_integrity ON media_files(integrity_status);
CREATE INDEX IF NOT EXISTS idx_media_files_parent ON media_files(disk_id, parent_rel_path);

-- 10. VIDEO VERSIONS & QUALITY SCORING ENGINE
CREATE TABLE IF NOT EXISTS media_versions (
    id TEXT PRIMARY KEY,
    file_id TEXT NOT NULL UNIQUE REFERENCES media_files(id) ON DELETE CASCADE,
    work_id TEXT REFERENCES works(id) ON DELETE SET NULL,
    episode_id TEXT REFERENCES episodes(id) ON DELETE SET NULL,
    container_format TEXT NOT NULL DEFAULT '',
    width INTEGER NOT NULL DEFAULT 0 CHECK (width >= 0),
    height INTEGER NOT NULL DEFAULT 0 CHECK (height >= 0),
    resolution_label TEXT NOT NULL DEFAULT '', -- e.g., 4K, 1080p, 720p, SD
    video_codec TEXT NOT NULL DEFAULT '',
    video_profile TEXT NOT NULL DEFAULT '',
    hdr_format TEXT NOT NULL DEFAULT 'SDR'
        CHECK (hdr_format IN ('SDR', 'HDR10', 'HDR10+', 'DolbyVision', 'HLG')),
    bitrate_kbps INTEGER NOT NULL DEFAULT 0 CHECK (bitrate_kbps >= 0),
    frame_rate REAL NOT NULL DEFAULT 0 CHECK (frame_rate >= 0),
    duration_ms INTEGER NOT NULL DEFAULT 0 CHECK (duration_ms >= 0),
    audio_codec TEXT NOT NULL DEFAULT '',
    audio_channels INTEGER NOT NULL DEFAULT 0 CHECK (audio_channels >= 0),
    audio_tracks_json TEXT NOT NULL DEFAULT '[]',
    embedded_subtitles_json TEXT NOT NULL DEFAULT '[]',
    computed_quality_score REAL NOT NULL DEFAULT 0,
    is_preferred_auto INTEGER NOT NULL DEFAULT 0 CHECK (is_preferred_auto IN (0, 1)),
    is_preferred_manual INTEGER NOT NULL DEFAULT 0 CHECK (is_preferred_manual IN (0, 1)),
    version_role TEXT NOT NULL DEFAULT 'primary'
        CHECK (version_role IN ('primary', 'alternate_quality', 'exact_duplicate', 'lower_quality_extra', 'corrupted')),
    analyzed_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_media_versions_work ON media_versions(work_id, computed_quality_score DESC);
CREATE INDEX IF NOT EXISTS idx_media_versions_episode ON media_versions(episode_id, computed_quality_score DESC);
CREATE INDEX IF NOT EXISTS idx_media_versions_resolution ON media_versions(resolution_label);

-- 11. DUPLICATE GROUPS (Exact SHA-256 vs Alternate Quality Versions)
CREATE TABLE IF NOT EXISTS duplicate_groups (
    id TEXT PRIMARY KEY,
    duplicate_kind TEXT NOT NULL
        CHECK (duplicate_kind IN ('exact_sha256', 'same_work_multi_version', 'same_episode_multi_version')),
    sha256_hash TEXT,
    work_id TEXT REFERENCES works(id) ON DELETE CASCADE,
    episode_id TEXT REFERENCES episodes(id) ON DELETE CASCADE,
    selected_file_id TEXT REFERENCES media_files(id) ON DELETE SET NULL,
    resolution_state TEXT NOT NULL DEFAULT 'unresolved'
        CHECK (resolution_state IN ('unresolved', 'kept_intentionally', 'queued_for_review', 'resolved')),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE IF NOT EXISTS duplicate_group_members (
    group_id TEXT NOT NULL REFERENCES duplicate_groups(id) ON DELETE CASCADE,
    file_id TEXT NOT NULL REFERENCES media_files(id) ON DELETE CASCADE,
    member_reason TEXT NOT NULL DEFAULT '',
    proposed_action TEXT NOT NULL DEFAULT 'none'
        CHECK (proposed_action IN ('none', 'keep_selected', 'keep_other_disk', 'propose_move_to_errors', 'manual_delete_candidate')),
    PRIMARY KEY (group_id, file_id)
);

-- 12. SUBTITLES (SRT, ASS, SSA, VTT & Embedded)
CREATE TABLE IF NOT EXISTS subtitle_tracks (
    id TEXT PRIMARY KEY,
    work_id TEXT REFERENCES works(id) ON DELETE CASCADE,
    episode_id TEXT REFERENCES episodes(id) ON DELETE CASCADE,
    file_id TEXT REFERENCES media_files(id) ON DELETE CASCADE,
    matched_video_file_id TEXT REFERENCES media_files(id) ON DELETE SET NULL,
    source_type TEXT NOT NULL CHECK (source_type IN ('external_file', 'embedded_stream')),
    format TEXT NOT NULL CHECK (format IN ('srt', 'ass', 'ssa', 'vtt', 'subrip', 'pgs', 'other')),
    language_code TEXT NOT NULL DEFAULT 'und',
    is_forced INTEGER NOT NULL DEFAULT 0 CHECK (is_forced IN (0, 1)),
    is_hearing_impaired INTEGER NOT NULL DEFAULT 0 CHECK (is_hearing_impaired IN (0, 1)),
    encoding TEXT NOT NULL DEFAULT 'UTF-8',
    sync_offset_ms INTEGER NOT NULL DEFAULT 0,
    is_matched_primary INTEGER NOT NULL DEFAULT 0 CHECK (is_matched_primary IN (0, 1)),
    is_redundant_extra INTEGER NOT NULL DEFAULT 0 CHECK (is_redundant_extra IN (0, 1)),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_subtitle_tracks_video ON subtitle_tracks(matched_video_file_id);
CREATE INDEX IF NOT EXISTS idx_subtitle_tracks_work ON subtitle_tracks(work_id, language_code);

-- 13. DATA PROVIDERS (API HUB) & METADATA CLAIMS
-- Note: API keys are NEVER stored here; only `credential_manager_target` referencing Windows Credential Manager.
CREATE TABLE IF NOT EXISTS data_providers (
    id TEXT PRIMARY KEY, -- e.g., tmdb, omdb, tvmaze, tvdb, trakt, simkl, opensubtitles, deepseek, elcinema
    display_name TEXT NOT NULL,
    provider_category TEXT NOT NULL
        CHECK (provider_category IN ('metadata', 'ai_assistant', 'subtitles', 'tracking', 'arabic_cinema')),
    integration_status TEXT NOT NULL DEFAULT 'disabled'
        CHECK (integration_status IN ('enabled', 'disabled', 'search_only', 'deferred_legal_review', 'unverified')),
    priority_rank INTEGER NOT NULL DEFAULT 100,
    rate_limit_per_minute INTEGER NOT NULL DEFAULT 30 CHECK (rate_limit_per_minute > 0),
    requires_api_key INTEGER NOT NULL DEFAULT 1 CHECK (requires_api_key IN (0, 1)),
    credential_manager_target TEXT NOT NULL DEFAULT '',
    require_per_request_consent INTEGER NOT NULL DEFAULT 0 CHECK (require_per_request_consent IN (0, 1)),
    terms_reviewed INTEGER NOT NULL DEFAULT 0 CHECK (terms_reviewed IN (0, 1)),
    commercial_use_allowed INTEGER NOT NULL DEFAULT 0 CHECK (commercial_use_allowed IN (0, 1)),
    attribution_requirement TEXT NOT NULL DEFAULT '',
    cache_ttl_seconds INTEGER NOT NULL DEFAULT 86400 CHECK (cache_ttl_seconds >= 0),
    last_health_check_at TEXT,
    last_health_status TEXT NOT NULL DEFAULT 'untested',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE IF NOT EXISTS metadata_claims (
    id TEXT PRIMARY KEY,
    entity_type TEXT NOT NULL CHECK (entity_type IN ('work', 'season', 'episode', 'person', 'franchise')),
    entity_id TEXT NOT NULL,
    provider_id TEXT NOT NULL REFERENCES data_providers(id) ON DELETE RESTRICT,
    external_entity_id TEXT NOT NULL DEFAULT '',
    field_name TEXT NOT NULL, -- e.g., title_ar, production_year, synopsis_ar, director
    claimed_value_json TEXT NOT NULL,
    confidence_score REAL NOT NULL DEFAULT 0.5 CHECK (confidence_score >= 0.0 AND confidence_score <= 1.0),
    is_selected INTEGER NOT NULL DEFAULT 0 CHECK (is_selected IN (0, 1)),
    has_conflict INTEGER NOT NULL DEFAULT 0 CHECK (has_conflict IN (0, 1)),
    fetched_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    expires_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_metadata_claims_entity ON metadata_claims(entity_type, entity_id, field_name);
CREATE INDEX IF NOT EXISTS idx_metadata_claims_provider ON metadata_claims(provider_id, external_entity_id);

CREATE TABLE IF NOT EXISTS provider_http_cache (
    cache_key TEXT PRIMARY KEY,
    provider_id TEXT NOT NULL REFERENCES data_providers(id) ON DELETE CASCADE,
    response_payload BLOB NOT NULL,
    byte_size INTEGER NOT NULL CHECK (byte_size >= 0),
    fetched_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    expires_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_provider_http_cache_expiry ON provider_http_cache(expires_at);

-- 14. ARTWORK ASSETS & SMART IMAGE ENGINE CACHE
-- Rule: Unsafe SVG is forbidden; user-owned local images are protected from deletion/replacement.
CREATE TABLE IF NOT EXISTS artwork_assets (
    id TEXT PRIMARY KEY,
    entity_type TEXT NOT NULL CHECK (entity_type IN ('work', 'season', 'episode', 'person', 'franchise', 'collection')),
    entity_id TEXT NOT NULL,
    artwork_kind TEXT NOT NULL
        CHECK (artwork_kind IN ('poster', 'backdrop', 'celebrity_portrait', 'series_artwork', 'season_poster', 'still', 'extra_image')),
    origin_type TEXT NOT NULL CHECK (origin_type IN ('user_local', 'external_provider')),
    local_file_id TEXT REFERENCES media_files(id) ON DELETE SET NULL,
    provider_id TEXT REFERENCES data_providers(id) ON DELETE SET NULL,
    source_url TEXT NOT NULL DEFAULT '',
    mime_type TEXT NOT NULL CHECK (mime_type IN ('image/jpeg', 'image/png', 'image/webp')),
    width INTEGER NOT NULL CHECK (width > 0 AND width <= 16384),
    height INTEGER NOT NULL CHECK (height > 0 AND height <= 16384),
    byte_size INTEGER NOT NULL CHECK (byte_size > 0),
    sha256_hash TEXT,
    is_primary INTEGER NOT NULL DEFAULT 0 CHECK (is_primary IN (0, 1)),
    is_protected_user_asset INTEGER NOT NULL DEFAULT 0 CHECK (is_protected_user_asset IN (0, 1)),
    is_redundant_extra INTEGER NOT NULL DEFAULT 0 CHECK (is_redundant_extra IN (0, 1)),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_artwork_assets_entity ON artwork_assets(entity_type, entity_id, artwork_kind);

CREATE TABLE IF NOT EXISTS image_cache_entries (
    cache_key TEXT PRIMARY KEY,
    artwork_id TEXT REFERENCES artwork_assets(id) ON DELETE CASCADE,
    cache_tier TEXT NOT NULL
        CHECK (cache_tier IN ('poster', 'backdrop', 'celebrity', 'series_artwork', 'thumbnail')),
    cache_rel_path TEXT NOT NULL UNIQUE, -- Strictly inside AppData cache dir, NEVER inside user media folders
    mime_type TEXT NOT NULL CHECK (mime_type IN ('image/jpeg', 'image/png', 'image/webp')),
    width INTEGER NOT NULL CHECK (width > 0),
    height INTEGER NOT NULL CHECK (height > 0),
    byte_size INTEGER NOT NULL CHECK (byte_size > 0),
    access_count INTEGER NOT NULL DEFAULT 1 CHECK (access_count >= 0),
    last_accessed_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    expires_at TEXT,
    pinned_offline INTEGER NOT NULL DEFAULT 0 CHECK (pinned_offline IN (0, 1)),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_image_cache_lru ON image_cache_entries(pinned_offline, last_accessed_at ASC);
CREATE INDEX IF NOT EXISTS idx_image_cache_tier ON image_cache_entries(cache_tier);

-- 15. WATCH MANAGEMENT, PROGRESS, RATINGS, NOTES & LISTS (Hybrid Tracking)
CREATE TABLE IF NOT EXISTS watch_states (
    id TEXT PRIMARY KEY,
    work_id TEXT NOT NULL REFERENCES works(id) ON DELETE CASCADE,
    episode_id TEXT REFERENCES episodes(id) ON DELETE CASCADE,
    watch_status TEXT NOT NULL DEFAULT 'unwatched'
        CHECK (watch_status IN ('unwatched', 'watching', 'watched', 'watch_later', 'rewatching')),
    resume_position_ms INTEGER NOT NULL DEFAULT 0 CHECK (resume_position_ms >= 0),
    duration_ms INTEGER NOT NULL DEFAULT 0 CHECK (duration_ms >= 0),
    play_count INTEGER NOT NULL DEFAULT 0 CHECK (play_count >= 0),
    preferred_file_id TEXT REFERENCES media_files(id) ON DELETE SET NULL,
    preferred_subtitle_id TEXT REFERENCES subtitle_tracks(id) ON DELETE SET NULL,
    preferred_audio_track INTEGER,
    subtitle_delay_ms INTEGER NOT NULL DEFAULT 0,
    playback_speed REAL NOT NULL DEFAULT 1.0 CHECK (playback_speed > 0.0 AND playback_speed <= 4.0),
    last_watched_episode_id TEXT REFERENCES episodes(id) ON DELETE SET NULL,
    updated_source TEXT NOT NULL DEFAULT 'manual' CHECK (updated_source IN ('auto_player', 'manual')),
    last_played_at TEXT,
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE (work_id, episode_id)
);

CREATE INDEX IF NOT EXISTS idx_watch_states_status ON watch_states(watch_status, last_played_at DESC);

CREATE TABLE IF NOT EXISTS playback_history (
    id TEXT PRIMARY KEY,
    work_id TEXT NOT NULL REFERENCES works(id) ON DELETE CASCADE,
    episode_id TEXT REFERENCES episodes(id) ON DELETE SET NULL,
    file_id TEXT REFERENCES media_files(id) ON DELETE SET NULL,
    player_engine TEXT NOT NULL CHECK (player_engine IN ('internal_mpv', 'external_vlc', 'external_custom')),
    started_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    ended_at TEXT,
    start_position_ms INTEGER NOT NULL DEFAULT 0,
    end_position_ms INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS user_ratings_notes (
    work_id TEXT PRIMARY KEY REFERENCES works(id) ON DELETE CASCADE,
    rating_out_of_10 REAL CHECK (rating_out_of_10 IS NULL OR (rating_out_of_10 >= 0.0 AND rating_out_of_10 <= 10.0)),
    personal_notes TEXT NOT NULL DEFAULT '',
    is_favorite INTEGER NOT NULL DEFAULT 0 CHECK (is_favorite IN (0, 1)),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE IF NOT EXISTS user_lists (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    list_kind TEXT NOT NULL DEFAULT 'watchlist'
        CHECK (list_kind IN ('favorites', 'watchlist', 'custom')),
    description TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE IF NOT EXISTS user_list_items (
    list_id TEXT NOT NULL REFERENCES user_lists(id) ON DELETE CASCADE,
    work_id TEXT NOT NULL REFERENCES works(id) ON DELETE CASCADE,
    episode_id TEXT REFERENCES episodes(id) ON DELETE CASCADE,
    sort_order INTEGER NOT NULL DEFAULT 0,
    added_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    PRIMARY KEY (list_id, work_id)
);

-- 16. REVIEW CENTER & SAFETY PROTECTION ENGINE
-- Mandatory Rule: Zero file operations without explicit user approval on review items.
CREATE TABLE IF NOT EXISTS review_batches (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    batch_type TEXT NOT NULL
        CHECK (batch_type IN (
            'organize_library', 'rename_standardize', 'move_extras_to_errors',
            'unknown_program_review', 'duplicate_cleanup', 'manual_trash'
        )),
    batch_status TEXT NOT NULL DEFAULT 'draft_preview'
        CHECK (batch_status IN (
            'draft_preview', 'awaiting_approval', 'partially_approved',
            'approved', 'executing', 'completed', 'halted_on_interrupt', 'rolled_back', 'cancelled'
        )),
    total_items INTEGER NOT NULL DEFAULT 0 CHECK (total_items >= 0),
    approved_items INTEGER NOT NULL DEFAULT 0 CHECK (approved_items >= 0),
    approved_by_user_at TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE IF NOT EXISTS review_items (
    id TEXT PRIMARY KEY,
    batch_id TEXT NOT NULL REFERENCES review_batches(id) ON DELETE CASCADE,
    file_id TEXT REFERENCES media_files(id) ON DELETE SET NULL,
    work_id TEXT REFERENCES works(id) ON DELETE SET NULL,
    action_kind TEXT NOT NULL
        CHECK (action_kind IN (
            'rename', 'move', 'create_folder', 'move_to_errors',
            'move_to_trash', 'permanent_delete', 'classify_program'
        )),
    source_disk_id TEXT REFERENCES disks(id) ON DELETE RESTRICT,
    old_rel_path TEXT NOT NULL,
    old_name TEXT NOT NULL,
    target_disk_id TEXT REFERENCES disks(id) ON DELETE RESTRICT,
    proposed_rel_path TEXT NOT NULL,
    proposed_name TEXT NOT NULL,
    user_edited_rel_path TEXT,
    user_edited_name TEXT,
    reason_description TEXT NOT NULL DEFAULT '',
    decision_status TEXT NOT NULL DEFAULT 'pending'
        CHECK (decision_status IN (
            'pending', 'approved', 'excluded', 'rejected', 'deferred', 'executed', 'failed', 'rolled_back'
        )),
    conflict_detected INTEGER NOT NULL DEFAULT 0 CHECK (conflict_detected IN (0, 1)),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    decided_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_review_items_batch_status ON review_items(batch_id, decision_status);

-- 17. CRASH-SAFE OPERATION JOURNAL, ROLLBACK STEPS & AUDIT LOG
CREATE TABLE IF NOT EXISTS operation_journal (
    id TEXT PRIMARY KEY,
    batch_id TEXT NOT NULL REFERENCES review_batches(id) ON DELETE RESTRICT,
    review_item_id TEXT NOT NULL REFERENCES review_items(id) ON DELETE RESTRICT,
    step_sequence INTEGER NOT NULL CHECK (step_sequence >= 1),
    operation_state TEXT NOT NULL DEFAULT 'queued'
        CHECK (operation_state IN (
            'queued', 'precheck_passed', 'in_progress', 'verifying_hash',
            'completed', 'interrupted_power_or_disk', 'aborted_target_exists', 'rolled_back'
        )),
    pre_sha256 TEXT,
    post_sha256 TEXT,
    source_snapshot_path TEXT NOT NULL,
    destination_snapshot_path TEXT NOT NULL,
    last_successful_substep TEXT NOT NULL DEFAULT 'init',
    error_message TEXT NOT NULL DEFAULT '',
    started_at TEXT,
    completed_at TEXT,
    UNIQUE (batch_id, step_sequence)
);

CREATE INDEX IF NOT EXISTS idx_operation_journal_state ON operation_journal(operation_state);

CREATE TABLE IF NOT EXISTS rollback_steps (
    id TEXT PRIMARY KEY,
    journal_id TEXT NOT NULL UNIQUE REFERENCES operation_journal(id) ON DELETE CASCADE,
    batch_id TEXT NOT NULL REFERENCES review_batches(id) ON DELETE CASCADE,
    reverse_order INTEGER NOT NULL,
    reverse_action TEXT NOT NULL CHECK (reverse_action IN ('move_back', 'rename_back', 'remove_empty_created_dir', 'restore_from_trash')),
    from_disk_id TEXT NOT NULL REFERENCES disks(id) ON DELETE RESTRICT,
    from_rel_path TEXT NOT NULL,
    to_disk_id TEXT NOT NULL REFERENCES disks(id) ON DELETE RESTRICT,
    to_rel_path TEXT NOT NULL,
    expected_sha256 TEXT,
    rollback_status TEXT NOT NULL DEFAULT 'ready'
        CHECK (rollback_status IN ('ready', 'executed', 'blocked_collision', 'skipped')),
    executed_at TEXT
);

CREATE TABLE IF NOT EXISTS audit_logs (
    id TEXT PRIMARY KEY,
    event_category TEXT NOT NULL
        CHECK (event_category IN ('safety', 'fsops', 'review', 'backup', 'provider', 'disk', 'security', 'settings')),
    severity TEXT NOT NULL DEFAULT 'info' CHECK (severity IN ('info', 'warning', 'error', 'critical')),
    actor TEXT NOT NULL DEFAULT 'system' CHECK (actor IN ('user', 'system', 'watcher', 'scheduler')),
    summary TEXT NOT NULL,
    details_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_created ON audit_logs(created_at DESC);

-- 18. INTERNAL TRASH (Manual Empty ONLY after Explicit Selection & Approval)
CREATE TABLE IF NOT EXISTS trash_items (
    id TEXT PRIMARY KEY,
    file_id TEXT NOT NULL UNIQUE REFERENCES media_files(id) ON DELETE RESTRICT,
    origin_disk_id TEXT NOT NULL REFERENCES disks(id) ON DELETE RESTRICT,
    original_rel_path TEXT NOT NULL,
    trash_rel_path TEXT NOT NULL,
    size_bytes INTEGER NOT NULL CHECK (size_bytes >= 0),
    sha256_hash TEXT,
    trashed_reason TEXT NOT NULL DEFAULT '',
    review_item_id TEXT REFERENCES review_items(id) ON DELETE SET NULL,
    trashed_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    restored_at TEXT,
    purged_manually_at TEXT
);

-- 19. BACKUPS, SETTINGS, NOTIFICATIONS & RESUMABLE BACKGROUND JOBS
CREATE TABLE IF NOT EXISTS backup_records (
    id TEXT PRIMARY KEY,
    backup_kind TEXT NOT NULL CHECK (backup_kind IN ('auto_scheduled', 'pre_migration', 'pre_batch', 'manual_user')),
    archive_path TEXT NOT NULL,
    size_bytes INTEGER NOT NULL CHECK (size_bytes >= 0),
    sha256_checksum TEXT NOT NULL CHECK (length(sha256_checksum) = 64),
    schema_version INTEGER NOT NULL,
    includes_settings INTEGER NOT NULL DEFAULT 1 CHECK (includes_settings IN (0, 1)),
    includes_watch_state INTEGER NOT NULL DEFAULT 1 CHECK (includes_watch_state IN (0, 1)),
    includes_audit_journal INTEGER NOT NULL DEFAULT 1 CHECK (includes_audit_journal IN (0, 1)),
    verification_status TEXT NOT NULL DEFAULT 'verified'
        CHECK (verification_status IN ('verified', 'corrupted', 'missing_file')),
    portable_manifest_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    verified_at TEXT
);

CREATE TABLE IF NOT EXISTS app_settings (
    setting_key TEXT PRIMARY KEY,
    setting_value_json TEXT NOT NULL,
    category TEXT NOT NULL DEFAULT 'general',
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE IF NOT EXISTS notifications (
    id TEXT PRIMARY KEY,
    notification_type TEXT NOT NULL
        CHECK (notification_type IN ('disk_event', 'new_files_detected', 'missing_files', 'review_ready', 'backup_status', 'integrity_alert')),
    title_ar TEXT NOT NULL,
    title_en TEXT NOT NULL,
    body_ar TEXT NOT NULL DEFAULT '',
    body_en TEXT NOT NULL DEFAULT '',
    is_read INTEGER NOT NULL DEFAULT 0 CHECK (is_read IN (0, 1)),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE IF NOT EXISTS background_jobs (
    id TEXT PRIMARY KEY,
    job_type TEXT NOT NULL
        CHECK (job_type IN ('disk_scan', 'sha256_hash', 'media_probe', 'provider_sync', 'image_prefetch', 'backup_verify')),
    job_state TEXT NOT NULL DEFAULT 'queued'
        CHECK (job_state IN ('queued', 'running', 'paused_gracefully', 'resumable_after_restart', 'completed', 'failed', 'cancelled')),
    checkpoint_cursor_json TEXT NOT NULL DEFAULT '{}',
    processed_units INTEGER NOT NULL DEFAULT 0 CHECK (processed_units >= 0),
    total_units INTEGER NOT NULL DEFAULT 0 CHECK (total_units >= 0),
    started_at TEXT,
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    completed_at TEXT
);

-- Seed Default Registered Providers in Safe Initial State
INSERT OR IGNORE INTO data_providers (
    id, display_name, provider_category, integration_status, priority_rank,
    rate_limit_per_minute, requires_api_key, credential_manager_target,
    require_per_request_consent, terms_reviewed, commercial_use_allowed, attribution_requirement
) VALUES
    ('tmdb', 'The Movie Database (TMDb)', 'metadata', 'disabled', 10, 40, 1, 'EstirahaNext/Providers/TMDb', 0, 0, 0, 'TMDb attribution logo & notice required'),
    ('omdb', 'OMDb API', 'metadata', 'disabled', 20, 30, 1, 'EstirahaNext/Providers/OMDb', 0, 0, 0, 'Verify OMDb tier & license before activation'),
    ('tvdb', 'TheTVDB', 'metadata', 'disabled', 30, 30, 1, 'EstirahaNext/Providers/TVDB', 0, 0, 0, 'TVDB attribution & subscription terms check required'),
    ('tvmaze', 'TVMaze', 'metadata', 'disabled', 40, 20, 0, 'EstirahaNext/Providers/TVMaze', 0, 0, 0, 'CC BY-SA / TVMaze attribution check required'),
    ('trakt', 'Trakt', 'tracking', 'disabled', 50, 30, 1, 'EstirahaNext/Providers/Trakt', 0, 0, 0, 'Powered by Trakt attribution required'),
    ('simkl', 'Simkl', 'tracking', 'disabled', 60, 30, 1, 'EstirahaNext/Providers/Simkl', 0, 0, 0, 'Verify Simkl API terms before activation'),
    ('opensubtitles', 'OpenSubtitles', 'subtitles', 'search_only', 70, 20, 1, 'EstirahaNext/Providers/OpenSubtitles', 0, 0, 0, 'Search-only mode; auto-download strictly disabled'),
    ('deepseek', 'DeepSeek AI Assistant', 'ai_assistant', 'disabled', 90, 10, 1, 'EstirahaNext/Providers/DeepSeek', 1, 0, 0, 'Opt-in per-request consent; never send disk paths or third-party provider payloads'),
    ('elcinema', 'ElCinema (السينما.كوم)', 'arabic_cinema', 'deferred_legal_review', 999, 5, 1, 'EstirahaNext/Providers/ElCinema', 1, 0, 0, 'DEFERRED: Do not activate until official/permitted integration method is verified');
