package migrations_test

import "testing"

func TestMigration20261116000000CreatesTheAffiliateIndex(t *testing.T) {
	db := runUpTo(t, "affiliate_index", "20261116000000")
	runAll(t, db)
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'affiliate_created_at_idx_users_options'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("index: count %d, err %v", n, err)
	}
}
