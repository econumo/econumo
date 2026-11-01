package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// CopyAccessTokens is step 2 of the access_tokens repair (migrations
// 20261031000010-20261031000012): it fills access_tokens_new from access_tokens.
//
// provider / id_token are copied where the old table still has them (fresh
// installs, anything upgraded from v1.6.x) and written as NULL where the
// out-of-order 20260901000000 rebuild dropped them (a v1.5.x upgrade); those
// values are gone and cannot be recovered, so such a session keeps no provider
// and logout skips the RP-initiated redirect for it. Every other column,
// scope included, is copied verbatim: widening an ingest token to full would
// be a privilege escalation.
//
// Idempotent: the copy is replaced in one transaction, so a run interrupted
// before step 3 is simply redone. pending is false when there is no
// access_tokens_new to fill (step 1 not applied yet, or step 3 already swapped
// the tables), so nothing was done.
func CopyAccessTokens(ctx context.Context, db *sql.DB) (copied int64, pending bool, err error) {
	pending, err = tableExists(ctx, db, "access_tokens_new")
	if err != nil || !pending {
		return 0, pending, err
	}
	columns := []string{"id", "user_id", "kind", "token_hash", "scope", "name", "user_agent",
		"created_at", "last_used_at", "expires_at", "revoked_at", "provider", "id_token", "grant_id"}
	selects := make([]string, len(columns))
	for i, column := range columns {
		selects[i] = column
		if column != "provider" && column != "id_token" {
			continue
		}
		present, err := columnExists(ctx, db, "access_tokens", column)
		if err != nil {
			return 0, true, err
		}
		if !present {
			selects[i] = "NULL"
		}
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, true, fmt.Errorf("copy access_tokens: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM access_tokens_new`); err != nil {
		return 0, true, fmt.Errorf("copy access_tokens: clear: %w", err)
	}
	res, err := tx.ExecContext(ctx, fmt.Sprintf(`INSERT INTO access_tokens_new (%s) SELECT %s FROM access_tokens`,
		strings.Join(columns, ", "), strings.Join(selects, ", ")))
	if err != nil {
		return 0, true, fmt.Errorf("copy access_tokens: %w", err)
	}
	copied, err = res.RowsAffected()
	if err != nil {
		return 0, true, fmt.Errorf("copy access_tokens: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, true, fmt.Errorf("copy access_tokens: commit: %w", err)
	}
	return copied, true, nil
}

func tableExists(ctx context.Context, db *sql.DB, table string) (bool, error) {
	var n int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&n); err != nil {
		return false, fmt.Errorf("inspect table %s: %w", table, err)
	}
	return n > 0, nil
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
