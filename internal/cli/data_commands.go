package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/infra/storage/migrate"
	"github.com/econumo/econumo/internal/infra/storage/migrations"
	"github.com/econumo/econumo/internal/infra/storage/sqliteimport"
)

func dataCommands() []command {
	return []command{
		{
			name:    "data:import-sqlite",
			summary: "Copy all data from a SQLite DB into the configured PostgreSQL: data:import-sqlite [--force] <sqlite-path>",
			run:     runImportSQLite,
		},
	}
}

func parseImportArgs(args []string) (path string, force bool, err error) {
	const usage = "data:import-sqlite [--force] <sqlite-path>"
	for _, a := range args {
		switch {
		case a == "--force":
			force = true
		case strings.HasPrefix(a, "-"):
			return "", false, usageErr(usage)
		default:
			if path != "" {
				return "", false, usageErr(usage)
			}
			path = a
		}
	}
	if path == "" {
		return "", false, usageErr(usage)
	}
	return path, force, nil
}

func runImportSQLite(ctx context.Context, c *container, args []string) error {
	path, force, err := parseImportArgs(args)
	if err != nil {
		return err
	}

	if c.cfg.DatabaseDriver != "postgresql" {
		return fmt.Errorf("data:import-sqlite requires DATABASE_URL to point at PostgreSQL; current engine is %q", c.cfg.DatabaseDriver)
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if _, err := os.Stat(abs); err != nil {
		return fmt.Errorf("source sqlite file: %w", err)
	}

	be, ok := backend.Get("sqlite")
	if !ok {
		return errors.New("sqlite backend not registered")
	}
	src, err := be.Open(ctx, abs)
	if err != nil {
		return fmt.Errorf("open source sqlite: %w", err)
	}
	defer func() { _ = src.Close() }()

	// Bring the target to the current schema (schema + schema_migrations), so a
	// bare createdb'd Postgres works in one command; migrate.Run is idempotent.
	if err := migrate.Run(ctx, c.db, toMigratePg(migrations.Pgsql())); err != nil {
		return fmt.Errorf("migrate target: %w", err)
	}

	start := time.Now()
	report, err := sqliteimport.Import(ctx, src, c.db, force)
	if err != nil {
		if errors.Is(err, sqliteimport.ErrTargetNotEmpty) {
			return errors.New("the target PostgreSQL already contains data; re-run with --force to truncate and replace it")
		}
		return err
	}

	for _, t := range report.Tables {
		fmt.Printf("  %-32s %d\n", t.Name, t.Rows)
	}
	fmt.Printf("Imported %d row(s) across %d table(s) in %s.\n",
		report.Total, len(report.Tables), time.Since(start).Round(time.Millisecond))
	return nil
}

func toMigratePg(files []migrations.File) []migrate.Migration {
	out := make([]migrate.Migration, len(files))
	for i, f := range files {
		out[i] = migrate.Migration{Version: f.Version, SQL: f.SQL}
	}
	return out
}
