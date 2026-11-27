package wipeguard

import (
	"testing"

	"github.com/econumo/econumo/internal/test/dbtest"
)

func TestSchemaGuard_SQLite(t *testing.T) {
	Check(t, dbtest.NewSQLite(t))
}

func TestUserColumns_SQLite(t *testing.T) {
	assertUserColumns(t, dbtest.NewSQLite(t))
}

func assertUserColumns(t *testing.T, db *dbtest.DB) {
	t.Helper()
	cols := UserColumns(t, db)
	seen := map[Column]bool{}
	for _, c := range cols {
		if seen[c] {
			t.Errorf("UserColumns lists %s.%s twice", c.Table, c.Name)
		}
		seen[c] = true
	}
	for _, want := range []Column{
		{"users_audit", "user_id"},
		{"idempotency_keys", "user_id"},
		{"oauth_states", "link_user_id"},
		{"users_deletion_requests", "user_id"},
		{"access_tokens", "user_id"},
	} {
		if !seen[want] {
			t.Errorf("UserColumns misses %s.%s", want.Table, want.Name)
		}
	}
}
