package migrations

import "embed"

// FS embeds all ordered SQLite migration files directly into the Estiraha Next binary
// so Windows single-executable installations never depend on loose SQL files on disk.
//
//go:embed *.sql
var FS embed.FS
