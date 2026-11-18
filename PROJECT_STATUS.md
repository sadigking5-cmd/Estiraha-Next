# ESTIRAHA NEXT — PROJECT STATUS (`PROJECT_STATUS.md`)

- **Project Name:** Estiraha Next
- **Official Repository:** `https://github.com/sadigking5-cmd/Estiraha-Next.git`
- **Active Branch:** `phase1a/wails-v3-poc` (branched from `main` @ `72a76f6`)
- **Current Phase:** **Phase 1A — Windows Desktop Proof of Concept (Code, Cross-Build & NSIS Installer Completed; Awaiting Native Windows Runner/Host Execution & User Sign-Off)**
- **Next Scheduled Phase:** Phase 1B — Multimedia Probing & Player PoC (`ffprobe` / `mpv` / `VLC`) *(Blocked awaiting explicit user approval)*
- **Last Updated:** 2026-11-17

---

## 1. Phase 1A Deliverables & Verification Summary
- [x] Cloned latest official repo and verified base commit `72a76f6`; created isolated branch `phase1a/wails-v3-poc`.
- [x] Verified Wails v3 (`v3.1.0`) Go toolchain requirement (`go >= 1.26.0`) and upgraded toolchain to **`go1.27.4`**.
- [x] Configured Windows x64 CGO cross-compilation environment (`x86_64-w64-mingw32-gcc 14-win32`) and NSIS (`makensis v3.11`).
- [x] Wired **Wails v3.1.0** + **React 18.3.1** + **TypeScript 5.6.3** + **Vite 5.4.11** + **`@wailsio/runtime@3.1.0`** with zero-warning auto-generated TypeScript bindings (`wails3 generate bindings -ts`).
- [x] Implemented native Windows window (`1240x820`), bilingual Arabic `RTL` (right sidebar) & English `LTR` (left sidebar) switching, bidirectional RPC + Event Bus (`React <-> Go`), System Tray (`Hide`/`Restore`/`Clean Quit` with programmatic 64x64 PNG icon), native **Windows Credential Manager** (`github.com/danieljoos/wincred v1.2.3` / `Advapi32` DPAPI) roundtrip probe with zero secret leakage, and Microsoft Edge **WebView2** registry detection + bilingual `MessageBoxW` / exit-code `78` fallback.
- [x] Compiled real Windows x64 GUI executable (`EstirahaNext.exe`, `13 MB`, `PE32+ Windows GUI x86-64`) and packaged real **NSIS installer** (`EstirahaNext-Setup-0.1.0-phase1a-x64.exe`, `5.2 MB`).
- [x] Authored `.github/workflows/ci-windows.yml` in its official path to run native `windows-latest` execution, silent NSIS install, live window screenshot capture, System Tray & Credential Manager smoke verification, and zero-orphan-process checks.

## 2. Strict Phase 1A Boundary Attestation
- **Zero new SQLite migrations added** (remains at Phase 0 Schema v2).
- **Zero movie library features or Phase 1B items started.**
- **Zero real user disks scanned or files modified.**
- **Honest Platform Reporting:** Cross-compilation, unit/SQLite/FTS5 tests, PE64 binary verification, and NSIS `.exe` packaging executed and passed on Linux sandbox; native Windows GUI rendering, live Tray HWND click, and Win32 `Advapi32` DPAPI calls require execution on a real Windows host or the prepared GitHub Actions `windows-latest` runner.
