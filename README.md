# Estiraha Next — Windows Desktop Media Library Management System

**Estiraha Next (استراحة نكست)** is a native, offline-first **Windows 10/11 (x64) Desktop** media library indexing, organization, deduplication, subtitle, health-reporting, watch-tracking, and integrated playback application built with **Go 1.27.4 + Wails v3.1.0 + React 18 + TypeScript + SQLite 3 (WAL + Foreign Keys + Bilingual FTS5) + Windows Credential Manager + NSIS**.

> **Scope Notice:** Estiraha Next is a locally installed Windows desktop application (`EstirahaNext-Setup-0.2.0-phase-b-x64.exe`). It is **not** a web or cloud service.

## Official Repository & Branches
- **Repository:** `https://github.com/sadigking5-cmd/Estiraha-Next.git`
- **Active Phase B Branch:** `phase-b/media-library-system` (`v0.2.0-phase-b`)
- **Phase 1A Verified PoC Branch:** `phase1a/wails-v3-poc`

## Completed Features (Phase B — Sub-Phases B1–B12)
1. **Multi-Disk Storage & Manual Scanner (B1):** Register drives & roots by content type, reject duplicate/overlapping roots, preserve offline drive catalogs safely, passive folder-change indicators, and strictly **Manual** scan execution with safe cancellation.
2. **Multi-Signal Media Identification (B2):** Combines folder layout, category, `SxxExx`/Season patterns, Arabic script detection, companion file linking, and optional TMDb lookup; routes uncertain items to Review (`Unidentified Files = Review`).
3. **Movie Library & Series/Episodes Management (B3 & B4):** Instant bilingual Arabic/English FTS5 search, multi-version inspection, manual metadata editing (`locked_by_user`), yearless Series folders (`Series Name/S01/01.mp4`), and missing season/episode gap detection.
4. **Canonical Naming & Mandatory `.shorts` Law (B5):** Enforces `Movie Name YYYY` (no parentheses or release tags; authentic Arabic preserved without speculative translation) and strictly protects `.shorts` companion files from stripping, deletion, or conversion.
5. **Duplicates & Best-Version Selection (B6):** Multi-factor quality scoring (resolution + codec + HDR + integrity + size density) while preserving copies across separate drives (`Cross-Disk Copies = Keep`).
6. **Smart Review Center, Safe Organizer & 1-Click Rollback (B7 & B8):** Zero file moves/renames without explicit user approval, automatic verified `pre_batch` SQLite backup (`VACUUM INTO` + `PRAGMA integrity_check` + `SHA-256` + JSON manifest), zero overwrites, post-move `SHA-256` verification, immutable UUID retention, and 1-click `RollbackBatch`.
7. **Missing Items & Health Reports (B9):** Unified reporting across missing episodes, missing seasons, missing subtitles, missing posters, incomplete metadata, unidentified files, 0-byte/corrupted files, same-disk duplicates, and offline drives.
8. **Integrated Media Player & Watch Tracking (B10):** Loopback (`127.0.0.1:0`) HTTP Byte-Range video streaming, on-the-fly UTF-8 `SRT`/`ASS` -> `WebVTT` subtitle conversion, local poster serving (SVG blocked), resume position tracking, and optional external player fallback.
9. **Hybrid Cinematic + Administrative Bilingual UI (B11):** 10 complete workspaces with automatic directional layout (`Arabic RTL` right-sidebar / `English LTR` left-sidebar) and System Tray integration.

## Verification & Windows Build Commands
```bash
# Run full TypeScript/Vite build, Linux & Windows Go vet, Unit + Phase B E2E tests, and CLI self-check
bash scripts/verify_foundation.sh

# Cross-compile Windows x64 EstirahaNext.exe and package NSIS Setup .exe
bash scripts/build_windows_poc.sh
```
