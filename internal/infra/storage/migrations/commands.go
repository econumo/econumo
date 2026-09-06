package migrations

import "strings"

type commandStep struct{ version, name string }

var registry []commandStep

// RegisterCommand adds a command migration step: at Version, the boot runner
// invokes the CLI command `name` (must be `migration:<slug>`). Called from
// init() next to the SQL files so the ordered list stays in one place.
func RegisterCommand(version, name string) {
	if !strings.HasPrefix(name, "migration:") {
		panic("migrations: command step " + version + " must be a migration:* command, got " + name)
	}
	for _, c := range registry {
		if c.version == version {
			panic("migrations: duplicate command step version " + version)
		}
	}
	registry = append(registry, commandStep{version: version, name: name})
}

func commandSteps() []commandStep { return registry }

func init() {
	// 20260817000000.sql creates budgets_accounts; this step zeroes every
	// deleted account; 20260817000002.sql seeds membership and drops the
	// blacklist. See docs/superpowers/specs/2026-08-16-budget-account-membership-design.md §2.
	RegisterCommand("20260817000001", "migration:zero-deleted-accounts")
	// No SQL file: the value comes from the deprecated ECONUMO_ANALYTICS, which
	// SQL cannot read, and each row needs a UUIDv7 id SQLite cannot mint.
	RegisterCommand("20260903000000", "migration:seed-analytics-option")
}
