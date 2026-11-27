package migrations_test

import (
	"context"
	"database/sql"
	"testing"
)

func TestMigration20261127AddsUserDeletionSchema(t *testing.T) {
	db := runUpTo(t, "user_deletion", "20261127000000")
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}
	checkUserDeletionSchema(t, db, func(q string) string { return q }, func() { runAll(t, db) })
}

func checkUserDeletionSchema(t *testing.T, db *sql.DB, rebind func(string) string, migrate func()) {
	t.Helper()
	ctx := context.Background()
	exec := func(q string, args ...any) error {
		_, err := db.ExecContext(ctx, rebind(q), args...)
		return err
	}
	mustExec := func(q string, args ...any) {
		t.Helper()
		if err := exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	const ts = "'2026-01-01 00:00:00'"
	insertUser := func(id, email string) {
		t.Helper()
		mustExec(`INSERT INTO users (id, identifier, email, name, avatar, password, salt, algorithm, created_at, updated_at, is_active)
			VALUES (?, ?, ?, 'U', 'face:sky', 'x', 's', 'argon2id', `+ts+`, `+ts+`, TRUE)`, id, id, email)
	}
	const before = "00000000-0000-4000-8000-000000000001"
	const after = "00000000-0000-4000-8000-000000000002"
	insertUser(before, "before@x.test")

	migrate()

	var markedNull, activeNull bool
	var reason string
	if err := db.QueryRowContext(ctx, rebind(`SELECT deletion_marked_at IS NULL, deletion_reason, last_active_at IS NULL FROM users WHERE id = ?`), before).
		Scan(&markedNull, &reason, &activeNull); err != nil {
		t.Fatal(err)
	}
	if !markedNull || reason != "" || activeNull {
		t.Fatalf("pre-existing user: marked null %v, reason %q, last_active null %v; want true, \"\", false", markedNull, reason, activeNull)
	}

	insertUser(after, "after@x.test")
	if err := db.QueryRowContext(ctx, rebind(`SELECT deletion_marked_at IS NULL, deletion_reason, last_active_at IS NULL FROM users WHERE id = ?`), after).
		Scan(&markedNull, &reason, &activeNull); err != nil {
		t.Fatal(err)
	}
	if !markedNull || reason != "" || !activeNull {
		t.Fatalf("new user: marked null %v, reason %q, last_active null %v; want true, \"\", true", markedNull, reason, activeNull)
	}
	if err := exec(`UPDATE users SET deletion_reason = NULL WHERE id = ?`, after); err == nil {
		t.Fatal("deletion_reason accepted NULL")
	}

	mustExec(`INSERT INTO users_audit (id, user_id, event, email_hash, ref_id, created_at) VALUES (?, ?, 'deleted', 'h', NULL, `+ts+`)`,
		"00000000-0000-4000-8000-0000000000a1", before)
	mustExec(`INSERT INTO users_audit (id, user_id, event, email_hash, ref_id, created_at) VALUES (?, ?, 'deleted', 'h', NULL, `+ts+`)`,
		"00000000-0000-4000-8000-0000000000a2", before)
	mustExec(`INSERT INTO users_audit (id, user_id, event, ref_id, created_at) VALUES (?, ?, 'lifetime_regranted', ?, `+ts+`)`,
		"00000000-0000-4000-8000-0000000000a3", after, "00000000-0000-4000-8000-0000000000a1")
	if err := exec(`INSERT INTO users_audit (id, user_id, event, ref_id, created_at) VALUES (?, ?, 'lifetime_regranted', ?, `+ts+`)`,
		"00000000-0000-4000-8000-0000000000a4", after, "00000000-0000-4000-8000-0000000000a1"); err == nil {
		t.Fatal("a second audit row consumed the same ref_id")
	}
	var details string
	if err := db.QueryRowContext(ctx, rebind(`SELECT details FROM users_audit WHERE id = ?`), "00000000-0000-4000-8000-0000000000a1").Scan(&details); err != nil || details != "{}" {
		t.Fatalf("details default = %q, err %v", details, err)
	}

	mustExec(`INSERT INTO users_deletion_requests (id, user_id, email, code, created_at, expired_at) VALUES (?, ?, 'after@x.test', 'c', `+ts+`, `+ts+`)`,
		"00000000-0000-4000-8000-0000000000b1", after)
	if err := exec(`INSERT INTO users_deletion_requests (id, user_id, email, code, created_at, expired_at) VALUES (?, ?, 'after@x.test', 'c2', `+ts+`, `+ts+`)`,
		"00000000-0000-4000-8000-0000000000b2", after); err == nil {
		t.Fatal("a second deletion request for one user was accepted")
	}
	mustExec(`DELETE FROM users WHERE id = ?`, after)
	var requests, audits int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users_deletion_requests`).Scan(&requests); err != nil || requests != 0 {
		t.Fatalf("deletion requests after the user went: %d, err %v", requests, err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users_audit`).Scan(&audits); err != nil || audits != 3 {
		t.Fatalf("audit rows after the user went: %d, err %v (the audit outlives the user)", audits, err)
	}
}
