package api_test

import (
	"net/http"
	"testing"
)

// TestCredentialsAlgorithm covers issue #64's core matrix over the real HTTP
// handlers: legacy (sha512) users keep logging in, registration writes
// argon2id, and both kinds of hash verify only their own password.
func TestCredentialsAlgorithm(t *testing.T) {
	h := newHarness(t)

	// The fixture-seeded user is a legacy sha512 account.
	var alg string
	if err := h.db.QueryRow(`SELECT algorithm FROM users WHERE id = ?`, seedUserID).Scan(&alg); err != nil {
		t.Fatalf("read seed algorithm: %v", err)
	}
	if alg != "sha512" {
		t.Fatalf("seed user algorithm = %q, want sha512", alg)
	}
	if st, env := h.do(t, http.MethodPost, "/api/v1/user/login-user", "", map[string]string{
		"username": seedEmail, "password": seedPassword,
	}); st != http.StatusOK {
		t.Fatalf("legacy login = %d; body: %s", st, env.raw)
	}
	if st, _ := h.do(t, http.MethodPost, "/api/v1/user/login-user", "", map[string]string{
		"username": seedEmail, "password": "wrong-pw",
	}); st != http.StatusUnauthorized {
		t.Fatalf("legacy login with wrong password = %d, want 401", st)
	}

	// Registration creates an argon2id user.
	if st, env := h.do(t, http.MethodPost, "/api/v1/user/register-user", "", map[string]string{
		"name": "New User", "email": "new@example.test", "password": "brand-new-pw",
	}); st != http.StatusOK {
		t.Fatalf("register = %d; body: %s", st, env.raw)
	}
	var newAlg, newHash string
	if err := h.db.QueryRow(`SELECT algorithm, password FROM users WHERE email <> '' AND id <> ? ORDER BY created_at DESC LIMIT 1`,
		seedUserID).Scan(&newAlg, &newHash); err != nil {
		t.Fatalf("read new user: %v", err)
	}
	if newAlg != "argon2id" {
		t.Errorf("new user algorithm = %q, want argon2id", newAlg)
	}
	if len(newHash) == 0 || newHash[0] != '$' {
		t.Errorf("new user hash is not a PHC string: %q", newHash)
	}

	// The argon2id user can log in; a wrong password is rejected.
	if st, env := h.do(t, http.MethodPost, "/api/v1/user/login-user", "", map[string]string{
		"username": "new@example.test", "password": "brand-new-pw",
	}); st != http.StatusOK {
		t.Fatalf("argon2id login = %d; body: %s", st, env.raw)
	}
	if st, _ := h.do(t, http.MethodPost, "/api/v1/user/login-user", "", map[string]string{
		"username": "new@example.test", "password": "wrong-pw",
	}); st != http.StatusUnauthorized {
		t.Fatalf("argon2id login with wrong password = %d, want 401", st)
	}
}
