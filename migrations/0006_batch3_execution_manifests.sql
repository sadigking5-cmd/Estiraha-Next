-- ==============================================================================
-- ESTIRAHA NEXT — MIGRATION 0006: BATCH 3 IMMUTABLE EXECUTION MANIFESTS,
-- SAFE PAUSE/CANCEL/RECOVERY TRACKING & ERROR CENTER BATCH RESTORE INDEXES
-- Preserves 100% of existing UUIDs, works, media_files, review_batches, and settings.
-- ==============================================================================

CREATE TABLE IF NOT EXISTS execution_manifests (
    id TEXT PRIMARY KEY,
    batch_id TEXT NOT NULL REFERENCES review_batches(id) ON DELETE CASCADE,
    manifest_sha256 TEXT NOT NULL CHECK (length(manifest_sha256) = 64),
    manifest_json TEXT NOT NULL,
    summary_json TEXT NOT NULL DEFAULT '{}',
    total_approved_steps INTEGER NOT NULL DEFAULT 0 CHECK (total_approved_steps >= 0),
    executed_steps INTEGER NOT NULL DEFAULT 0 CHECK (executed_steps >= 0),
    manifest_status TEXT NOT NULL DEFAULT 'locked'
        CHECK (manifest_status IN ('locked', 'executing', 'paused', 'cancelled', 'completed', 'halted_on_interrupt', 'rolled_back')),
    pre_batch_backup_id TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    completed_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_execution_manifests_batch ON execution_manifests(batch_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_error_folder_entries_batch_status ON error_folder_entries(batch_id, entry_status);
