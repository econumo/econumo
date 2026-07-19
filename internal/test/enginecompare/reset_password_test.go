//go:build enginecompare

package enginecompare

// Full password-recovery flow exercised against BOTH engines.
//
// Unlike the byte-parity catalogue in apiparity_test.go, the reset code is
// random, so this can't compare wire bytes between engines. Instead it asserts
// the flow WORKS end-to-end on each engine independently: remind-password
// persists a code, that code resets the password, the new password logs in, and
// the code is consumed. It reads the issued code straight from
// users_password_requests (the test mailer is a no-op).
//
// It exists because the reset flow's only prior coverage ran on SQLite, while
// production runs the PostgreSQL adapter — exactly the path a "nothing happens,
// no code in the database" report points at. Running this against a real
// Postgres (DATABASE_TEST_PGSQL_URL) reproduces the production code path.
//
// Synthetic data only: dbtest provisions a throwaway, randomly-named database
// and runs the project's own migrations — it never touches a real dataset.

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/test/apiparity"
	"github.com/econumo/econumo/internal/test/dbtest"
)

func TestResetPasswordFlow_PerEngine(t *testing.T) {
	run := func(t *testing.T, db *dbtest.DB) {
		h := apiparity.NewHarness(t, db)
		const newPassword = "reset-pw-9981"

		// 1. remind-password issues a code for the seeded owner.
		if st, body := h.Call(t, http.MethodPost, "/api/v1/user/remind-password", "", map[string]string{
			"username": apiparity.OwnerEmail,
		}); st != http.StatusOK {
			t.Fatalf("remind status = %d; body: %s", st, body)
		}

		// 2. A code must actually be persisted (this is the assertion that catches
		//    "no code being added to the database"); the emitted plaintext is
		//    recovered from the email, since the code is hashed at rest.
		if n := countResetCodes(t, db, apiparity.OwnerID); n != 1 {
			t.Fatalf("issued reset codes = %d, want 1 — was it inserted?", n)
		}
		code := h.LastResetCode(t)
		if len(code) != 12 {
			t.Fatalf("issued code = %q (len %d), want 12 chars", code, len(code))
		}

		// 3. A wrong code is rejected (400).
		if st, _ := h.Call(t, http.MethodPost, "/api/v1/user/reset-password", "", map[string]string{
			"username": apiparity.OwnerEmail, "code": "ffffffffffff", "password": newPassword,
		}); st != http.StatusBadRequest {
			t.Fatalf("reset with wrong code = %d, want 400", st)
		}

		// 4. The correct code resets the password.
		if st, body := h.Call(t, http.MethodPost, "/api/v1/user/reset-password", "", map[string]string{
			"username": apiparity.OwnerEmail, "code": code, "password": newPassword,
		}); st != http.StatusOK {
			t.Fatalf("reset status = %d; body: %s", st, body)
		}

		// 5. The new password logs in (and the code is single-use: consumed).
		if st, body := h.Call(t, http.MethodPost, "/api/v1/user/login-user", "", map[string]string{
			"username": apiparity.OwnerEmail, "password": newPassword,
		}); st != http.StatusOK {
			t.Fatalf("login with new password = %d, want 200; body: %s", st, body)
		}
		if n := countResetCodes(t, db, apiparity.OwnerID); n != 0 {
			t.Errorf("reset code not consumed: %d row(s) remain", n)
		}

		// 6. The old password no longer works.
		if st, _ := h.Call(t, http.MethodPost, "/api/v1/user/login-user", "", map[string]string{
			"username": apiparity.OwnerEmail, "password": apiparity.SeedPassword,
		}); st == http.StatusOK {
			t.Fatal("old password should no longer work after reset")
		}

		// 7. remind-password for an unknown user is a silent success (anti-
		//    enumeration) that writes no row.
		if st, _ := h.Call(t, http.MethodPost, "/api/v1/user/remind-password", "", map[string]string{
			"username": "nobody@example.test",
		}); st != http.StatusOK {
			t.Fatalf("remind unknown user = %d, want 200", st)
		}
	}

	t.Run("sqlite", func(t *testing.T) { run(t, dbtest.NewSQLite(t)) })
	t.Run("postgresql", func(t *testing.T) { run(t, dbtest.NewPostgres(t)) }) // SKIPs if env unset
}

// countResetCodes returns how many reset codes remain for userID.
func countResetCodes(t *testing.T, db *dbtest.DB, userID string) int {
	t.Helper()
	var n int
	q := "SELECT COUNT(*) FROM users_password_requests WHERE user_id = " + placeholder(db, 1)
	if err := db.Raw.QueryRow(q, userID).Scan(&n); err != nil {
		t.Fatalf("count reset codes (%s): %v", db.Engine, err)
	}
	return n
}

// placeholder renders the engine's positional-parameter marker.
func placeholder(db *dbtest.DB, n int) string {
	if db.Engine == "postgresql" {
		return fmt.Sprintf("$%d", n)
	}
	return "?"
}
