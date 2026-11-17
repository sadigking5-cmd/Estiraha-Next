#!/usr/bin/env bash
set -euo pipefail

export PATH="/usr/local/go/bin:$HOME/go/bin:$PATH"
cd "$(dirname "$0")/.."

BUILD_TAGS="libsqlite3 sqlite_fts5"

echo "==> [1/4] Go Vet across all packages..."
go vet -tags "$BUILD_TAGS" ./...

echo "==> [2/4] Running Unit & SQLite/FTS5 Integration Tests..."
go test -v -tags "$BUILD_TAGS" ./...

echo "==> [3/4] Running Estiraha Next CLI --self-check..."
go run -tags "$BUILD_TAGS" ./cmd/estiraha --self-check --config ./configs/default.config.json

echo "==> [4/4] Verifying Windows Cross-Compilation of Pure-Go Packages..."
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./internal/domain ./internal/config ./internal/naming ./internal/review ./internal/fsops ./internal/providers ./internal/imagecache ./internal/secrets

echo "==> All Phase 0 Foundation Checks PASSED."
