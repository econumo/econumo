package api_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/shared/errs"
)

// mustQuery parses a URL's query (same helper as internal/oauth's tests).
func mustQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u.Query()
}

func TestGetProviderList_Public(t *testing.T) {
	h := newHarness(t)
	status, env := h.do(t, http.MethodGet, "/api/v1/oauth/get-provider-list", "", nil)
	if status != 200 || !strings.Contains(string(env.Data), `"id":"google"`) {
		t.Fatalf("%d %s", status, env.raw)
	}
}

func TestStartLogin_ReturnsAuthorizationURL(t *testing.T) {
	h := newHarness(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/oauth/start-login", "", map[string]any{"provider": "google", "client": "web"})
	if status != 200 {
		t.Fatalf("%d %s", status, env.raw)
	}
	var res struct{ Url string }
	_ = json.Unmarshal(env.Data, &res)
	if !strings.HasPrefix(res.Url, h.fake.IssuerURL()+"/authorize?") {
		t.Fatalf("url %s", res.Url)
	}
	status, _ = h.do(t, http.MethodPost, "/api/v1/oauth/start-login", "", map[string]any{"provider": "apple2", "client": "web"})
	if status != 400 {
		t.Fatalf("unconfigured provider must be 400, got %d", status)
	}
}

func TestExchangeHandoff_RequiresTheInitiatingClientsFlowSecret(t *testing.T) {
	h := newHarness(t)
	_, env := h.do(t, http.MethodPost, "/api/v1/oauth/start-login", "", map[string]any{"provider": "google", "client": "web"})
	var res struct{ Url, Flow string }
	_ = json.Unmarshal(env.Data, &res)
	if len(res.Flow) != 43 {
		t.Fatalf("start-login must return the flow secret, got %q", res.Flow)
	}
	q := mustQuery(t, res.Url)
	code := h.fake.IssueCode(q.Get("nonce"), q.Get("code_challenge"))

	resp := h.rawGet(t, "/api/v1/oauth/callback-google?code="+url.QueryEscape(code)+"&state="+url.QueryEscape(q.Get("state")))
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if !strings.HasPrefix(loc, "https://app.example.test/oauth/callback#handoff=") {
		t.Fatalf("location %s", loc)
	}
	frag, _ := url.ParseQuery(strings.SplitN(loc, "#", 2)[1])
	// A handoff presented without the initiating client's flow secret is refused.
	status, _ := h.doRaw(t, http.MethodPost, "/api/v1/oauth/exchange-handoff", "", map[string]any{"code": frag.Get("handoff"), "flow": "wrong"})
	if status != 401 {
		t.Fatalf("a foreign flow secret must be 401, got %d", status)
	}
}

func TestCallbackGoogle_RedirectsWithHandoff_ThenExchange(t *testing.T) {
	h := newHarness(t)
	_, env := h.do(t, http.MethodPost, "/api/v1/oauth/start-login", "", map[string]any{"provider": "google", "client": "web"})
	var res struct{ Url, Flow string }
	_ = json.Unmarshal(env.Data, &res)
	q := mustQuery(t, res.Url)
	code := h.fake.IssueCode(q.Get("nonce"), q.Get("code_challenge"))

	resp := h.rawGet(t, "/api/v1/oauth/callback-google?code="+url.QueryEscape(code)+"&state="+url.QueryEscape(q.Get("state")))
	loc := resp.Header.Get("Location")
	frag, _ := url.ParseQuery(strings.SplitN(loc, "#", 2)[1])
	body := map[string]any{"code": frag.Get("handoff"), "flow": res.Flow}
	status, raw := h.doRaw(t, http.MethodPost, "/api/v1/oauth/exchange-handoff", "", body)
	if status != 200 || !strings.Contains(string(raw), `"token":"eco_ses_`) || strings.Contains(string(raw), `"success"`) {
		t.Fatalf("exchange must be the raw login shape: %d %s", status, raw)
	}
	status, _ = h.doRaw(t, http.MethodPost, "/api/v1/oauth/exchange-handoff", "", body)
	if status != 401 {
		t.Fatalf("second exchange must be 401, got %d", status)
	}
}

func TestCallbackApple_FormPost(t *testing.T) {
	h := newHarness(t) // the harness configures the fake as "apple" too (form_post, no PKCE)
	_, env := h.do(t, http.MethodPost, "/api/v1/oauth/start-login", "", map[string]any{"provider": "apple", "client": "app"})
	var res struct{ Url string }
	_ = json.Unmarshal(env.Data, &res)
	q := mustQuery(t, res.Url)
	code := h.fake.IssueCode(q.Get("nonce"), "")
	form := url.Values{"code": {code}, "state": {q.Get("state")}, "user": {`{"name":{"firstName":"Ada","lastName":"Lovelace"}}`}}
	resp := h.rawPostForm(t, "/api/v1/oauth/callback-apple", form)
	if resp.StatusCode != http.StatusFound || !strings.HasPrefix(resp.Header.Get("Location"), "econumo://oauth?handoff=") {
		t.Fatalf("%d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
}

func TestCallback_ErrorRedirect(t *testing.T) {
	h := newHarness(t)
	resp := h.rawGet(t, "/api/v1/oauth/callback-oidc?code=x&state=bogus")
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "https://app.example.test/login?oauthError=invalid_state" {
		t.Fatalf("%d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
}

func TestIdentityEndpoints_RequireAuth(t *testing.T) {
	h := newHarness(t)
	if status, _ := h.do(t, http.MethodGet, "/api/v1/oauth/get-identity-list", "", nil); status != 401 {
		t.Fatalf("want 401, got %d", status)
	}
	token := h.issueToken(t)
	status, env := h.do(t, http.MethodGet, "/api/v1/oauth/get-identity-list", token, nil)
	if status != 200 || string(env.Data) != "[]" {
		t.Fatalf("%d %s", status, env.raw)
	}
	status, _ = h.do(t, http.MethodPost, "/api/v1/oauth/unlink-identity", token, map[string]any{"provider": "google"})
	if status != 400 {
		t.Fatalf("unlinking nothing is 400, got %d", status)
	}
	status, env = h.do(t, http.MethodPost, "/api/v1/oauth/start-link", token, map[string]any{"provider": "google", "client": "web"})
	if status != 200 || !strings.Contains(string(env.Data), `"url"`) {
		t.Fatalf("%d %s", status, env.raw)
	}
}

// blockedLimiter stands in for a limiter whose global per-minute cap is spent.
type blockedLimiter struct{}

func (blockedLimiter) Allow(string, string) error {
	return errs.NewTooManyRequestsRetryAfter("Too many attempts. Try again later.", 30)
}

func TestStartLogin_RateLimited(t *testing.T) {
	h := newHarnessWith(t, blockedLimiter{})
	status, env := h.do(t, http.MethodPost, "/api/v1/oauth/start-login", "", map[string]any{"provider": "google", "client": "web"})
	if status != 429 || !strings.Contains(string(env.raw), "Too many attempts") {
		t.Fatalf("%d %s", status, env.raw)
	}
}
