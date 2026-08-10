package migrate_test

import (
	"testing"

	"github.com/econumo/econumo/internal/test/dbtest"
)

func TestLabelsSchemaExists(t *testing.T) {
	db := dbtest.New(t)
	var n int
	if err := db.Raw.QueryRow(db.Rebind(`SELECT COUNT(*) FROM labels`)).Scan(&n); err != nil {
		t.Fatalf("labels table missing: %v", err)
	}
	if err := db.Raw.QueryRow(db.Rebind(`SELECT COUNT(*) FROM transactions_labels`)).Scan(&n); err != nil {
		t.Fatalf("transactions_labels table missing: %v", err)
	}
	if err := db.Raw.QueryRow(db.Rebind(`SELECT COUNT(icon) FROM tags`)).Scan(&n); err != nil {
		t.Fatalf("tags.icon column missing: %v", err)
	}
}
