package migrations_test

import (
	"context"
	"testing"
)

// The normalizing step checks each address against this index, so it must exist
// first: without it that check scans the table once per user during boot.
func TestMigration20261103_AddressesAreIndexedBeforeTheyAreNormalized(t *testing.T) {
	db := runUpTo(t, "email_index", "20261103000001")
	var n int
	if err := db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'users_email_uniq'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("users_email_uniq before the normalizing step: %d, want 1", n)
	}
}
