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
