# Database Constraints, Foreign Keys & 200k-Scale Indexing (`docs/database/CONSTRAINTS_AND_INDEXES.md`)

## 1. Referential Integrity Policies
- `ON DELETE RESTRICT` is enforced on safety-critical parents (`disks`, `countries`, `genres`, `data_providers`, `review_batches` -> `operation_journal`, `media_files` -> `trash_items`) so records tied to physical storage or audit history cannot be orphaned accidentally.
- `ON DELETE CASCADE` is used strictly for owned sub-entities (`works` -> `seasons` -> `episodes`, `duplicate_groups` -> `duplicate_group_members`, `works` -> `works_fts` via triggers).
- `ON DELETE SET NULL` preserves physical `media_files` if a logical grouping (`franchises`, `works`, `episodes`) is unlinked during re-identification.

## 2. Key Performance Indexes for 200,000+ Files
- `idx_media_files_sha256` (partial index `WHERE sha256_hash IS NOT NULL`) for O(log N) exact duplicate clustering.
- `idx_media_files_parent(disk_id, parent_rel_path)` for instant folder-level sidecar & multi-version inspection.
- `idx_media_versions_work(work_id, computed_quality_score DESC)` for instant best-version selection.
- `idx_image_cache_lru(pinned_offline, last_accessed_at ASC)` for O(log N) LRU eviction without table scans.
- `works_fts` & `people_fts` (`fts5` with `unicode61 remove_diacritics 2`) for diacritic-tolerant Arabic (`أحمد` / `احمد`) and English full-text queries.
