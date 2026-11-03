package migrations_test

import (
	"context"
	"testing"
)

// The seed step checks each user's key against this index, so it must exist first:
// without it that check scans the table once per user during boot.
func TestMigration20261103_TheKeyIsIndexedBeforeItIsSeeded(t *testing.T) {
	db := runUpTo(t, "email_key_index", "20261103000001")
	var n int
	if err := db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'users_email_key_uniq'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("users_email_key_uniq before the seed step: %d, want 1", n)
	}
}
