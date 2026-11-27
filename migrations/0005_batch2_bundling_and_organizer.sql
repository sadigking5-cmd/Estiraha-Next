-- ==============================================================================
-- ESTIRAHA NEXT — MIGRATION 0005: BATCH 2 MOVIE BUNDLING, MULTI-VERSION IDENTITY
-- & SMART ORGANIZER + MANAGED ERROR FOLDER INTEGRATION
-- Preserves 100% of existing UUIDs, works, media_files, and user settings.
-- ==============================================================================

ALTER TABLE works ADD COLUMN normalized_identity_key TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_works_identity_key ON works(media_type, production_year, normalized_identity_key);

ALTER TABLE review_items ADD COLUMN item_file_kind TEXT NOT NULL DEFAULT 'file'
    CHECK (item_file_kind IN ('file', 'directory'));
ALTER TABLE review_items ADD COLUMN error_category_code TEXT NOT NULL DEFAULT ''
    CHECK (error_category_code IN (
        '',
        'empty_folders',
        'zero_byte_files',
        'unlinked_subtitles',
        'auxiliary_or_unknown',
        'samples_and_short_clips',
        'conflicts_needing_review'
    ));
ALTER TABLE review_items ADD COLUMN safety_checks_json TEXT NOT NULL DEFAULT '[]';

ALTER TABLE rollback_steps ADD COLUMN error_entry_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_rollback_steps_error_entry ON rollback_steps(error_entry_id) WHERE error_entry_id != '';
