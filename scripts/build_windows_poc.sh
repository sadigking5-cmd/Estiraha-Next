#!/usr/bin/env bash
set -euo pipefail

export PATH="/usr/local/go/bin:$HOME/go/bin:$PATH"
cd "$(dirname "$0")/.."

echo "==> [1/4] Building Frontend Production Bundle (React + TypeScript + Vite)..."
(cd frontend && npm run build)

echo "==> [2/4] Cross-Compiling Windows x64 GUI Executable (CGO + Wails v3 + SQLite FTS5)..."
mkdir -p dist/windows-amd64
GOOS=windows \
GOARCH=amd64 \
CGO_ENABLED=1 \
CC=x86_64-w64-mingw32-gcc \
CXX=x86_64-w64-mingw32-g++ \
go build -trimpath -tags "production sqlite_fts5" -ldflags "-s -w -H windowsgui" -o dist/windows-amd64/EstirahaNext.exe ./cmd/estiraha

echo "==> [3/4] Packaging NSIS Windows Installer (.exe)..."
makensis build/windows/nsis/installer.nsi

echo "==> [4/4] Verifying Output Artifacts..."
ls -lh dist/windows-amd64/EstirahaNext.exe dist/EstirahaNext-Setup-0.2.0-phase-b-x64.exe
file dist/windows-amd64/EstirahaNext.exe dist/EstirahaNext-Setup-0.2.0-phase-b-x64.exe
