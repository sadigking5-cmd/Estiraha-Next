# Estiraha Next — Windows Desktop Media Library System

**Estiraha Next** is a native, offline-first **Windows Desktop** media library indexing, organization, deduplication, artwork, subtitle, watch-tracking, and playback platform engineered in **Go + Wails v3 (PoC-gated) + React/TypeScript + SQLite 3 (FTS5)**.

> **Important Scope Notice:** Estiraha Next is a locally installed Windows desktop system (`NSIS .exe`) built to handle **200,000+ local media files** across multiple physical drives. It is **not** a cloud web application.

## Official Repository
- **GitHub (Single Source of Truth):** `https://github.com/sadigking5-cmd/Estiraha-Next.git`
- **Active Branch:** `main`
- **Current Milestone:** **Phase 0 — Official Project Foundation (Completed)**

## Core Capabilities (Target Architecture)
- **12 Supported Media Categories:** Movies, Series, TV Programs, Talk Shows, Game Shows, Sports, Entertainment, Standalone Documentaries, Documentary Series, Anime, Cartoons, and Cinematic Franchises & Celebrity Filmographies.
- **Deterministic Naming Engine:** Strict enforcement of `Movie Name YYYY` (no brackets/tags/dots), yearless Series roots (`S01/01.mp4`), yearless Franchise roots, and `الاسم بالعربية (English Name)` Celebrity profiles without physical file duplication.
- **Zero-Risk Review Center:** Side-by-side before/after path previews, inline edits, exclusions, zero automatic moves/deletes, zero file overwrites, power-loss `operation_journal` recovery, reversible `rollback_steps`, and manual-empty Trash.
- **Quality & SHA-256 Deduplication:** Stream quality ranking, exact SHA-256 content duplicate detection vs. alternate-quality versions, and cross-disk `Keep` preservation.
- **API Hub & Smart Image Engine:** Isolated provider adapters with confidence/conflict tracking, Windows Credential Manager secret protection, and a 2 GiB LRU image cache (JPEG/PNG/WebP verified; SVG blocked; user local artwork protected).
- **Bilingual Offline-First UX:** Instant local SQLite FTS5 search in Arabic (`RTL` — right sidebar) and English (`LTR` — left sidebar).

## Essential Governance & Continuity Documents
1. [`PROJECT_STATUS.md`](./PROJECT_STATUS.md) — Current milestone status & verification checklist
2. [`HANDOFF.md`](./HANDOFF.md) — Cross-account Genspark onboarding & continuity protocol
3. [`PROJECT_SPEC.md`](./PROJECT_SPEC.md) — Complete functional specification
4. [`ARCHITECTURE.md`](./ARCHITECTURE.md) — System & 37-package modular design
5. [`DATABASE_SCHEMA.md`](./DATABASE_SCHEMA.md) — SQLite 3 + FTS5 tables, constraints & ER diagram
6. [`NAMING_RULES.md`](./NAMING_RULES.md) — Mandatory naming conventions
7. [`SAFETY_RULES.md`](./SAFETY_RULES.md) — 12 non-negotiable file protection laws
8. [`DECISIONS.md`](./DECISIONS.md) — Confirmed ADRs & open PoC/license gates
9. [`ROADMAP.md`](./ROADMAP.md) — Phased implementation plan
10. [`TEST_PLAN.md`](./TEST_PLAN.md) — Module-by-module test matrix

## Quick Foundation Verification
```bash
bash scripts/verify_foundation.sh
```
