# ESTIRAHA NEXT — CROSS-ACCOUNT CONTINUITY & HANDOFF GUIDE (`HANDOFF.md`)

## 1. Purpose of This Document
Estiraha Next is engineered in **Genspark Super App — GenCode** across multiple sessions and potentially different Genspark accounts. **`https://github.com/a73971054-lab/Estiraha-Next.git`** is the single permanent source of truth. Any new Genspark account or developer joining the project MUST follow the protocol below before writing a single line of code.

---

## 2. Mandatory Onboarding Protocol for Any New Genspark Session / Account

1. **Authenticate GitHub First:**
   - Call `setup_github_environment` (ensure GitHub authorization is active in the `#github` tab).
   - Never create a new repository; always use `https://github.com/a73971054-lab/Estiraha-Next.git` (`main` branch unless a feature branch is explicitly requested).

2. **Clone / Sync into Workspace (`/home/user/webapp`):**
   ```bash
   rm -rf /home/user/webapp
   git clone https://github.com/a73971054-lab/Estiraha-Next.git /home/user/webapp
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
   - Run `go vet ./...`, `go test -v -race ./...`, and `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./cmd/estiraha`.
   - Never push untested code or mark unverified features as completed.
   - Never commit API keys, passwords, `.db` files, caches, or media files.
   - Update **`PROJECT_STATUS.md`** and **`HANDOFF.md`** in the final commit of every phase, then push cleanly to `origin main`.

---

## 3. Windows Build & Transfer Strategy (Linux Sandbox -> Native Windows `.exe`)
Detailed in `docs/architecture/WINDOWS_BUILD_PLAN.md`:
1. **Pure-Go Core & Cross-Compile Gate (Inside Linux GenCode Sandbox):**
   - Because `internal/db` uses `modernc.org/sqlite` (CGo-free), 100% of domain logic, SQLite migrations, FTS5 search, naming, review safety, API Hub, and Image Engine compile directly to `EstirahaNext.exe` via `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./cmd/estiraha`.
2. **Automated Windows Native Build & NSIS Packaging (GitHub Actions `windows-latest`):**
   - `.github/workflows/ci-windows.yml` compiles on native Windows Server with WebView2/Win32 headers, runs native Windows tests, invokes `makensis build/windows/nsis/installer.nsi`, and publishes `EstirahaNext-Setup-<version>-x64.exe` as a downloadable artifact.
3. **Local Windows Developer Build:**
   - Clone repo on Windows 10/11 -> `wails3 build` (once Phase 1 PoC gate passes) -> `makensis build\windows\nsis\installer.nsi`.

---

## 4. Current State at End of Phase 0 & Immediate Next Step
- **Completed:** Phase 0 Master Project Initialization (Structure, 33+2 SQLite/FTS5 schema, core safety/naming/DB/API-Hub/Image-Engine contracts, 10 governance docs, automated tests, Windows cross-build check).
- **Pending User Action:**
  1. Authorize GitHub in Genspark `#github` tab if not yet connected in the active session so local commits on `main` push directly to `https://github.com/a73971054-lab/Estiraha-Next.git`.
  2. Review Phase 0 deliverables and approve transition to **Phase 1 (Wails v3 + Windows Credential Manager + mpv/ffprobe PoC & License Gate)**.
