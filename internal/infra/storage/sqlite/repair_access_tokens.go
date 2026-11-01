package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
)

// oauthSessionColumns are the access_tokens columns 20260907000000 adds and
// 20260901000000's table rebuild drops.
var oauthSessionColumns = []string{"provider", "id_token"}

// RepairAccessTokensOAuthColumns re-adds access_tokens.provider / id_token when
// they are missing.
//
// They go missing on exactly one upgrade path. 20260907000000 (v1.5.0) adds
// them with ALTER TABLE; 20260901000000 (v1.6.0) carries an EARLIER version yet
// was released later, and rebuilds access_tokens to add a NOT NULL scope
// without carrying the two columns over. An instance that ran v1.5.x recorded
// 20260907000000, so upgrading to v1.6.x applies the still-pending, lower
// -numbered 20260901000000 afterwards and drops them; 20260907000000 is already
// recorded, so nothing re-adds them. Login then fails with "no such column:
// provider", because minting a session writes both.
//
// Only SQLite is affected: the PostgreSQL 20260901000000 adds scope with ALTER
// TABLE instead of rebuilding the table.
//
// Both columns are nullable with no default, so re-adding them restores the
// shape a fresh install has; the values were lost with the rebuild and cannot be
// recovered (a session loses its provider, so logout skips the RP-initiated
// redirect for it). Idempotent: a database with the columns is left untouched,
// which is why this is a command step and not SQL — SQLite supports neither
// ADD COLUMN IF NOT EXISTS nor a conditional ALTER.
func RepairAccessTokensOAuthColumns(ctx context.Context, db *sql.DB) (int, error) {
	var added int
	for _, column := range oauthSessionColumns {
		present, err := columnExists(ctx, db, "access_tokens", column)
		if err != nil {
			return added, err
		}
		if present {
			continue
		}
		if _, err := db.ExecContext(ctx, fmt.Sprintf(`ALTER TABLE access_tokens ADD COLUMN %s TEXT`, column)); err != nil {
			return added, fmt.Errorf("add access_tokens.%s: %w", column, err)
		}
		added++
		slog.WarnContext(ctx, "repair-access-tokens: re-added column dropped by an out-of-order migration",
			"table", "access_tokens", "column", column)
	}
	return added, nil
}

func columnExists(ctx context.Context, db *sql.DB, table, column string) (bool, error) {
	var n int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pragma_table_info(?) WHERE lower(name) = lower(?)`,
		table, column).Scan(&n); err != nil {
		return false, fmt.Errorf("inspect %s.%s: %w", table, column, err)
	}
	return n > 0, nil
}
