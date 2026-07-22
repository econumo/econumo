package api_test

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// The resend cooldown travels on the standard Retry-After header, never in the
// response body — one representation, so the two can never disagree. This is
// also why the handler is hand-written instead of using endpoint.HandlePublic,
// which cannot set response headers.
func TestResendVerificationCodeCarriesRetryAfterHeader(t *testing.T) {
	h := newHarness(t)

	status, retryAfter := h.doHeader(t, http.MethodPost, "/api/v1/user/resend-verification-code",
		map[string]string{"username": seedEmail}, "Retry-After")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	secs, err := strconv.Atoi(retryAfter)
	if err != nil {
		t.Fatalf("Retry-After = %q, want an integer seconds value", retryAfter)
	}
	if secs != 60 {
		t.Errorf("Retry-After = %d, want 60", secs)
	}

	// The header is the ONLY place the wait appears: the body stays the frozen
	// empty-object envelope.
	if st, env := h.do(t, http.MethodPost, "/api/v1/user/resend-verification-code", "",
		map[string]string{"username": seedEmail}); st != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", st, env.raw)
	} else if strings.TrimSpace(string(env.raw)) != `{"success":true,"message":"","data":{}}` {
		t.Errorf("body = %s, want the empty-data envelope with no retryAfter field", env.raw)
	}
}

// An unknown username is answered identically — same status, same header — so
// the cooldown can never be read as proof that an account exists.
func TestResendVerificationCodeHeaderDoesNotLeakExistence(t *testing.T) {
	h := newHarness(t)

	realStatus, realRetry := h.doHeader(t, http.MethodPost, "/api/v1/user/resend-verification-code",
		map[string]string{"username": seedEmail}, "Retry-After")
	ghostStatus, ghostRetry := h.doHeader(t, http.MethodPost, "/api/v1/user/resend-verification-code",
		map[string]string{"username": "nobody@example.test"}, "Retry-After")

	if realStatus != ghostStatus || realRetry != ghostRetry {
		t.Errorf("responses differ: real=(%d,%q) unknown=(%d,%q)",
			realStatus, realRetry, ghostStatus, ghostRetry)
	}
}
