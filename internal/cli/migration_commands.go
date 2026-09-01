package cli

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/infra/storage/migrate"
)

// migrationCommands are data migrations the boot runner invokes as command
// steps (internal/infra/storage/migrations/commands.go). Every one must be
// idempotent — the runner records the version only after success, and an
// operator may re-run them by hand.
func migrationCommands() []command {
	return []command{
		{
			name:    "migration:zero-deleted-accounts",
			summary: "write a 'Balance adjustment (account deleted)' correction so every deleted account's balance is zero (idempotent)",
			run: func(ctx context.Context, c *container, args []string) error {
				n, err := c.account.ZeroDeletedAccounts(ctx)
				if err != nil {
					return err
				}
				fmt.Printf("zeroed %d account(s)\n", n)
				return nil
			},
		},
	}
}

// MigrationCommandRunner adapts the CLI registry to migrate.CommandRunner for
// the composition root (serve, data:import-sqlite). The container is built on
// first use over the caller's db and is never closed here.
func MigrationCommandRunner(cfg config.Config, db *sql.DB) migrate.CommandRunner {
	var c *container
	cmds := index(migrationCommands())
	return func(ctx context.Context, name string) error {
		if !strings.HasPrefix(name, "migration:") {
			return fmt.Errorf("cli: %q is not a migration command", name)
		}
		cmd, ok := cmds[name]
		if !ok {
			return fmt.Errorf("cli: unknown migration command %q", name)
		}
		if c == nil {
			c = newContainerFor(cfg, db)
		}
		logCommandStart(name, nil)
		return cmd.run(ctx, c, nil)
	}
}
