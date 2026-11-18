package migrations_test

import (
	"strings"
	"testing"
)

func TestMigration20261116000000CreatesTheAffiliateIndex(t *testing.T) {
	db := runUpTo(t, "affiliate_index", "20261116000000")
	runAll(t, db)
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'affiliate_created_at_idx_users_options'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("index: count %d, err %v", n, err)
	}
	rows, err := db.Query(`SELECT name FROM pragma_index_info('affiliate_created_at_idx_users_options') ORDER BY seqno`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			t.Fatal(err)
		}
		cols = append(cols, c)
	}
	if got := strings.Join(cols, ","); got != "created_at,user_id" {
		t.Fatalf("index columns = %s", got)
	}
}
