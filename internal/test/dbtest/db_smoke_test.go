package dbtest_test

import (
	"testing"

	"github.com/econumo/econumo/internal/test/dbtest"
)

func TestNewSQLite_MigratesAndQueries(t *testing.T) {
	db := dbtest.NewSQLite(t)
	if db.Engine != "sqlite" {
		t.Fatalf("engine = %q", db.Engine)
	}
	// The baseline migration seeds USD; assert the schema + seed are present.
	var n int
	if err := db.Raw.QueryRow(`SELECT COUNT(*) FROM currencies WHERE code = 'USD'`).Scan(&n); err != nil {
		t.Fatalf("query currencies: %v", err)
	}
	if n != 1 {
		t.Fatalf("USD currency count = %d, want 1 (migrations + seed ran)", n)
	}
	// The TxManager is wired over the same DB.
	if db.TX == nil || db.TX.DB() != db.Raw {
		t.Fatal("TxManager not wired to the raw DB")
	}
}
