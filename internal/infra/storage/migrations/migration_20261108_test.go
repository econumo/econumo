package migrations_test

import (
	"context"
	"database/sql"
	"testing"
)

const (
	owA       = "02a00000-0000-7000-8000-00000000000a"
	owB       = "02a00000-0000-7000-8000-00000000000b"
	owC       = "02a00000-0000-7000-8000-00000000000c"
	owCliNone = "02b00000-0000-7000-8000-000000000001"
	owCliOne  = "02b00000-0000-7000-8000-000000000002"
	owCliMany = "02b00000-0000-7000-8000-000000000003"
	owGA      = "02c00000-0000-7000-8000-0000000000a1"
	owGC      = "02c00000-0000-7000-8000-0000000000c1"
	owGB      = "02c00000-0000-7000-8000-0000000000b1"
	owGB2     = "02c00000-0000-7000-8000-0000000000b2"
)

// 20261108000000 gives each existing OAuth client the user of its earliest
// grant as owner and leaves it a user app (not global).
func TestMigration20261108_BackfillsClientOwners(t *testing.T) {
	db := runUpTo(t, "oauth_owner", "20261108000000")
	checkOAuthOwnerBackfill(t, db, func(q string) string { return q }, func() {
		runAll(t, db)
		if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
			t.Fatal(err)
		}
	})
}

// checkOAuthOwnerBackfill is shared with the PostgreSQL twin (enginecompare tag).
func checkOAuthOwnerBackfill(t *testing.T, db *sql.DB, rebind func(string) string, finish func()) {
	t.Helper()
	ctx := context.Background()
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, rebind(q), args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	const ts = "'2026-01-01 00:00:00'"
	for i, u := range []string{owA, owB, owC} {
		mustExec(`INSERT INTO users (id, identifier, email, name, avatar, password, salt, algorithm, created_at, updated_at, is_active)
			VALUES (?, ?, ?, 'U', 'face:sky', 'x', 's', 'argon2id', `+ts+`, `+ts+`, TRUE)`, u, u, "u"+string(rune('a'+i))+"@x.test")
	}
	for _, c := range []string{owCliNone, owCliOne, owCliMany} {
		mustExec(`INSERT INTO oauth_clients (id, name, redirect_uris, created_at) VALUES (?, 'App', '[]', `+ts+`)`, c)
	}
	grant := func(id, user, client, created string, revoked bool) {
		rev := "NULL"
		if revoked {
			rev = ts
		}
		mustExec(`INSERT INTO oauth_grants (id, user_id, client_id, refresh_token_hash, created_at, last_used_at, expires_at, revoked_at)
			VALUES (?, ?, ?, ?, '`+created+`', '`+created+`', '2027-01-01 00:00:00', `+rev+`)`, id, user, client, "h-"+id)
	}
	grant(owGB, owB, owCliOne, "2026-01-01 00:00:00", false)
	grant(owGA, owA, owCliMany, "2026-01-01 00:00:00", true)
	grant(owGC, owC, owCliMany, "2026-01-01 00:00:00", false)
	grant(owGB2, owB, owCliMany, "2026-01-02 00:00:00", false)

	finish()

	owner := func(id string) (o sql.NullString) {
		t.Helper()
		if err := db.QueryRowContext(ctx, rebind(`SELECT owner_user_id FROM oauth_clients WHERE id = ?`), id).Scan(&o); err != nil {
			t.Fatal(err)
		}
		return
	}
	isGlobal := func(id string) (g bool) {
		t.Helper()
		if err := db.QueryRowContext(ctx, rebind(`SELECT is_global FROM oauth_clients WHERE id = ?`), id).Scan(&g); err != nil {
			t.Fatal(err)
		}
		return
	}
	if o := owner(owCliNone); o.Valid {
		t.Errorf("client without grants has owner %v", o)
	}
	if o := owner(owCliOne); o.String != owB {
		t.Errorf("single-grant client owner = %v, want B", o)
	}
	if o := owner(owCliMany); o.String != owA {
		t.Errorf("earliest grant (revoked counts, tie by grant id) owner = %v, want A", o)
	}
	for _, c := range []string{owCliNone, owCliOne, owCliMany} {
		if isGlobal(c) {
			t.Errorf("client %s is global", c)
		}
	}
	mustExec(`DELETE FROM users WHERE id = ?`, owB)
	var n int
	if err := db.QueryRowContext(ctx, rebind(`SELECT COUNT(*) FROM oauth_clients WHERE id = ?`), owCliOne).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("deleting the owner leaves the client row (%d)", n)
	}
}
