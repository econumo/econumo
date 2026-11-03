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
	// No SQL file: the value comes from the deprecated ECONUMO_ANALYTICS, which
	// SQL cannot read, and each row needs a UUIDv7 id SQLite cannot mint.
	RegisterCommand("20260903000000", "migration:seed-analytics-option")
	// A command, not SQL: each value is parsed in Go with the exact layout the
	// driver wrote (time.Time.String()) rather than sliced with string
	// functions, columns are discovered from the live schema, and anything that
	// does not parse is left as stored.
	RegisterCommand("20260915000001", "migration:normalize-sqlite-datetimes")
	// 20261024000000.sql adds the transaction sort key columns; this step fills
	// them with fractional keys in the order the lists showed before. A command
	// because the keys come from the Go sortkey package, the one generator every
	// later write uses.
	RegisterCommand("20261024000001", "migration:seed-transaction-sort-keys")
	// Step 2 of 3 of the access_tokens repair (sqlite/20261031000010.sql): copies
	// the rows into access_tokens_new. A command because provider / id_token are
	// copied where they still exist and written as NULL where an out-of-order
	// migration dropped them, and SQL cannot name a column that may be missing.
	RegisterCommand("20261031000011", "migration:copy-access-tokens")
	// Zeroes every deleted account (it ran at 20260817000001, now a no-op). It
	// writes corrections with the current transaction write path, which needs
	// the final transactions schema, so it must stay numbered above every
	// migration that changes the tables that path writes. Zeroing never touches
	// accounts.updated_at, so the membership seed (20260817000002) reads the
	// same deletion times either way.
	RegisterCommand("20261101000000", "migration:zero-deleted-accounts")
	// A command because model.NormalizeEmail folds what SQLite's ASCII-only lower()
	// cannot, and only Go can tell a colliding address from a free one row by row.
	RegisterCommand("20261103000001", "migration:normalize-emails")
}
