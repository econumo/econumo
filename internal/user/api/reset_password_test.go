package api_test

import (
	"net/http"
	"testing"
)

// TestRemindAndResetPassword exercises the full reset flow end-to-end over the
// real HTTP handlers + sqlite: remind issues a code (read here from the DB, since
// the test mailer is a no-op), reset consumes it and changes the password, the
// code is single-use, wrong codes are rejected, and remind for an unknown user
// still returns 200 (anti-enumeration).
func TestRemindAndResetPassword(t *testing.T) {
	h := newHarness(t)

	// 1. Request a reset code for the seeded user.
	if st, env := h.do(t, http.MethodPost, "/api/v1/user/remind-password", "", map[string]string{
		"username": seedEmail,
	}); st != http.StatusOK {
		t.Fatalf("remind status = %d; body: %s", st, env.raw)
	}

	// 2. Recover the generated code from the email (it is hashed at rest, so the
	//    DB no longer holds the plaintext).
	code := h.mail.lastResetCode(t)
	if len(code) != 6 {
		t.Fatalf("code = %q, want 6 chars", code)
	}
	// The stored code is the sha256 hex of the emailed code, never the plaintext.
	var stored string
	if err := h.db.QueryRow(`SELECT code FROM users_password_requests WHERE user_id = ?`, seedUserID).Scan(&stored); err != nil {
		t.Fatalf("read stored reset code: %v", err)
	}
	if stored == code {
		t.Fatal("reset code stored in plaintext; want a hash")
	}

	// 3. Wrong code -> validation error (400), generic message. Derived from the
	//    real code so the two can never coincide in the 6-digit code space.
	wrongCode := "000000"
	if code == wrongCode {
		wrongCode = "111111"
	}
	if st, env := h.do(t, http.MethodPost, "/api/v1/user/reset-password", "", map[string]string{
		"username": seedEmail, "code": wrongCode, "password": "newpass123",
	}); st != http.StatusBadRequest {
		t.Fatalf("reset wrong code status = %d; body: %s", st, env.raw)
	}

	// 4. Correct code -> success.
	if st, env := h.do(t, http.MethodPost, "/api/v1/user/reset-password", "", map[string]string{
		"username": seedEmail, "code": code, "password": "newpass123",
	}); st != http.StatusOK {
		t.Fatalf("reset status = %d; body: %s", st, env.raw)
	}

	// The reset rehashed the account to argon2id (issue #64).
	var alg string
	if err := h.db.QueryRow(`SELECT algorithm FROM users WHERE id = ?`, seedUserID).Scan(&alg); err != nil {
		t.Fatalf("read algorithm: %v", err)
	}
	if alg != "argon2id" {
		t.Errorf("algorithm after reset = %q, want argon2id", alg)
	}

	// 5. New password works; old one does not.
	if st, _ := h.do(t, http.MethodPost, "/api/v1/user/login-user", "", map[string]string{
		"username": seedEmail, "password": "newpass123",
	}); st != http.StatusOK {
		t.Fatalf("login with new password = %d, want 200", st)
	}
	if st, _ := h.do(t, http.MethodPost, "/api/v1/user/login-user", "", map[string]string{
		"username": seedEmail, "password": seedPassword,
	}); st == http.StatusOK {
		t.Fatal("old password should no longer work after reset")
	}

	// 6. The code is single-use: reusing it fails (it was deleted on reset).
	if st, _ := h.do(t, http.MethodPost, "/api/v1/user/reset-password", "", map[string]string{
		"username": seedEmail, "code": code, "password": "another12",
	}); st != http.StatusBadRequest {
		t.Fatalf("reused code should fail, got %d", st)
	}

	// 7. remind-password for an unknown user still returns 200 (anti-enumeration).
	if st, env := h.do(t, http.MethodPost, "/api/v1/user/remind-password", "", map[string]string{
		"username": "nobody@example.test",
	}); st != http.StatusOK {
		t.Fatalf("remind unknown user = %d, want 200; body: %s", st, env.raw)
	}
}
