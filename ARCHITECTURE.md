# ESTIRAHA NEXT — SYSTEM ARCHITECTURE (`ARCHITECTURE.md`)

## 1. High-Level Architectural Style
Estiraha Next is designed as a **Modular Layered Desktop Monolith** written in **Go** (Core Engine) paired with **Wails v3** (subject to Phase 1 Windows PoC gate) and **React + TypeScript** (Presentation Layer), backed by a hardened **Local SQLite 3 + FTS5** persistence engine.

```
┌────────────────────────────────────────────────────────────────────────────┐
│                   PRESENTATION LAYER (React + TypeScript)                  │
│  RTL (Arabic / Right Sidebar) & LTR (English / Left Sidebar) | Dark/Light  │
│  Dashboard | Unified Search | Library Views | Review Diff | Player Overlay │
└─────────────────────────────────────┬──────────────────────────────────────┘
                                      │ Typed Wails v3 IPC Bindings & Events
┌─────────────────────────────────────▼──────────────────────────────────────┐
│                        APPLICATION & API BOUNDARY                          │
│      internal/app  |  internal/api  |  internal/jobs  |  internal/events   │
│      internal/tray |  internal/i18n |  internal/notifications              │
└──────────┬──────────────────────────┬───────────────────────────┬──────────┘
           │                          │                           │
┌──────────▼───────────┐  ┌───────────▼────────────┐  ┌───────────▼──────────┐
│  LIBRARY & MEDIA     │  │  GOVERNANCE & SAFETY   │  │  EXTERNAL & CACHING  │
│  internal/disks      │  │  internal/review       │  │  internal/providers  │
│  internal/watcher    │  │  internal/organizer    │  │  internal/identify   │
│  internal/scanner    │  │  internal/naming       │  │  internal/imagecache │
│  internal/media      │  │  internal/fsops        │  │  internal/artwork    │
│  internal/duplicates │  │  internal/trash        │  │  internal/secrets    │
│  internal/subtitles  │  │  internal/audit        │  │  (Win Cred Manager)  │
│  movies/series/etc.  │  │  internal/backup       │  │                      │
└──────────┬───────────┘  └───────────┬────────────┘  └───────────┬──────────┘
           └──────────────────────────┼───────────────────────────┘
                                      ▼
┌────────────────────────────────────────────────────────────────────────────┐
│               PERSISTENCE & INDEXING LAYER (internal/db)                   │
│   SQLite 3 (WAL Mode, Foreign Keys ON,Immutable UUIDs) + SQLite FTS5       │
│   Stored strictly in %LOCALAPPDATA%\EstirahaNext\ (Never in media dirs)    │
└────────────────────────────────────────────────────────────────────────────┘
```

## 2. Module Responsibilities (`internal/` — 37 Independent Packages)

| Package | Single Responsibility |
|---|---|
| `app` | Application bootstrap, lifecycle hooks, clean shutdown, resumable startup recovery |
| `api` | Typed IPC command/query endpoints exposed to Wails v3 frontend |
| `config` | Configuration loading, defaults, and immutable safety invariant validation |
| `logging` | Structured local diagnostic logs with secret & path redaction |
| `db` | SQLite connection pool, WAL/FK pragmas, SHA-256 pre-migration backup, atomic migrations |
| `domain` | Core entities, immutable UUIDv7 generator, canonical enumerations |
| `library` | Unified media catalog orchestration across all 12 media types |
| `disks` | Volume serial/GUID registry, mount detection, capacity comparison, cross-disk replica map |
| `scanner` | 200k-file streaming directory walker, nested folder detector, read-only ZIP/RAR inspector |
| `identify` | Local title/year parser, NFO reader, and claim resolver (zero speculative translation) |
| `providers` | **API Hub**: adapter registry, rate limiter, consent/privacy guard, claim confidence & conflict engine |
| `media` | Video/audio stream analyzer (`ffprobe` candidate adapter) & quality score calculator |
| `movies` | Standalone movie rules, multi-version ranking, extra-file classification |
| `series` | Series/Anime/Cartoon season-episode hierarchy (`S01/01.mp4`) & missing-episode detector |
| `programs` | Known Program season manager vs. Unknown Program preservation + Review routing |
| `documentaries` | Standalone documentary (Movie rules) & Documentary Series (Series rules) engine |
| `people` | Celebrity profiles (`الاسم بالعربية (English Name)`) & virtual credit linking without file duplication |
| `collections` | Cinematic Franchises (yearless parent folder) & custom/smart collections |
| `duplicates` | Two-stage fingerprint + full SHA-256 exact duplicate & alternate-version classifier |
| `subtitles` | External (`SRT`,`ASS`,`SSA`,`VTT`) & embedded track matcher, sync offset, extra detector |
| `artwork` | Primary poster/backdrop selector, local user-image protection, extra image detector |
| `imagecache` | **Smart Image Engine**: Memory + 2GB LRU Disk cache, magic-byte/dimension validator, SVG blocker |
| `search` | Dual search (Standard + Advanced facets + Bilingual NL parser) over local FTS5 & optional providers |
| `review` | Approval gatekeeper, before/after path preview, per-item edit/exclude/defer state machine |
| `organizer` | Non-destructive organization planner producing Review Batches |
| `naming` | Deterministic canonical naming engine enforcing Section 5 rules |
| `fsops` | Permit-gated, collision-proof, journaled file move/rename executor |
| `trash` | Internal recovery vault with manual-only purge after explicit approval |
| `player` | Internal `mpv` candidate controller + external `VLC` launcher |
| `progress` | Hybrid watch status, resume position, 0–10 rating, personal notes, watchlists |
| `backup` | SHA-256 verified SQLite + settings + journal backup & cross-PC restore |
| `reports` | Dashboard KPI aggregator & Excel/PDF report generator |
| `notifications` | Bilingual AR/EN notification dispatcher |
| `audit` | Crash-safe `operation_journal`, power-loss checkpointing, and `rollback_steps` |
| `secrets` | Windows Credential Manager (`Advapi32`) integration for API keys |
| `watcher` | Strictly read-only filesystem event monitor (detect & alert only) |
| `tray` | Windows System Tray controller for background disk monitoring |
| `jobs` | Resumable background worker queue persisted in `background_jobs` |
| `events` | Typed internal pub/sub bus |
| `i18n` | Arabic RTL / English LTR localization catalogs |

