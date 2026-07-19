package api_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/infra/ratelimit"
	appuser "github.com/econumo/econumo/internal/user"
)

// tight per-key limits so a handful of calls trips the 429; global generous so
// only the per-key path is under test here.
func newLimitedHarness(t *testing.T) *harness {
	t.Helper()
	limiter := ratelimit.New(ratelimit.Config{
		Limits: map[string]int{
			appuser.RateScopeLogin:    2,
			appuser.RateScopeReset:    2,
			appuser.RateScopeRemind:   2,
			appuser.RateScopeRegister: 2,
		},
		Window: 15 * time.Minute,
		Global: 1000,
	}, realClock{})
	return newHarnessWithLimiter(t, limiter)
}

// realClock: the limiter needs a monotonic-ish clock; the harness fixedClock
// would also work (all attempts land in one window), but real time matches
// production wiring.
type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func assert429(t *testing.T, status int, env envelope) {
	t.Helper()
	if status != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429 (body: %s)", status, env.raw)
	}
	if env.Success || env.Code != 429 || env.Message != "Too many attempts. Try again later." {
		t.Fatalf("envelope = %+v, want success=false code=429 frozen message", env)
	}
	if string(env.Errors) != "{}" {
		t.Fatalf("errors = %s, want {}", env.Errors)
	}
}

func login(h *harness, t *testing.T, username, password string) (int, envelope) {
	return h.do(t, "POST", "/api/v1/user/login-user", "", map[string]any{
		"username": username, "password": password,
	})
}

func TestLogin_RateLimitedAfterFailures(t *testing.T) {
	h := newLimitedHarness(t)
	for i := 0; i < 2; i++ {
		if status, _ := login(h, t, seedEmail, "wrong-pw"); status != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want 401", i+1, status)
		}
	}
	// Third attempt is blocked even with the CORRECT password: lockout is by
	// attempt count, not credential.
	status, env := login(h, t, seedEmail, seedPassword)
	assert429(t, status, env)
}

func TestLogin_UnknownUserAlsoRateLimited(t *testing.T) {
	h := newLimitedHarness(t)
	for i := 0; i < 2; i++ {
		if status, _ := login(h, t, "ghost@example.test", "x"); status != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want 401", i+1, status)
		}
	}
	status, env := login(h, t, "ghost@example.test", "x")
	assert429(t, status, env) // anti-enumeration: same behavior as an existing user
}

func TestLogin_SuccessClearsCounter(t *testing.T) {
	h := newLimitedHarness(t)
	if status, _ := login(h, t, seedEmail, "wrong-pw"); status != http.StatusUnauthorized {
		t.Fatal("expected 401")
	}
	if status, _ := login(h, t, seedEmail, seedPassword); status != http.StatusOK {
		t.Fatal("expected 200 (1 failure < limit 2)")
	}
	// Counter was cleared: two more failures allowed before a block.
	if status, _ := login(h, t, seedEmail, "wrong-pw"); status != http.StatusUnauthorized {
		t.Fatal("expected 401")
	}
	if status, _ := login(h, t, seedEmail, "wrong-pw"); status != http.StatusUnauthorized {
		t.Fatal("expected 401")
	}
	status, env := login(h, t, seedEmail, seedPassword)
	assert429(t, status, env)
}

func TestLogin_KeyIsCaseAndSpaceInsensitive(t *testing.T) {
	h := newLimitedHarness(t)
	if status, _ := login(h, t, seedEmail, "wrong-pw"); status != http.StatusUnauthorized {
		t.Fatal("expected 401")
	}
	if status, _ := login(h, t, "  "+upperFirst(seedEmail)+" ", "wrong-pw"); status != http.StatusUnauthorized {
		t.Fatal("expected 401")
	}
	// Both failures hit the same bucket -> third attempt blocked.
	status, env := login(h, t, seedEmail, seedPassword)
	assert429(t, status, env)
}

func upperFirst(s string) string {
	if s == "" {
		return s
	}
	return string(s[0]-32) + s[1:] // seedEmail starts with a lowercase ASCII letter
}

func TestLogin_NilLimiterUnlimited(t *testing.T) {
	h := newHarness(t) // nil limiter
	for i := 0; i < 10; i++ {
		if status, _ := login(h, t, seedEmail, "wrong-pw"); status != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want 401 (never 429)", i+1, status)
		}
	}
}

func remind(h *harness, t *testing.T, username string) (int, envelope) {
	return h.do(t, "POST", "/api/v1/user/remind-password", "", map[string]any{"username": username})
}

