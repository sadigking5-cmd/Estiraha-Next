# ESTIRAHA NEXT — ARCHITECTURAL DECISIONS LOG (`DECISIONS.md`)

## Part A: Confirmed Decisions (Locked in Phase 0)

| ID | Area | Confirmed Decision | Rationale |
|---|---|---|---|
| **ADR-001** | Platform Nature | Native **Windows Desktop Application** (Offline-first local install via NSIS `.exe`). No Hono / Cloudflare Pages / SaaS cloud dependency. | Direct multi-disk NTFS access, zero upload latency for 200k+ files, complete user data sovereignty. |
| **ADR-002** | Core Backend Language | **Go 1.23+** modular monolith (`internal/*` 37 independent packages). | High concurrency for hashing/scanning 200k files, low memory footprint, deterministic error handling, clean Windows cross-compilation. |
| **ADR-003** | Persistence & Local Search | **SQLite 3 (`WAL` mode + Foreign Keys)** with **SQLite FTS5 (`unicode61 remove_diacritics 2` + Arabic Alef/Teh-Marbuta/Alef-Maksura trigger normalization)** via `github.com/mattn/go-sqlite3@v1.14.53` (`-tags "sqlite_fts5"`). | Native C-speed SQLite execution (<80 MiB compile RAM vs >1.2 GiB OOM with transpiled drivers), sub-50ms bilingual Arabic/English full-text search, atomic transactions. |
| **ADR-004** | Immutable Entity Identity | **UUIDv7** string primary keys (`works.id`, `media_files.id`, `people.id`). | Renaming or moving files never breaks watch progress, ratings, metadata claims, or rollback history. |
| **ADR-005** | Title Separation Policy | Separate `title_original`, `title_ar`, and `title_en` columns; zero speculative machine translation of movie/series titles. | Preserves authentic Arabic cinema titles and original foreign titles accurately. |
| **ADR-006** | Safety & Review Gate | Mandatory `review_batches` -> `review_items` -> `ExecutionPermit` -> `operation_journal` -> `rollback_steps`. | Enforces zero unapproved file mutation, zero overwrites, zero auto-delete, and power-loss recovery. |
| **ADR-007** | Secret Storage | **Windows Credential Manager** (`Advapi32` Generic Credentials) referenced only by key name in `data_providers.credential_manager_target`. | OS-encrypted per-user Windows DPAPI protection; zero plaintext secrets in DB or Git. |
| **ADR-008** | Smart Image Engine | 5-tier cache + Memory/Disk LRU (2 GiB initial customizable cap) in `%LOCALAPPDATA%`, JPEG/PNG/WebP magic-byte check, **SVG blocked**, user local images immutable. | High-FPS poster scrolling at 200k scale while preventing XSS/XXE and protecting user folders. |
| **ADR-009** | API Hub Governance | Pluggable `Adapter` per provider; `metadata_claims` provenance & confidence; DeepSeek opt-in per request + path scrubbing; OpenSubtitles `search_only`; ElCinema `deferred_legal_review`. | Legal compliance, attribution tracking, conflict visibility, and strict user privacy. |
| **ADR-010** | Source of Truth & Continuity | Official GitHub repo `https://github.com/sadigking5-cmd/Estiraha-Next.git` + 10 root continuity documents updated at every phase boundary. | Seamless handoff across different Genspark accounts without losing state or architectural context. |

---

## Part B: Open Decisions & Gated Candidates (Require Explicit PoC / License Gate Before Lock-In)

| ID | Candidate / Topic | Current Status | Verification Gate Required Before Final Lock-In |
|---|---|---|---|
| **OPEN-001** | **Wails v3** Desktop Shell | Candidate (Target UI Bridge) | **Phase 1 PoC Gate:** Wails v3 is pre-stable/alpha upstream; must pass a dedicated Windows 10/11 WebView2 stability, RTL rendering, System Tray, custom file-protocol poster streaming, and memory soak PoC. Fallback if PoC fails: Wails v2 stable. |
| **OPEN-002** | **mpv / libmpv** Embedded Player | Candidate (Internal Player) | **License & Integration Gate:** Verify LGPLv2.1+ vs GPLv2+ build flags of distributed `libmpv-2.dll`, hardware decoding (`d3d11va`) inside/alongside Wails WebView2 window, and Arabic ASS/SRT shaping (`libass`). External **VLC** remains supported fallback. |
| **OPEN-003** | **ffprobe** Media Analyzer | Candidate (Stream Prober) | **License & Distribution Gate:** Review FFmpeg/ffprobe LGPL vs GPL binary redistribution obligations for Windows installer (bundled LGPL binary vs user-supplied path detection) + benchmark probing throughput on 200k files. |
| **OPEN-004** | **ElCinema (السينما.كوم)** | **Deferred** (`deferred_legal_review`) | Remains strictly disabled until official terms of service / permitted API or partnership mechanism is legally confirmed. |
| **OPEN-005** | **Commercial/Terms Audit per API Provider** | Pending per-provider legal check | Before enabling TMDb, OMDb, TVDB, TVMaze, Trakt, Simkl, or OpenSubtitles in UI, verify commercial vs non-commercial clauses, rate limits, cache TTL caps, and mandatory UI attribution badges. |
| **OPEN-006** | **RAR v5 Read-Only Header Library** | Candidate selection in Phase 2 | Select pure-Go read-only RAR v4/v5 manifest reader (`nwaples/rardecode/v2` BSD license check) to list archive entries without extraction. |
| **OPEN-007** | **Code Signing Certificate for Windows `.exe`** | Operational decision before public release | Decide on Authenticode OV/EV certificate strategy to prevent Windows SmartScreen warnings on NSIS installer. |
