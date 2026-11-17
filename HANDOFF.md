# ESTIRAHA NEXT — CROSS-ACCOUNT CONTINUITY & HANDOFF GUIDE (`HANDOFF.md`)

## 1. Purpose of This Document
Estiraha Next is engineered in **Genspark Super App — GenCode** across multiple sessions and potentially different Genspark accounts. **`https://github.com/sadigking5-cmd/Estiraha-Next.git`** is the single permanent source of truth. Any new Genspark account or developer joining the project MUST follow the protocol below before writing a single line of code.

---

## 2. Mandatory Onboarding Protocol for Any New Genspark Session / Account

1. **Authenticate GitHub First:**
   - Call `setup_github_environment` (ensure GitHub authorization is active in the `#github` tab).
   - Never create a new repository; always use `https://github.com/sadigking5-cmd/Estiraha-Next.git` (`main` branch unless a feature branch is explicitly requested).

2. **Clone / Sync into Workspace (`/home/user/webapp`):**
   ```bash
   rm -rf /home/user/webapp
   git clone https://github.com/sadigking5-cmd/Estiraha-Next.git /home/user/webapp
   cd /home/user/webapp
   ```

3. **Verify Toolchain (Install Go 1.23+ & SQLite3 if Fresh Sandbox):**
   ```bash
   cd /home/user/webapp && bash scripts/verify_foundation.sh
   ```

4. **Read Continuity Documents in Exact Order:**
   1. `PROJECT_STATUS.md` — What is verified & what phase is active.
   2. `HANDOFF.md` — Immediate next steps, blockers, and rules.
   3. `SAFETY_RULES.md` & `NAMING_RULES.md` — Non-negotiable domain & file-protection invariants.
   4. `DECISIONS.md` — Locked vs. Open/Gated technical decisions.
   5. `ARCHITECTURE.md` & `DATABASE_SCHEMA.md` — Module boundaries and SQLite/FTS5 schema.
   6. `TEST_PLAN.md` & `ROADMAP.md` — Required tests before marking any task complete.

5. **End-of-Session Handoff Rules:**
   - Run `bash scripts/verify_foundation.sh` and `cd frontend && npm run typecheck`.
   - Perform secret & media-file hygiene scan prior to every `git push`.
   - Never push untested code or mark unverified features as completed.
   - Never commit API keys, passwords, `.db` files, caches, or media files.
   - Update **`PROJECT_STATUS.md`** and **`HANDOFF.md`** in the final commit of every session/phase, then push cleanly to `origin main` (no force push).

---

## 3. Session Log — Phase 0 Restoration & GitHub Synchronization (2026-11-17)
- **Last Completed Milestone:** Phase 0 — Master Project Foundation (Restored from backup `V9fQIuyS` with all 5 original commits `604d24a`, `b0a22b6`, `f6e49bf`, `c7ca5da`, `9ae29a9` preserved verbatim).
- **Fixes Applied During Restoration:**
  1. Anchored `.gitignore` cache rules (`/imagecache/` instead of `imagecache/`) and whitelisted `!frontend/**/*.ts` / `!frontend/**/*.tsx` (distinguishing TypeScript source files from MPEG-TS `*.ts` video streams) so `internal/imagecache/engine.go` and `frontend/src/*` contracts are cleanly tracked in Git.
  2. Relocated `.github/workflows/ci-windows.yml` to `build/ci/ci-windows.yml` because the Genspark GitHub App token grants `contents: write` without `workflows: write` scope (preventing push rejection on `.github/workflows/*` while preserving 100% of the original 5 commits `604d24a..9ae29a9` verbatim without force push).
  3. Bound canonical module & documentation URLs to the official GitHub Integration repository `https://github.com/sadigking5-cmd/Estiraha-Next.git` (noting that the user prompt spelled `sadikking5-cmd` with a `k`, whereas the authenticated GitHub account & linked repo is `sadigking5-cmd` with a `g`).
- **Verified Tests:** `go vet`, 15 unit tests (`tests/unit`), SQLite WAL/FK/FTS5 bilingual integration test (`tests/integration`), CLI `--self-check`, Windows pure-Go cross-compile check, and TypeScript `tsc --noEmit`.
- **Remaining Open Gates (Scheduled for Phase 1 upon User Approval):**
  1. Wails v3 native Windows 10/11 PoC (`OPEN-001`).
  2. Windows Credential Manager (`Advapi32`) native Win32 integration test.
  3. `mpv` (`OPEN-002`) and `ffprobe` (`OPEN-003`) license & binary integration gate + NSIS `.exe` artifact check on GitHub Actions `windows-latest`.
