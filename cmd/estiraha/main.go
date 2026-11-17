// Command estiraha is the official entry point of Estiraha Next (Windows Desktop Media Library System).
// In Phase 0 Foundation mode, it verifies configuration safety invariants, embedded SQLite migrations,
// Foreign Key enforcement, and FTS5 readiness without scanning or modifying any real user disks.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/sadigking5-cmd/Estiraha-Next/internal/config"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/db"
	"github.com/sadigking5-cmd/Estiraha-Next/internal/secrets"
)

func main() {
	selfCheck := flag.Bool("self-check", false, "Run non-destructive Phase 0 foundation verification in memory and exit")
	configPath := flag.String("config", "", "Optional path to JSON configuration file")
	flag.Parse()

	cfg := config.Default()
	if *configPath != "" {
		loaded, err := config.Load(*configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
			os.Exit(1)
		}
		cfg = loaded
	} else if err := cfg.ValidateSafetyInvariants(); err != nil {
		fmt.Fprintf(os.Stderr, "safety invariant error: %v\n", err)
		os.Exit(1)
	}

	if *selfCheck {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		engine, err := db.Open(ctx, ":memory:")
		if err != nil {
			fmt.Fprintf(os.Stderr, "db open failed: %v\n", err)
			os.Exit(1)
		}
		defer engine.Close()

		ver, err := engine.Migrate(ctx, "")
		if err != nil {
			fmt.Fprintf(os.Stderr, "db migration failed: %v\n", err)
			os.Exit(1)
		}

		health, err := engine.HealthCheck(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "db health check failed: %v\n", err)
			os.Exit(1)
		}

		vault := secrets.NewDefaultVault()
		summary := map[string]any{
			"app":               cfg.App.Name,
			"version":           cfg.App.Version,
			"phase":             "Phase 0 — Official Foundation",
			"schema_version":    ver,
			"registered_tables": health.RegisteredTables,
			"foreign_keys_on":   health.ForeignKeys,
			"fts5_enabled":      health.FTS5Enabled,
			"secrets_backend":   vault.BackendName(),
			"safety_lock":       "ACTIVE (Zero unapproved file operations)",
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(summary)
		return
	}

	fmt.Printf("%s (%s) — Foundation Core Ready. Run with --self-check to verify engine health.\n", cfg.App.Name, cfg.App.Version)
}