## 3. API Hub Architecture
Detailed in `docs/architecture/API_HUB.md`. Key architectural pillars:
- **Strict Adapter Isolation:** Every provider (`tmdb`, `omdb`, `tvdb`, `tvmaze`, `trakt`, `simkl`, `opensubtitles`, `deepseek`, `elcinema`) implements an isolated `Adapter` contract.
- **No Auto-Binding:** Unknown services discovered via `public-apis` catalog are never connected automatically; legal terms, cost, rate limits, caching rules, and attribution requirements must be explicitly reviewed and marked `terms_reviewed = 1`.
- **Provenance & Conflict Engine:** Provider outputs are stored as immutable `metadata_claims` rows with `provider_id`, `confidence_score`, and `has_conflict` rather than blindly overwriting `works`.
- **Provider-Specific Controls:**
  - **DeepSeek:** Disabled by default; requires explicit **per-request user consent**; outbound payload scrubber strips all local disk paths (`D:\...`, `\\UNC\...`) and third-party provider raw data.
  - **ElCinema:** Locked in `deferred_legal_review` mode until an official/permitted integration method is verified.
  - **OpenSubtitles:** Locked in `search_only` mode initially; automatic subtitle downloading is blocked at the hub gate.

## 4. Smart Image Engine Architecture
Detailed in `docs/architecture/SMART_IMAGE_ENGINE.md`. Key architectural pillars:
- **5 Specialized Cache Tiers:** Poster Cache, Backdrop Cache, Celebrity Image Cache, Series Artwork Cache, and Generated Thumbnails.
- **Two-Level Caching:** Fast bounded in-memory LRU + Disk LRU initialized at **2 GiB** (`2,147,483,648` bytes, user-configurable) stored exclusively under `%LOCALAPPDATA%\EstirahaNext\ImageCache\`.
- **Zero Library Pollution:** `VerifyCacheDirIsolated` guarantees temporary/cached files are never written inside user media folders.
- **Security & Validation Gate:** Validates HTTP Content-Type, binary magic bytes (`JPEG`, `PNG`, `WebP` only), max file size, and pixel dimensions (`<= 8192px`) before decode. **SVG is unconditionally rejected** to prevent XML/XSS/XXE vectors.
- **User Asset Immunity:** Images owned locally by the user (`origin_type = 'user_local'`, `is_protected_user_asset = 1`) are never evicted, overwritten, or deleted.
- **UX States:** Supports Shimmer skeleton loading, deterministic offline fallback placeholders, smart viewport prefetching, and provider-compliant TTL expiration.

## 5. Scalability Design for 200,000+ Files
1. **Two-Phase Duplicate Hashing:** Size grouping -> fast 64KB head/tail `partial_fingerprint` -> full streaming `SHA-256` only on size+fingerprint collisions, eliminating 95%+ unnecessary disk I/O.
2. **SQLite WAL + Batched Transactions:** Scanner commits discovered files in bounded batches (e.g., 500–1,000 rows/tx) with prepared statements and covering indexes.
3. **Keyset Pagination & Virtualized Lists:** UI queries use cursor/keyset pagination + virtualized grids so memory usage stays flat regardless of library size.
4. **Pure-Go Cross-Compilable SQLite + FTS5:** `modernc.org/sqlite` provides embedded FTS5 `unicode61 remove_diacritics 2` indexing for sub-50ms bilingual local queries.