func reset(h *harness, t *testing.T, username, code, password string) (int, envelope) {
	return h.do(t, "POST", "/api/v1/user/reset-password", "", map[string]any{
		"username": username, "code": code, "password": password,
	})
}

// Remind counts EVERY request (each one sends an email), success included.
func TestRemindPassword_RateLimited(t *testing.T) {
	h := newLimitedHarness(t)
	for i := 0; i < 2; i++ {
		if status, _ := remind(h, t, seedEmail); status != http.StatusOK {
			t.Fatalf("attempt %d: expected 200", i+1)
		}
	}
	status, env := remind(h, t, seedEmail)
	assert429(t, status, env)
}

// Anti-enumeration holds under limiting: unknown users return the same 200s
// then the same 429.
func TestRemindPassword_UnknownUserSameShape(t *testing.T) {
	h := newLimitedHarness(t)
	for i := 0; i < 2; i++ {
		if status, _ := remind(h, t, "ghost@example.test"); status != http.StatusOK {
			t.Fatalf("attempt %d: expected 200 (anti-enumeration)", i+1)
		}
	}
	status, env := remind(h, t, "ghost@example.test")
	assert429(t, status, env)
}

func TestResetPassword_RateLimitedOnBadCodes(t *testing.T) {
	h := newLimitedHarness(t)
	for i := 0; i < 2; i++ {
		if status, _ := reset(h, t, seedEmail, "000000000000", "new-pw-123"); status != http.StatusBadRequest {
			t.Fatalf("attempt %d: expected 400", i+1)
		}
	}
	status, env := reset(h, t, seedEmail, "000000000000", "new-pw-123")
	assert429(t, status, env)
}

// A successful reset clears the counter; look up the real code in the DB the
// same way internal/user/api/reset_password_test.go does (read that file and
// reuse its query helper/pattern verbatim).
func TestResetPassword_SuccessClearsCounter(t *testing.T) {
	h := newLimitedHarness(t)
	if status, _ := reset(h, t, seedEmail, "000000000000", "new-pw-123"); status != http.StatusBadRequest {
		t.Fatal("expected 400")
	}
	if status, _ := remind(h, t, seedEmail); status != http.StatusOK {
		t.Fatal("expected 200 remind")
	}
	code := fetchResetCode(t, h)
	if status, _ := reset(h, t, seedEmail, code, "new-pw-123"); status != http.StatusOK {
		t.Fatal("expected 200 reset")
	}
	// Counter cleared: two more bad attempts before a block.
	if status, _ := reset(h, t, seedEmail, "000000000000", "x-pw-123"); status != http.StatusBadRequest {
		t.Fatal("expected 400")
	}
	if status, _ := reset(h, t, seedEmail, "000000000000", "x-pw-123"); status != http.StatusBadRequest {
		t.Fatal("expected 400")
	}
	status, env := reset(h, t, seedEmail, "000000000000", "x-pw-123")
	assert429(t, status, env)
}

// fetchResetCode returns the emitted reset code from the recording mailer (the
// code is hashed at rest, so the DB no longer holds the plaintext).
func fetchResetCode(t *testing.T, h *harness) string {
	t.Helper()
	return h.mail.lastResetCode(t)
}

func registerUser(h *harness, t *testing.T, email string) (int, envelope) {
	return h.do(t, "POST", "/api/v1/user/register-user", "", map[string]any{
		"email": email, "password": "brand-new-pw", "name": "New User",
	})
}

// Register counts EVERY attempt (mass-creation protection), success included.
func TestRegister_RateLimited(t *testing.T) {
	h := newLimitedHarness(t)
	if status, _ := registerUser(h, t, "fresh@example.test"); status != http.StatusOK {
		t.Fatal("expected 200 first register")
	}
	if status, _ := registerUser(h, t, "fresh@example.test"); status != http.StatusBadRequest {
		t.Fatal("expected 400 duplicate")
	}
	status, env := registerUser(h, t, "fresh@example.test")
	assert429(t, status, env)
}

func TestRegister_KeysIndependent(t *testing.T) {
	h := newLimitedHarness(t)
	for i := 0; i < 2; i++ {
		registerUser(h, t, "a@example.test")
	}
	if status, env := registerUser(h, t, "a@example.test"); status != http.StatusTooManyRequests {
		t.Fatalf("expected 429 for exhausted key, got %d: %s", status, env.raw)
	}
	if status, _ := registerUser(h, t, "b@example.test"); status != http.StatusOK {
		t.Fatal("expected 200 for a different email")
	}
}
