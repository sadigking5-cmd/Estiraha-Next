# ESTIRAHA NEXT — MODULE-BY-MODULE TEST PLAN (`TEST_PLAN.md`)

## 1. Testing Principles & Safety Constraints
- **Never Touch Real User Media:** All automated tests run exclusively against `:memory:` SQLite databases or isolated ephemeral temp directories (`t.TempDir()`) containing synthetic zero-media fixtures.
- **Four Verification Layers:**
  1. **Unit Tests (`tests/unit` & package `*_test.go`):** Pure deterministic logic per module.
  2. **Integration Tests (`tests/integration`):** SQLite WAL + Foreign Keys + FTS5 triggers + pre-migration backup + cross-module workflows.
  3. **Scale & Fault-Injection Tests:** 200,000-row catalog/file synthetic benchmark + simulated mid-batch power/disk interruption recovery.
  4. **Windows Native Acceptance (`tests/acceptance` & GitHub Actions `windows-latest`):** NTFS path rules, Credential Manager DPAPI, Wails v3 UI, mpv/VLC launch, NSIS `.exe` install/uninstall safety.

---

## 2. Per-Module Test Matrix (All Core Engines)

| Module | Planned Test Coverage | Phase Target |
|---|---|---|
| `config` | Default safety invariants; rejection of any config attempting to enable auto-move, auto-delete, overwrite, SVG, or unconsented DeepSeek. | **Phase 0 (Executed)** |
| `domain` | UUIDv7 uniqueness, format stability, completeness of all 12 media types. | **Phase 0 (Executed)** |
| `db` | Embedded migrations `0001` & `0002`, idempotency, checksum tamper detection, pre-migration SHA-256 `.db.bak` creation, FK enforcement, FTS5 Arabic/English queries. | **Phase 0 (Executed)** |
| `naming` | Movie `Name YYYY` cleaning (strip brackets, dots, codecs, groups), Arabic movie retention, Series yearless folder + `S01/01.mp4`, Franchise yearless root, Celebrity `الاسم بالعربية (English Name)`, Known vs Unknown program routing. | **Phase 0 (Executed)** |
| `review` & `fsops` | Block execution without approved `ExecutionPermit`; block overwrite when target file exists; verify missing-source handling. | **Phase 0 (Executed)** |
| `providers` (API Hub) | Block unknown providers, block deferred `elcinema`, enforce `terms_reviewed`, enforce per-request consent & Windows/UNC path leak blocking for `deepseek`, enforce `search_only` on `opensubtitles`. | **Phase 0 (Executed)** |
| `imagecache` | Accept valid JPEG/PNG/WebP magic bytes; block SVG (MIME & `<svg` payload); enforce size/dimension caps; protect `user_local` assets; block cache paths inside user library roots; LRU eviction at 2 GiB cap (Phase 5). | **Phase 0 (Core Guards Executed)** / Phase 5 (LRU Disk I/O) |
| `secrets` | Vault CRUD contract (`Set`/`Get`/`Delete`) + Windows Credential Manager native `Advapi32` roundtrip on `windows-latest`. | **Phase 0 (Vault Contract)** / Phase 1 (Win32 Native) |
| `disks` & `watcher` | Volume serial mapping, offline/online state transition, verification that watcher emits read-only events with zero write syscalls. | Phase 2 |
| `scanner` | Nested directory discovery, `.srt`/`.ass`/`.ssa`/`.vtt`/`.nfo`/`.shorts` grouping, read-only ZIP/RAR manifest parsing without extraction, 200k synthetic file throughput & pause/resume checkpointing. | Phase 2 |
| `media` & `duplicates` | Stream quality score ranking, best-version auto-pick + manual override, partial hash + SHA-256 exact match across renamed files, cross-disk `Keep` preservation. | Phase 3 |
| `movies`, `series`, `programs`, `documentaries`, `people`, `collections` | Category-specific organization plan generation, missing episode detection, non-duplicating celebrity filmography split (Movies with year / Series without year). | Phase 4 |
| `organizer`, `audit`, `trash` | Diff generation, staged execution, simulated mid-move disk disconnect & rollback verification, manual-only trash purge gate. | Phase 4 |
| `search` | FTS5 Arabic diacritic-insensitive matching, multi-facet filtering (actor, director, country, year, genre, quality, disk, watch status, duplicate status), NL query parsing, Local vs External separation. | Phase 6 |
| `player` & `progress` | Resume timestamp persistence, version/audio/subtitle switching, subtitle sync offset, Hybrid auto+manual watch state transitions, 0–10 rating bounds. | Phase 6 |
| `backup` & `reports` | SHA-256 backup archive verification, corrupted backup rejection, cross-machine path remapping restore, Excel/PDF export schema accuracy. | Phase 7 |
