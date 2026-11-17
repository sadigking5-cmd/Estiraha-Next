# ESTIRAHA NEXT — OFFICIAL PROJECT SPECIFICATION (`PROJECT_SPEC.md`)

## 1. Project Identity & Mission
- **Official Name:** Estiraha Next
- **Platform Target:** Native Windows Desktop Application (Windows 10/11 x64), installable locally via NSIS `.exe` installer.
- **Nature of System:** Strictly a local desktop application — **NOT** a cloud SaaS, **NOT** a web hosting app, and **NOT** built on Hono or Cloudflare Pages.
- **Official Permanent Repository:** `https://github.com/a73971054-lab/Estiraha-Next.git`
- **Development Environment:** Genspark Super App — GenCode (multi-account continuity backed by GitHub as single source of truth).

## 2. Core Engineering Objectives
1. **High Speed & Scale:** Smoothly index, search, filter, and manage libraries containing **200,000+ physical files** across multiple internal/external disks.
2. **Offline-First Autonomy:** All local library browsing, FTS5 search, watch tracking, organization previews, duplicate analysis, and playback work 100% offline.
3. **Crash & Restart Resilience:** Graceful job checkpointing and atomic operation journals allow closing and reopening the app—or recovering from power loss/disk unplug—without data loss or corruption.
4. **Zero Unapproved File Mutation:** No file or directory is ever renamed, moved, replaced, or deleted without explicit user review and approval.
5. **Bilingual Native UX:** Full Arabic (`RTL` — sidebar right) and English (`LTR` — sidebar left) support in Cinematic Dark & Light themes.

## 3. Supported Media Categories (12 Types)
Unified search across all categories with specialized organization rules per category:
1. Movies (`movie`)
2. TV Series (`series`)
3. General TV Programs (`tv_program`)
4. Talk Shows (`talk_show`)
5. Game / Competition Shows (`game_show`)
6. Sports Programs (`sports_program`)
7. Entertainment Shows (`entertainment_program`)
8. Standalone Documentary Movies (`documentary_movie`)
9. Episodic Documentary Series (`documentary_series`)
10. Anime (`anime`)
11. Cartoons (`cartoon`)
12. Franchise & Celebrity-Linked Works (`franchise_entry` + virtual `work_credits` links)

## 4. Major Functional Subsystems
1. **Multi-Disk & Read-Only Watcher Engine:** Volume serial/GUID tracking, connect/disconnect detection, free space comparison, replica location mapping, System Tray background monitoring (alert-only; zero file writes).
2. **Scanner & Folder Intelligence:** Nested directory detection, sidecar grouping (`.srt`, `.ass`, `.ssa`, `.vtt`, posters, `.nfo`, `.shorts`), read-only `ZIP`/`RAR` archive manifest inspection (no auto-extraction).
3. **Quality Scoring & Duplicate Engine:** Stream probing (`ffprobe` candidate pending license verification), best-version auto-selection with manual override, SHA-256 exact content deduplication vs. alternate-quality version retention, cross-disk `Keep` decisions.
4. **Official Naming & Review Center:** Before/after path diffing, per-batch & per-item approval, inline name editing, exclusion/deferral, staged execution, SHA-256 post-move verification, reversible rollback, and manual-empty internal Trash.
5. **API Hub:** Pluggable provider adapters (TMDb, OMDb, TVMaze, TVDB, Trakt, Simkl, OpenSubtitles [search-only], DeepSeek [opt-in per request, privacy-scrubbed], ElCinema [deferred pending legal/integration verification]), rate limiting, caching, per-claim provenance & confidence scores, conflict surfacing.
6. **Smart Image Engine:** Dedicated caches (Poster, Backdrop, Celebrity, Series Artwork, Thumbnails), Memory + Disk LRU (2 GiB initial customizable budget), strict MIME/magic-byte/dimension checks (SVG blocked), user-owned local image protection, zero cache files inside user media folders.
7. **Dual & Natural Search:** Standard + Advanced multi-facet search + bilingual natural-language query parser over SQLite FTS5; clear visual separation between Local Library items and External Catalog results.
8. **Hybrid Media Player & Watch Tracking:** Embedded player candidate (`mpv` pending PoC/license review) + external `VLC` fallback, resume position, version/audio/subtitle switching, subtitle sync offset, playback speed, next-episode navigation, Hybrid watch states, 0–10 ratings, personal notes, and custom watchlists.
9. **Dashboard, Reports & Portable Backups:** Customizable statistics cards, Excel/PDF report exports, automated/manual SHA-256 verified backups, cross-machine restore, and Windows Credential Manager secret storage.
