# ESTIRAHA NEXT — CROSS-ACCOUNT CONTINUITY & HANDOFF GUIDE (`HANDOFF.md`)

## 1. Official Source of Truth
- **Repository:** `https://github.com/sadigking5-cmd/Estiraha-Next.git`
- **Production Baseline Branch:** `main` (Phase 0 closed at `72a76f6`)
- **Active PoC Branch:** `phase1a/wails-v3-poc`

---

## 2. Toolchain Requirements (Updated in Phase 1A)
- **Go:** `go1.27.4` (`go >= 1.26.0` required by `github.com/wailsapp/wails/v3@v3.1.0`)
- **Wails v3:** `v3.1.0` (`@wailsio/runtime@3.1.0`)
- **Node.js / Frontend:** Node `22.x`, React `18.3.1`, TypeScript `5.6.3`, Vite `5.4.11`
- **Windows Cross-Compiler (on Linux):** `gcc-mingw-w64-x86-64` + `g++-mingw-w64-x86-64` (`CGO_ENABLED=1`)
- **Windows Installer Builder:** NSIS `v3.11` (`makensis`)

### One-Command Verification & Build Scripts
```bash
# Run Frontend build + Linux/Windows Go vet + Unit & SQLite/FTS5 tests + Self-Check + WebView2 missing exit check
bash scripts/verify_foundation.sh

# Build EstirahaNext.exe (Windows x64 GUI with CGO) + EstirahaNext-Setup-0.1.0-phase1a-x64.exe (NSIS)
bash scripts/build_windows_poc.sh
```

---

## 3. Session Log — Phase 1A Windows Desktop PoC (2026-11-17)
- **Completed Work (`phase1a/wails-v3-poc`):**
  1. Upgraded Go toolchain compatibility to `go 1.26.0` / `go1.27.4` and integrated `github.com/wailsapp/wails/v3 v3.1.0` + `github.com/danieljoos/wincred v1.2.3`.
  2. Implemented `internal/secrets/store_windows.go` (real Win32 `Advapi32` DPAPI Generic Credentials) + non-leaking `RunDiagnosticProbe`.
  3. Implemented `internal/app/webview2_windows.go` (Registry detection + bilingual Win32 `MessageBoxW` prompt + deterministic exit code `78` when missing).
  4. Implemented `internal/tray/icon.go` (programmatic 64x64 RGBA PNG icon + thread-safe hide/restore/destroy state tracker).
  5. Implemented `internal/api/poc_service.go` + `cmd/estiraha/desktop_windows.go` + generated official Wails v3 TS bindings (`frontend/src/bindings`) + bilingual RTL/LTR React/TypeScript PoC workspace (`frontend/src/App.tsx`).
  6. Placed GitHub Actions workflow at `.github/workflows/ci-windows.yml` (kept strictly in its official path per project rules).

- **Important GitHub Workflow Permission Note:**
  - If pushing via a GitHub App token that lacks the `workflows` permission (`refusing to allow a GitHub App to create or update workflow .github/workflows/ci-windows.yml`), either:
    1. Authorize GitHub **OAuth** (with `workflow` scope) in the Genspark `#github` tab and push `phase1a/wails-v3-poc`, **OR**
    2. Add `.github/workflows/ci-windows.yml` once via GitHub's web UI ("Add file -> Create new file") on branch `phase1a/wails-v3-poc` using the exact file committed locally.

- **Next Step (Awaiting User Approval):**
  1. Run/inspect `EstirahaNext-Setup-0.1.0-phase1a-x64.exe` on Windows 10/11 or trigger `.github/workflows/ci-windows.yml` on GitHub Actions `windows-latest` to collect the live Windows screenshot (`phase1a-windows-window.png`) and native smoke report (`phase1a-smoke-report.json`).
  2. Upon user sign-off on Phase 1A, merge `phase1a/wails-v3-poc` into `main` and begin **Phase 1B** (`mpv` / `VLC` / `ffprobe` PoC & license audit).
