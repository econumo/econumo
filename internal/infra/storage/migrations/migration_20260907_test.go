package migrations_test

import "testing"

func TestMigration20260907_OAuthTables(t *testing.T) {
	// runUpTo/runAll live in migration_20260803_test.go (same package).
	db := runUpTo(t, "oauth_tables", "20260907000000")
	runAll(t, db)
	const ts = "'2026-01-01 00:00:00'"
	if _, err := db.Exec(`INSERT INTO users (id, identifier, email, name, avatar, password, salt, created_at, updated_at) VALUES ('u1', 'u1', 'u1@e.test', 'U', '', 'x', '', ` + ts + `, ` + ts + `)`); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO users_identities (id, user_id, provider, subject, email, created_at, updated_at)
		 VALUES ('i1', (SELECT id FROM users LIMIT 1), 'google', 'sub-1', 'a@example.test', '2026-09-07 00:00:00', '2026-09-07 00:00:00')`,
		`INSERT INTO oauth_states (state_hash, provider, nonce, code_verifier, client, intent, link_user_id, created_at, expires_at)
		 VALUES ('h1', 'google', 'n', 'v', 'web', 'login', NULL, '2026-09-07 00:00:00', '2026-09-07 00:10:00')`,
		`SELECT provider, id_token FROM access_tokens LIMIT 1`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	// (provider, subject) is unique
	if _, err := db.Exec(`INSERT INTO users_identities (id, user_id, provider, subject, email, created_at, updated_at)
		VALUES ('i2', (SELECT id FROM users LIMIT 1), 'google', 'sub-1', 'b@example.test', '2026-09-07 00:00:00', '2026-09-07 00:00:00')`); err == nil {
		t.Fatal("duplicate (provider, subject) must be rejected")
	}
}
