# ESTIRAHA NEXT — PROJECT STATUS (`PROJECT_STATUS.md`)

- **Project Name:** Estiraha Next
- **Official Repository:** `https://github.com/sadigking5-cmd/Estiraha-Next.git`
- **Active Branch:** `phase-b/media-library-system` (built incrementally on approved Phase 1A baseline `23c620b`)
- **Current Phase:** **Phase B — Full Media Library Management System (Sub-Phases B1–B12 Implemented & Verified)**
- **Version:** `0.2.0-phase-b`
- **Last Updated:** 2026-11-24

---

## 1. Phase B Completed Sub-Phases & Verification Matrix
- [x] **SQLite Schema v3 (`migrations/0003_phase_b_library_management.sql`):** Extended `works` & `review_items`, added `library_issues` (all 8 Phase B9 categories), seeded bilingual `countries` & `genres`, and persisted all 17 official Phase B binding decisions in `app_settings` (`WAL`, `PRAGMA foreign_keys = ON`, `FTS5` Arabic diacritic-normalized triggers, 53 registered tables).
- [x] **B1 — Multi-Disk Roots & Manual Scanner (`internal/library`, `internal/scanner`):** Multi-disk registration (`By Content Type`), duplicate/overlapping root rejection, offline disk preservation without deleting missing records, passive read-only change monitoring (`Notifications + Monitoring = Both`), strictly **Manual** scan trigger (`Scan Trigger = Manual`), concurrent-scan lock, and safe cancellation.
- [x] **B2 — Multi-Signal Identification (`internal/identify`):** Combines folder hierarchy, category, `SxxExx`/Season patterns, Arabic script detection, companion linking (`.srt`, `.jpg`, `.shorts`), and optional user-initiated TMDb lookup via Windows Credential Manager; enforces `Unidentified Files = Review`.
- [x] **B3 — Movie Library Module (`internal/movies`):** Bilingual browsing, instant FTS5 search, multi-filter support, detail view with versions/subtitles/`.shorts`/disk paths, and manual metadata override (`locked_by_user`).
- [x] **B4 — Series, Seasons & Episodes Module (`internal/series`):** Consistent `Same` classification, yearless canonical Series folders (`Series Name/S01/01.mp4`), missing season & missing episode gap detection (`Missing Detection = Enabled`), and best episode version ranking.
- [x] **B5 — Deterministic Naming & `.shorts` Protection (`internal/naming`):** Enforces `Movie Name YYYY` (no parentheses, no codec/quality tags, authentic Arabic retained without speculative translation), yearless Series roots (`Series Name/poster.jpg`, `S01/01.ext`), and strict protection of `.shorts` across scanning, naming, review edits, execution, and rollback.
- [x] **B6 — Duplicates & Best-Version Engine (`internal/duplicates`):** Evaluates resolution + codec + HDR + file integrity + size density (never size alone; 0-byte/corrupted files score `0`); strictly enforces `Cross-Disk Copies = Keep`.
- [x] **B7 & B8 — Smart Review Center, Safe Organizer & Rollback (`internal/review`, `internal/organizer`):** Non-destructive before/after preview batches, mandatory user approval gate, automatic verified `pre_batch` SQLite backup + manifest before execution, zero overwrites, pre/post-move `SHA-256` verification, immutable file UUID preservation, and 1-click full `RollbackBatch`.
- [x] **B9 — Missing Items & Library Health Reports (`internal/reports`):** Detects missing episodes, missing seasons, missing subtitles, missing posters, incomplete metadata, unidentified files, 0-byte/corrupted files, same-disk duplicates, and offline disks.
- [x] **B10 — Integrated Media Player & Progress Tracking (`internal/player`):** Local-only (`127.0.0.1:0`) token-authenticated HTTP Byte-Range video streamer, on-the-fly UTF-8 `SRT`/`ASS` -> `WebVTT` subtitle converter, safe JPEG/PNG/WebP artwork server (SVG blocked), `watch_states` + `playback_history` resume persistence, next/previous episode navigation, and optional system player fallback.
- [x] **B11 — Hybrid Cinematic + Administrative Desktop UI (`frontend/src`):** Complete bilingual React 18 + TypeScript + Wails v3 interface across all **10 workspaces** (`Arabic RTL` right-sidebar + `English LTR` left-sidebar, Dark/Light themes, zero mock buttons).
- [x] **B12 — End-to-End Verification & Windows x64 NSIS Release (`tests/integration/phase_b_system_test.go`):** Verified E2E safety, `.shorts` tamper blocking, cross-disk `Keep`, HTTP 206 byte-range streaming, WebVTT conversion, rollback, backup corruption detection, zero API key leakage in SQLite, 5,000+ item FTS5 benchmark, `EstirahaNext.exe` (`PE32+ x86-64 GUI`), and `EstirahaNext-Setup-0.2.0-phase-b-x64.exe` (`NSIS`).
