package user_test

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

	// 2. Read the generated code from users_password_requests.
	var code string
	if err := h.db.QueryRow(`SELECT code FROM users_password_requests WHERE user_id = ?`, seedUserID).Scan(&code); err != nil {
		t.Fatalf("read reset code: %v", err)
	}
	if len(code) != 12 {
		t.Fatalf("code = %q, want 12 chars", code)
	}

	// 3. Wrong code -> validation error (400), generic message.
	if st, env := h.do(t, http.MethodPost, "/api/v1/user/reset-password", "", map[string]string{
		"username": seedEmail, "code": "wrongcode000", "password": "newpass123",
	}); st != http.StatusBadRequest {
		t.Fatalf("reset wrong code status = %d; body: %s", st, env.raw)
	}

	// 4. Correct code -> success.
	if st, env := h.do(t, http.MethodPost, "/api/v1/user/reset-password", "", map[string]string{
		"username": seedEmail, "code": code, "password": "newpass123",
	}); st != http.StatusOK {
		t.Fatalf("reset status = %d; body: %s", st, env.raw)
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
