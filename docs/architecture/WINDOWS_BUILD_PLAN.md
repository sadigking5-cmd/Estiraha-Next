# Windows Build, Cross-Compilation & Packaging Plan (`docs/architecture/WINDOWS_BUILD_PLAN.md`)

## 1. Dual-Environment Engineering Model
Because daily development runs inside **Genspark GenCode (Linux x86_64)** while the target product is a **native Windows 10/11 x64 Desktop Application**, Estiraha Next separates compilation into two verified stages:

### Stage A — Continuous Verification Inside Linux Sandbox
- Pure-Go SQLite (`modernc.org/sqlite`) + pure-Go core packages allow running:
  - `go vet ./...`
  - `go test -v -race ./...`
  - `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o dist/windows-amd64/EstirahaNext.exe ./cmd/estiraha`
- OS-specific syscalls (Windows Credential Manager `Advapi32`, Volume Serial Win32 APIs, System Tray) are isolated behind Go build tags (`//go:build windows` and `//go:build !windows`).

### Stage B — Native Windows Compilation & NSIS `.exe` Packaging
- Triggered automatically on GitHub Actions (`windows-latest` runner in `.github/workflows/ci-windows.yml`) or a local Windows 10/11 workstation:
  1. Executes native Windows unit & integration tests (`go test -v ./...`).
  2. Compiles Wails v3 + WebView2 desktop binary (`EstirahaNext.exe`) with embedded frontend assets and embedded SQLite migrations.
  3. Runs `makensis build\windows\nsis\installer.nsi` to produce `EstirahaNext-Setup-<version>-x64.exe`.
  4. Installer writes binaries only to `$PROGRAMFILES64\Estiraha Next`; uninstaller never touches user media libraries or user SQLite backups.
