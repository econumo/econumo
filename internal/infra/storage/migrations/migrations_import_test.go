package migrations_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/test/dbtest"
)

// The nine import tables plus access_tokens.scope come from 20260901000000.
// Probing each with a no-row SELECT is engine-neutral and fails loudly on a
// missing table or column.
func TestMigration20260901_ImportTablesAndTokenScope(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	for _, q := range []string{
		"SELECT id, user_id, provider, name, credential_ciphertext, status, last_synced_at, created_at, updated_at FROM import_sources WHERE 1 = 0",
		"SELECT user_id, key_ciphertext, created_at FROM import_credential_keys WHERE 1 = 0",
		"SELECT id, user_id, source_id, provider, params, status, imported_count, matched_count, skipped_count, failed_count, started_at, finished_at FROM import_runs WHERE 1 = 0",
		"SELECT id, source_id, run_id, payload, payload_hash, status, parse_error, received_at FROM import_events WHERE 1 = 0",
		"SELECT id, source_id, external_account_id, external_name, external_currency, account_id, mode, created_at, updated_at FROM import_account_links WHERE 1 = 0",
		"SELECT id, source_id, run_id, event_id, external_account_id, external_transaction_id, transaction_id, status, external_payee, external_description, external_amount, external_currency, external_posted_at, applied_category_id, applied_payee_id, applied_tag_id, applied_rule_id, imported_at FROM import_transaction_links WHERE 1 = 0",
		"SELECT id, user_id, source_id, position, match_payee, match_description, match_amount_min, match_amount_max, action, category_id, payee_id, tag_id, created_at, updated_at FROM import_rules WHERE 1 = 0",
		"SELECT rule_id, label_id FROM import_rule_labels WHERE 1 = 0",
		"SELECT link_id, label_id FROM import_link_applied_labels WHERE 1 = 0",
		"SELECT scope FROM access_tokens WHERE 1 = 0",
	} {
		if _, err := db.Raw.ExecContext(ctx, db.Rebind(q)); err != nil {
			t.Errorf("%s: %v", q, err)
		}
	}
	// scope has no default: the column must be supplied on every insert.
	_, err := db.Raw.ExecContext(ctx, db.Rebind(
		"INSERT INTO access_tokens (id, user_id, kind, token_hash, created_at, last_used_at) VALUES (?, ?, ?, ?, ?, ?)"),
		"0f000000-0000-0000-0000-000000000001", "0f000000-0000-0000-0000-0000000000aa", "session", "h", "2026-01-01 00:00:00", "2026-01-01 00:00:00")
	if err == nil {
		t.Error("inserting an access token without scope must fail (NOT NULL, no default)")
	}
}
