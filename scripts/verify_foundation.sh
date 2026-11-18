#!/usr/bin/env bash
set -euo pipefail

export PATH="/usr/local/go/bin:$HOME/go/bin:$PATH"
cd "$(dirname "$0")/.."

BUILD_TAGS="libsqlite3 sqlite_fts5"

echo "==> [1/5] Frontend TypeScript Check & Production Vite Build..."
(cd frontend && npm run build)

echo "==> [2/5] Go Vet across all packages (Linux & Windows targets)..."
go vet -tags "$BUILD_TAGS" ./...
GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc go vet -tags "production sqlite_fts5" ./...

echo "==> [3/5] Running Unit & SQLite/FTS5 Integration Tests..."
go test -v -tags "$BUILD_TAGS" ./...

echo "==> [4/5] Running Estiraha Next CLI --self-check..."
go run -tags "$BUILD_TAGS" ./cmd/estiraha --self-check --config ./configs/default.config.json

echo "==> [5/5] Testing Simulated Missing WebView2 Exit Code (Expected 78)..."
go build -tags "$BUILD_TAGS" -o /tmp/estiraha-wvcheck ./cmd/estiraha
set +e
/tmp/estiraha-wvcheck --simulate-missing-webview2 --headless
WV_EXIT=$?
set -e
rm -f /tmp/estiraha-wvcheck
if [ "$WV_EXIT" -ne 78 ]; then
  echo "Expected exit code 78 for missing WebView2, got $WV_EXIT"
  exit 1
fi

echo "==> All Phase 0 + Phase 1A Verification Checks PASSED."
