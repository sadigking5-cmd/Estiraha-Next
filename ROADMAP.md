# ESTIRAHA NEXT — ENGINEERING ROADMAP (`ROADMAP.md`)

> **Current Active Milestone:** **Phase 0 — Master Project Foundation (COMPLETED — Awaiting User Sign-Off)**

---

## Phase 0: Project Foundation & Architecture Baseline *(Completed)*
- Verify official GitHub repository (`Estiraha-Next.git`) and sandbox toolchain.
- Establish clean 37-package `internal/` modular architecture + `frontend/` TypeScript contracts + `migrations/`.
- Design and validate complete SQLite 3 (`WAL` + Foreign Keys) + FTS5 schema (33 relational tables + 2 FTS5 tables).
- Codify Naming Laws, Safety Laws, API Hub governance, Smart Image Engine rules, Windows NSIS/CI build plan, and Genspark cross-account continuity docs.
- Execute Phase 0 automated unit & integration test suite.

## Phase 1: Windows Desktop PoC & Toolchain Verification Gate
- Execute **Wails v3 Windows PoC** (WebView2 windowing, RTL/LTR switching, System Tray, custom local image protocol).
- Execute **Windows Credential Manager (`Advapi32`)** native secret read/write integration test.
- Complete **ffprobe** and **mpv / VLC** license audit & Windows integration PoC.
- Verify end-to-end **NSIS `.exe` installer** build via GitHub Actions `windows-latest` runner.

## Phase 2: Multi-Disk Registry, Read-Only Scanner & Archive Inspector (200k Scale)
- Hardware volume serial/GUID discovery, connect/disconnect tracking, and capacity analytics (`internal/disks`).
- Resumable, checkpointed directory scanner (`internal/scanner`, `internal/jobs`) tested against a 200,000-synthetic-file benchmark.
- Nested folder detector, sidecar association (`.srt`, `.ass`, `.ssa`, `.vtt`, `.nfo`, `.shorts`, artwork), and read-only `ZIP`/`RAR` inspection.
- Read-only System Tray Watcher (`internal/watcher`, `internal/tray`) with bilingual notifications.

## Phase 3: Media Stream Probing, Quality Ranking & SHA-256 Deduplication
- Technical video/audio stream probing (`internal/media`) and deterministic quality scoring.
- Auto-selection of best version + manual override (`media_versions`).
- Two-stage partial fingerprint + full `SHA-256` exact duplicate detection vs. alternate quality versions + cross-disk `Keep` rules (`internal/duplicates`).

## Phase 4: Domain Organizers & Crash-Safe Review Center
- Full domain organizers for all 12 media types (`movies`, `series`, `programs`, `documentaries`, `people`, `collections`).
- Non-destructive Before/After Organization Preview & Review Center (`internal/organizer`, `internal/review`).
- Crash-safe staged `fsops` executor, `operation_journal` power-loss recovery, `rollback_steps` undo engine, and manual-empty `trash`.

## Phase 5: API Hub, Identification Engine & Smart Image Engine
- Isolated provider adapters (beginning with verified/permitted providers), rate limiter, TTL cache, `metadata_claims` confidence & conflict viewer.
- Opt-in per-request privacy-scrubbed DeepSeek helper & search-only OpenSubtitles adapter.
- Full 2 GiB LRU Smart Image Engine (`internal/imagecache`, `internal/artwork`) with thumbnail generation, prefetching, shimmer states, and offline mode.

## Phase 6: Bilingual FTS5 Dual Search, Cinematic RTL/LTR UI & Hybrid Player
- Standard + Advanced multi-facet + Natural Language AR/EN search (`internal/search`) separating Local vs. External works.
- Full Cinematic Dark/Light UI (Arabic RTL Right-Sidebar / English LTR Left-Sidebar), keyboard shortcuts, virtualized grids/tables.
- Integrated media player + VLC fallback, Hybrid watch tracking, resume playback, 0–10 ratings, notes, and custom watchlists.

## Phase 7: Customizable Dashboard, Excel/PDF Reports, Portable Backups & Release 1.0
- Customizable KPI Dashboard & Excel/PDF report exporter (`internal/reports`).
- Automated & manual SHA-256 verified backups + cross-machine portable restoration (`internal/backup`).
- Full 200,000-file stress acceptance suite, security audit, and signed Windows NSIS v1.0 release.
