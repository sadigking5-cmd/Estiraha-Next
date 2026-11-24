# ESTIRAHA NEXT — CROSS-ACCOUNT CONTINUITY & HANDOFF GUIDE (`HANDOFF.md`)

## 1. Official Source of Truth
- **Repository:** `https://github.com/sadigking5-cmd/Estiraha-Next.git`
- **Phase 0 Baseline:** `main` (`72a76f6`)
- **Phase 1A Verified Windows PoC Baseline:** `phase1a/wails-v3-poc` (`23c620b` — GitHub Actions Run `#45105946395` green)
- **Active Phase B Branch:** `phase-b/media-library-system` (`v0.2.0-phase-b`)

---

## 2. Toolchain Requirements
- **Go:** `go1.27.4` (`CGO_ENABLED=1`, `-tags "libsqlite3 sqlite_fts5"` on Linux; `-tags "production sqlite_fts5"` on Windows)
- **Wails v3:** `v3.1.0` (`@wailsio/runtime@3.1.0`)
- **Frontend:** Node `22.x`, React `18.3.1`, TypeScript `5.6.3`, Vite `5.4.11`
- **Windows Cross-Compiler (on Linux):** `gcc-mingw-w64-x86-64` + `g++-mingw-w64-x86-64`
- **Windows Installer Builder:** NSIS `v3.11` (`makensis`)

### One-Command Verification & Build Scripts
```bash
# 1. Frontend TS/Vite build + Linux & Windows Go vet + Unit & Phase B E2E Integration tests + Self-Check + WebView2 exit-78 check
bash scripts/verify_foundation.sh

# 2. Cross-compile Windows x64 EstirahaNext.exe + package EstirahaNext-Setup-0.2.0-phase-b-x64.exe
bash scripts/build_windows_poc.sh
```

---

## 3. Session Log — Phase B Full Media Library System (2026-11-24)
1. **Increment 1 (`121014e`):** Added `migrations/0003_phase_b_library_management.sql` and implemented real Go services for `backup`, `library`, `scanner`, `identify`, `movies`, `series`, `naming` (with strict `.shorts` preservation), `duplicates`, `review`, `organizer`, `reports`, `player`, and Wails-bound `api.LibraryService` alongside `api.PoCService` in `cmd/estiraha/desktop_windows.go`.
2. **Increment 2 (`6b1f77a`):** Built typed Wails v3 `Call.ByName` client (`frontend/src/services/libraryClient.ts`) and full Hybrid Cinematic + Administrative UI (`frontend/src/App.tsx`, `frontend/src/styles/poc.css`) covering all 10 workspaces in Arabic RTL (right sidebar) and English LTR (left sidebar).
3. **Increment 3:** Added comprehensive E2E test suite (`tests/integration/phase_b_system_test.go`), updated NSIS packaging (`0.2.0-phase-b`) and `.github/workflows/ci-windows.yml`, verified full Linux & Windows cross-builds, and pushed to `origin/phase-b/media-library-system`.
